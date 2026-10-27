// Package scan 遍历账号库的消息区，产出条目与空间统计。
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/classify"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/orphan"
)

// monthPattern 匹配微信的月份目录名，形如 "2025-02"。
var monthPattern = regexp.MustCompile(`^\d{4}-\d{2}$`)

// Result 是一次扫描的完整产物。
type Result struct {
	// Entries 是全部条目，含分类与缩略图标记。
	Entries []model.Entry
	// Space 是空间统计。
	Space model.SpaceReport
	// Duration 是扫描耗时。
	Duration time.Duration
}

// Scanner 遍历一个账号库的消息区。
type Scanner struct {
	// LibPath 是账号库根目录。
	LibPath string
}

// New 创建一个针对指定账号库的扫描器。
func New(libPath string) *Scanner {
	return &Scanner{LibPath: libPath}
}

// msgRoot 返回消息区目录。
func (s *Scanner) msgRoot() string {
	return filepath.Join(s.LibPath, "msg")
}

// Run 执行全量扫描。
//
// 遍历消息区下所有文件，为每条路径产出一条 Entry，并按 inode 归并
// 计算空间统计（见 ADR-0001：实占必须按 inode 归并，否则硬链接会
// 被重复计入）。空目录不产生条目，也不报错。
func (s *Scanner) Run() (Result, error) {
	start := time.Now()
	root := s.msgRoot()

	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			// 账号库存在但没有消息区：合法状态，返回空结果。
			return Result{Duration: time.Since(start)}, nil
		}
		return Result{}, fmt.Errorf("无法访问消息区 %s: %w", root, err)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("消息区 %s 不是目录", root)
	}

	var entries []model.Entry
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			// 单个文件无权访问或已消失，跳过而不中断整次扫描。
			return nil
		}
		if d.IsDir() {
			return nil
		}
		fi, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		inode, nlink := linkInfo(fi)
		entries = append(entries, model.Entry{
			Path:    path,
			Inode:   inode,
			Nlink:   nlink,
			Size:    fi.Size(),
			ModTime: fi.ModTime().Unix(),
			Month:   monthOf(path),
		})
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("遍历消息区失败: %w", err)
	}

	// 分类与孤儿标记在扫描阶段完成，保证 CLI 与 GUI 拿到同一份结果。
	classify.Apply(entries)
	orphan.Mark(entries)

	space := Summarize(entries)
	return Result{
		Entries:  entries,
		Space:    space,
		Duration: time.Since(start),
	}, nil
}

// Summarize 按 inode 归并计算空间统计。
//
// Nominal 是路径累加值；Actual 是 inode 去重后的值。两者之差即硬链接
// 虚增——删掉这些重复路径不会释放任何空间。
func Summarize(entries []model.Entry) model.SpaceReport {
	seen := make(map[uint64]struct{}, len(entries))
	var nominal, actual int64
	fileCount := 0

	for _, e := range entries {
		nominal += e.Size
		// Inode 为 0 表示当前平台取不到 inode（见 link_other.go），
		// 此时退化为按路径累加，不能把全部条目误归并成一个。
		if e.Inode == 0 {
			actual += e.Size
			fileCount++
			continue
		}
		if _, dup := seen[e.Inode]; dup {
			continue
		}
		seen[e.Inode] = struct{}{}
		actual += e.Size
		fileCount++
	}

	return model.SpaceReport{
		Nominal:           nominal,
		Actual:            actual,
		HardlinkOvercount: nominal - actual,
		EntryCount:        len(entries),
		FileCount:         fileCount,
	}
}

// monthOf 从路径中提取月份目录名。
//
// 微信把消息区文件放在 .../YYYY-MM/ 下，但也存在更深的层级
// （attach/<指纹>/YYYY-MM/Img/）。这里取路径中最后一个匹配
// YYYY-MM 的目录成分。
func monthOf(path string) string {
	dir := filepath.Dir(path)
	for {
		base := filepath.Base(dir)
		if monthPattern.MatchString(base) {
			return base
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// SortBySizeDesc 按字节数降序排列条目。
func SortBySizeDesc(entries []model.Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Size > entries[j].Size
	})
}
