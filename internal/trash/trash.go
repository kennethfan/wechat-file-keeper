// Package trash 把文件移入废纸篓，并记录可回滚的清单。
//
// 唯一的删除动作是 rename 到 ~/.Trash，绝不 unlink（见 ADR-0003）。
// 每次清理写一份 JSON 清单，作为撤销与审计的依据。
package trash

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

var (
	// ErrCrossDevice 目标与废纸篓不在同一卷。
	ErrCrossDevice = errors.New("目标与废纸篓不在同一卷，无法安全移动")
)

// Record 是清单中的一条记录。
type Record struct {
	// OriginalPath 是文件移动前的绝对路径。
	OriginalPath string `json:"original_path"`
	// TrashedPath 是文件在废纸篓中的绝对路径（可能因重名而改名）。
	TrashedPath string `json:"trashed_path"`
	// Size 是文件字节数。
	Size int64 `json:"size"`
	// Category 是清理时的分类。
	Category string `json:"category"`
	// TrashedAt 是移动时间的 RFC 3339 表示。
	TrashedAt string `json:"trashed_at"`
}

// Manifest 是一次清理的完整记录。
type Manifest struct {
	// CreatedAt 是本次清理的开始时间。
	CreatedAt string `json:"created_at"`
	// Library 是被清理的账号库路径。
	Library string `json:"library"`
	// Records 是逐条移动记录，按执行顺序排列。
	Records []Record `json:"records"`
}

// TotalBytes 汇总本次清理移动的字节数。
func (m Manifest) TotalBytes() int64 {
	var total int64
	for _, r := range m.Records {
		total += r.Size
	}
	return total
}

// Cleaner 执行清理。
type Cleaner struct {
	// TrashDir 是废纸篓目录，默认 ~/.Trash。
	TrashDir string
	// ManifestDir 是清单存放目录。
	ManifestDir string
	// Library 是被清理的账号库路径。
	Library string
	// Now 便于测试注入时间，为空时使用 time.Now。
	Now func() time.Time
	// Method 指定删除方式。已废弃：现在只有 Finder 一种。
	Method Method
	// finderMove 可注入，便于测试；为 nil 时用真实的 FinderMove。
	finderMove func(paths []string) error
	// findTrashPath 可注入，便于测试；为 nil 时用真实的 FindTrashPath。
	findTrashPath func(original string) (string, error)
}

// New 创建使用默认位置的清理器。
func New(manifestDir, library string) *Cleaner {
	home, err := os.UserHomeDir()
	trashDir := ""
	if err == nil {
		trashDir = filepath.Join(home, ".Trash")
	}
	return &Cleaner{
		TrashDir:    trashDir,
		ManifestDir: manifestDir,
		Library:     library,
	}
}

// Move 把给定条目移入废纸篓，返回清单。
//
// 单条失败不中断整体：已成功的条目仍会写入清单，失败的收集在返回的
// 错误里。这样用户能撤销已成功的部分，而不会因一条坏文件全盘作废。
func (c *Cleaner) Move(entries []model.Entry) (Manifest, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	startedAt := now()

	m := Manifest{
		CreatedAt: startedAt.Format(time.RFC3339),
		Library:   c.Library,
		Records:   make([]Record, 0, len(entries)),
	}

	// 唯一删除方式：交给 Finder。
	//
	// 之前还有一条 rename 路径（自己搬进 ~/.Trash），但那条路的问题在于
	// 系统不记录原路径，废纸篓里既没有「放回原处」也难以辨认——等于自己
	// 造了个废纸篓。既然目标就是系统废纸篓，直接用 Finder 才说得通。
	return c.moveViaFinder(entries, m, now)
}

func (c *Cleaner) moveViaFinderUnused(entries []model.Entry, m Manifest, now func() time.Time) (Manifest, error) {
	if c.TrashDir == "" {
		return m, fmt.Errorf("无法确定废纸篓目录")
	}
	if err := os.MkdirAll(c.TrashDir, 0o700); err != nil {
		return m, fmt.Errorf("无法访问废纸篓 %s: %w", c.TrashDir, err)
	}

	var failed []string
	for _, e := range entries {
		if err := c.checkSameVolume(e.Path); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", e.Path, err))
			continue
		}

		dest, err := c.uniqueDest(filepath.Base(e.Path))
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", e.Path, err))
			continue
		}
		if err := os.Rename(e.Path, dest); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", e.Path, err))
			continue
		}

		m.Records = append(m.Records, Record{
			OriginalPath: e.Path,
			TrashedPath:  dest,
			Size:         e.Size,
			Category:     string(e.Category),
			TrashedAt:    now().Format(time.RFC3339),
		})
	}

	if len(m.Records) > 0 {
		if err := c.writeManifest(m); err != nil {
			// 清单写不进去时，已移动的文件仍在废纸篓里，但无法自动撤销。
			return m, fmt.Errorf("文件已移动但清单写入失败（无法自动撤销）: %w", err)
		}
	}

	if len(failed) > 0 {
		return m, fmt.Errorf("%d 个条目未能移动: %v", len(failed), failed)
	}
	return m, nil
}

// checkSameVolume 确认目标与废纸篓在同一卷。
//
// 跨卷时 rename 会失败；提前检测能给出明确错误，而不是让调用方
// 面对一个含糊的系统错误，也避免有人误改成「复制 + 删除」。
func (c *Cleaner) checkSameVolume(path string) error {
	srcInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("无法访问源目录: %w", err)
	}
	dstInfo, err := os.Stat(c.TrashDir)
	if err != nil {
		return fmt.Errorf("无法访问废纸篓: %w", err)
	}
	srcDev, ok1 := deviceOf(srcInfo)
	dstDev, ok2 := deviceOf(dstInfo)
	if ok1 && ok2 && srcDev != dstDev {
		return ErrCrossDevice
	}
	return nil
}

