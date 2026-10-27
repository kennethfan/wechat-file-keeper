// Package dup 识别真重复：不同 inode 但内容完全一致的条目。
//
// 硬链接（同 inode）不算真重复——删除它们不释放任何空间，
// 混入结果会让用户误以为能回收空间（见 ADR-0001）。
package dup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// Group 是一组内容相同的条目。
type Group struct {
	// Hash 是这组内容的 SHA-256。
	Hash string
	// Size 是单份内容的字节数。
	Size int64
	// Entries 是这组内的全部条目，按路径排序。
	Entries []model.Entry
}

// Reclaimable 返回这组可回收的字节数。
//
// 按不同 inode 计算而非路径数：组内若混有硬链接（同 inode 多条路径），
// 保留每 inode 一份即可，删掉硬链接副本释放 0 字节。
func (g Group) Reclaimable() int64 {
	n := distinctInodes(g.Entries)
	if n <= 1 {
		return 0
	}
	return int64(n-1) * g.Size
}

// Finder 查找真重复。
type Finder struct {
	// MaxHashBytes 限制单个文件参与哈希的最大字节数，0 表示不限制。
	// 用于避免超大视频拖慢分析。
	MaxHashBytes int64
}

// Find 返回全部真重复分组，按可回收字节数降序。
//
// 两阶段：先按字节数分组粗筛（秒级），仅对同大小且分属不同 inode
// 的候选组算 SHA-256。这样既避免了对全量数据哈希，又不会把
// 「同大小但内容不同」误判为重复。
func (f Finder) Find(entries []model.Entry) ([]Group, error) {
	bySize := make(map[int64][]model.Entry, len(entries))
	for _, e := range entries {
		if e.Size <= 0 {
			continue
		}
		bySize[e.Size] = append(bySize[e.Size], e)
	}

	var groups []Group
	for size, candidates := range bySize {
		if len(candidates) < 2 {
			continue
		}
		// 同一 inode 的多条路径是硬链接，不是真重复。
		if distinctInodes(candidates) < 2 {
			continue
		}
		if f.MaxHashBytes > 0 && size > f.MaxHashBytes {
			continue
		}

		byHash := make(map[string][]model.Entry, len(candidates))
		for _, e := range candidates {
			sum, err := hashFile(e.Path)
			if err != nil {
				// 单个文件读失败不影响整体分析。
				continue
			}
			byHash[sum] = append(byHash[sum], e)
		}

		for sum, same := range byHash {
			if len(same) < 2 {
				continue
			}
			// 哈希相同但全部同 inode：仍是硬链接，不算真重复。
			if distinctInodes(same) < 2 {
				continue
			}
			sort.Slice(same, func(i, j int) bool { return same[i].Path < same[j].Path })
			groups = append(groups, Group{Hash: sum, Size: size, Entries: same})
		}
	}

	sort.Slice(groups, func(i, j int) bool {
		ri, rj := groups[i].Reclaimable(), groups[j].Reclaimable()
		if ri != rj {
			return ri > rj
		}
		return groups[i].Hash < groups[j].Hash
	})
	return groups, nil
}

// TotalReclaimable 汇总所有分组的可回收字节数。
func TotalReclaimable(groups []Group) int64 {
	var total int64
	for _, g := range groups {
		total += g.Reclaimable()
	}
	return total
}

// distinctInodes 统计一组条目里不同的 inode 数量。
//
// Inode 为 0 表示当前平台取不到 inode（见 scan 包的平台实现），
// 此时无法判断是否硬链接，按「每条路径独立」保守处理——
// 宁可高估可回收空间，也不能把独立副本误当成硬链接而漏报。
func distinctInodes(entries []model.Entry) int {
	seen := make(map[uint64]struct{}, len(entries))
	unknown := 0
	for _, e := range entries {
		if e.Inode == 0 {
			unknown++
			continue
		}
		seen[e.Inode] = struct{}{}
	}
	return len(seen) + unknown
}

// hashFile 计算文件的 SHA-256。
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开 %s 失败: %w", path, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
