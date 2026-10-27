// Package account 负责发现本机的微信账号库。
package account

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNoAccount 表示在给定根目录下没有找到任何账号库。
var ErrNoAccount = errors.New("未找到微信账号库")

// ErrRootMissing 表示微信数据根目录不存在。
var ErrRootMissing = errors.New("微信数据根目录不存在")

// Library 是一个账号库。
type Library struct {
	// WXID 是账号标识，取自目录名。
	WXID string
	// Path 是账号库根目录的绝对路径。
	Path string
}

// Root 返回本机微信数据根目录的默认位置。
//
// 微信 4.x 把数据放在沙箱容器的 Documents 下；这里返回 xwechat_files
// 这一层，其下每个 wxid_* 子目录是一个账号库。
func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户主目录: %w", err)
	}
	return filepath.Join(home,
		"Library", "Containers", "com.tencent.xinWeChat",
		"Data", "Documents", "xwechat_files"), nil
}

// Discover 枚举 root 下所有账号库，按 WXID 字典序返回。
//
// 只把目录名以 "wxid_" 开头且确实存在的目录当作账号库；
// 微信还会在 root 下放 all_users、Backup 等非账号目录，这些被忽略。
func Discover(root string) ([]Library, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrRootMissing, root)
		}
		return nil, fmt.Errorf("无法访问 %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s 不是目录", ErrRootMissing, root)
	}

	dirents, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("无法读取 %s: %w", root, err)
	}

	var libs []Library
	for _, d := range dirents {
		if !d.IsDir() || !strings.HasPrefix(d.Name(), "wxid_") {
			continue
		}
		libs = append(libs, Library{
			WXID: d.Name(),
			Path: filepath.Join(root, d.Name()),
		})
	}

	sort.Slice(libs, func(i, j int) bool { return libs[i].WXID < libs[j].WXID })
	return libs, nil
}

// SizeOf 返回账号库的实占字节数（按 inode 归并）。
//
// 只统计账号库内文件，符号链接不计入自身大小。统计失败时跳过该文件
// 而不是整体失败——账号库随时在变，个别文件消失是正常现象。
func SizeOf(lib Library) (int64, error) {
	seen := make(map[uint64]struct{}, 1024)
	var total int64

	err := filepath.WalkDir(lib.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 无权访问或已消失，跳过
		}
		if d.IsDir() {
			return nil
		}
		fi, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		inode, ok := inodeOf(fi)
		if !ok {
			total += fi.Size()
			return nil
		}
		if _, dup := seen[inode]; dup {
			return nil
		}
		seen[inode] = struct{}{}
		total += fi.Size()
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("统计账号库 %s 失败: %w", lib.WXID, err)
	}
	return total, nil
}

// Largest 返回实占最大的账号库。
//
// 这是工具的默认选择：用户通常只关心占用最大的那个账号。
// 没有任何账号库时返回 ErrNoAccount。
func Largest(root string) (Library, error) {
	libs, err := Discover(root)
	if err != nil {
		return Library{}, err
	}
	if len(libs) == 0 {
		return Library{}, fmt.Errorf("%w: %s", ErrNoAccount, root)
	}

	var best Library
	var bestSize int64 = -1
	for _, lib := range libs {
		size, sizeErr := SizeOf(lib)
		if sizeErr != nil {
			continue
		}
		if size > bestSize {
			best, bestSize = lib, size
		}
	}
	if bestSize < 0 {
		return Library{}, fmt.Errorf("%w: 所有账号库均无法统计", ErrNoAccount)
	}
	return best, nil
}