// uniqueDest 在废纸篓中找一个不冲突的目标路径。
//
// 同名已存在时追加 " (2)"、" (3)" 等序号，与 Finder 的习惯一致。
// 返回的路径会被写进清单，撤销据此找回文件。
func (c *Cleaner) uniqueDest(name string) (string, error) {
	candidate := filepath.Join(c.TrashDir, name)
	if _, err := os.Lstat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}

	ext := filepath.Ext(name)
	stem := name[:len(name)-len(ext)]
	for i := 2; i < 1000; i++ {
		candidate = filepath.Join(c.TrashDir, fmt.Sprintf("%s (%d)%s", stem, i, ext))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("废纸篓中同名文件过多: %s", name)
}

// writeManifest 把清单写入清单目录。
func (c *Cleaner) writeManifest(m Manifest) error {
	if err := os.MkdirAll(c.ManifestDir, 0o755); err != nil {
		return fmt.Errorf("无法创建清单目录 %s: %w", c.ManifestDir, err)
	}
	path := filepath.Join(c.ManifestDir, ManifestName(m.CreatedAt))
	return writeJSON(path, m)
}

// ManifestName 由创建时间生成清单文件名。
//
// 时间里的冒号在文件名中不合法，统一替换为短横线。
func ManifestName(createdAt string) string {
	safe := make([]rune, 0, len(createdAt))
	for _, r := range createdAt {
		switch r {
		case ':', '/', ' ':
			safe = append(safe, '-')
		default:
			safe = append(safe, r)
		}
	}
	return "clean-" + string(safe) + ".json"
}

// writeJSON 原子写入：先写临时文件再 rename，避免留下半个清单。
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化清单失败: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("写入临时清单失败: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("提交清单失败: %w", err)
	}
	return nil
}

// Load 读取指定清单文件。
func Load(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("读取清单 %s 失败: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("解析清单 %s 失败: %w", path, err)
	}
	return m, nil
}

// Latest 返回清单目录中最新的一份清单路径。
func Latest(manifestDir string) (string, error) {
	names, err := listManifests(manifestDir)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", fmt.Errorf("清单目录 %s 中没有任何清单", manifestDir)
	}
	return filepath.Join(manifestDir, names[len(names)-1]), nil
}

// listManifests 返回清单文件名，按名称升序（名称含时间，故即时间序）。
func listManifests(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取清单目录 %s 失败: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) != ".json" || len(name) < 7 || name[:6] != "clean-" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// moveViaFinder 用 Finder 删除，并记录实际落入废纸篓的路径。
//
// Finder 模式下系统会保留「放回原处」，但代价是需要自动化授权。
// 授权被拒时回退到 rename——用户至少还能通过清单撤销。
func (c *Cleaner) moveViaFinder(entries []model.Entry, m Manifest, now func() time.Time) (Manifest, error) {
	paths := make([]string, 0, len(entries))
	byPath := make(map[string]model.Entry, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
		byPath[e.Path] = e
	}

	mover := c.finderMove
	if mover == nil {
		mover = FinderMove
	}
	if err := mover(paths); err != nil {
		// 不回退到 rename：那样会得到一个「用户以为能放回原处、
		// 实际不能」的废纸篓，比直接失败更糟。
		return m, fmt.Errorf("移入废纸篓失败：%w\n\n请到「系统设置 → 隐私与安全性 → 自动化」允许本应用控制 Finder", err)
	}

	finder := c.findTrashPath
	if finder == nil {
		finder = FindTrashPath
	}

	for _, p := range paths {
		trashed, findErr := finder(p)
		if findErr != nil {
			// 找不到落点也要记录，撤销时会被跳过并报告。
			trashed = ""
		}
		e := byPath[p]
		m.Records = append(m.Records, Record{
			OriginalPath: p,
			TrashedPath:  trashed,
			Size:         e.Size,
			Category:     string(e.Category),
			TrashedAt:    now().Format(time.RFC3339),
		})
	}

	if len(m.Records) > 0 {
		if err := c.writeManifest(m); err != nil {
			return m, fmt.Errorf("文件已删除但清单写入失败: %w", err)
		}
	}
	return m, nil
}

// NewForTest 构造一个把文件搬到指定目录的 Cleaner，供其他包的测试使用。
//
// 真实删除走 Finder，需要系统授权，单元测试无法依赖。
// 这个构造器用 rename 模拟 Finder 的行为，只验证上层逻辑。
func NewForTest(trashDir, manifestDir string, now func() time.Time) *Cleaner {
	return &Cleaner{
		TrashDir:    trashDir,
		ManifestDir: manifestDir,
		Now:         now,
		finderMove: func(paths []string) error {
			for _, p := range paths {
				dest := filepath.Join(trashDir, filepath.Base(p))
				if _, err := os.Stat(dest); err == nil {
					dest = filepath.Join(trashDir, "dup-"+filepath.Base(p))
				}
				if err := os.Rename(p, dest); err != nil {
					return err
				}
			}
			return nil
		},
		findTrashPath: func(original string) (string, error) {
			base := filepath.Base(original)
			for _, name := range []string{base, "dup-" + base} {
				p := filepath.Join(trashDir, name)
				if _, err := os.Stat(p); err == nil {
					return p, nil
				}
			}
			return "", os.ErrNotExist
		},
	}
}
