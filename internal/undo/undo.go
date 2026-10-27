// Package undo 按清理清单把文件从废纸篓搬回原位。
//
// macOS 只对 Finder 移入的项目保留「放回原处」元数据，本工具用 rename
// 移入废纸篓（见 ADR-0003），因此撤销能力完全由清单保证——这就是本包
// 存在的理由。
package undo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kennethfan/wechat-file-keeper/internal/trash"
)

var (
	// ErrInvalidManifest 清单内容不完整或自相矛盾，拒绝执行。
	ErrInvalidManifest = errors.New("清单内容无效")
)

// SkipReason 说明某条记录为何未被恢复。
type SkipReason struct {
	// Record 是未恢复的记录。
	Record trash.Record
	// Reason 是未恢复的原因。
	Reason string
}

// Result 是一次撤销的结果。
type Result struct {
	// Restored 是已恢复的记录，按恢复顺序排列。
	Restored []trash.Record
	// Skipped 是未恢复的记录及原因。
	Skipped []SkipReason
	// RestoredBytes 是已恢复的字节数合计。
	RestoredBytes int64
}

// Apply 按清单逆序恢复文件。
//
// 逆序执行：后删的先回，这样同一路径被多次清理时能还原到较早的状态，
// 也减少恢复过程中的路径冲突。
//
// 单条失败不中断整体：已不在废纸篓或目标已被占用的条目会被跳过并报告，
// 其余条目照常恢复。
func Apply(m trash.Manifest) (Result, error) {
	if err := Validate(m); err != nil {
		return Result{}, err
	}

	var res Result
	for i := len(m.Records) - 1; i >= 0; i-- {
		r := m.Records[i]

		// 用户可能手动清空过废纸篓，此时文件已不存在，跳过而非失败。
		if _, err := os.Lstat(r.TrashedPath); err != nil {
			res.Skipped = append(res.Skipped, SkipReason{
				Record: r,
				Reason: "已不在废纸篓",
			})
			continue
		}

		// 目标位置已被占用时绝不覆盖：覆盖会丢失用户后来的文件。
		if _, err := os.Lstat(r.OriginalPath); err == nil {
			res.Skipped = append(res.Skipped, SkipReason{
				Record: r,
				Reason: "原位置已有同名文件",
			})
			continue
		}

		// 原路径的父目录可能已被清理掉，恢复前重建。
		if err := os.MkdirAll(filepath.Dir(r.OriginalPath), 0o755); err != nil {
			res.Skipped = append(res.Skipped, SkipReason{
				Record: r,
				Reason: fmt.Sprintf("无法重建父目录: %v", err),
			})
			continue
		}

		if err := os.Rename(r.TrashedPath, r.OriginalPath); err != nil {
			res.Skipped = append(res.Skipped, SkipReason{
				Record: r,
				Reason: fmt.Sprintf("移动失败: %v", err),
			})
			continue
		}

		res.Restored = append(res.Restored, r)
		res.RestoredBytes += r.Size
	}
	return res, nil
}

// Validate 检查清单是否可安全执行。
//
// 任一记录缺少路径或出现重复原路径，都视为清单损坏并整体拒绝：
// 部分执行会让用户难以判断到底恢复了多少，不如直接报错。
func Validate(m trash.Manifest) error {
	if len(m.Records) == 0 {
		return fmt.Errorf("%w: 清单中没有记录", ErrInvalidManifest)
	}

	seen := make(map[string]struct{}, len(m.Records))
	for i, r := range m.Records {
		if r.OriginalPath == "" || r.TrashedPath == "" {
			return fmt.Errorf("%w: 第 %d 条记录缺少路径", ErrInvalidManifest, i+1)
		}
		if !filepath.IsAbs(r.OriginalPath) || !filepath.IsAbs(r.TrashedPath) {
			return fmt.Errorf("%w: 第 %d 条记录的路径不是绝对路径", ErrInvalidManifest, i+1)
		}
		if _, dup := seen[r.OriginalPath]; dup {
			return fmt.Errorf("%w: 原路径重复 %s", ErrInvalidManifest, r.OriginalPath)
		}
		seen[r.OriginalPath] = struct{}{}
	}
	return nil
}
