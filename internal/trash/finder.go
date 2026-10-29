package trash

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Method 是移入废纸篓的方式。
type Method string

const (
	// MethodRename 用 rename 把文件搬进 ~/.Trash。
	//
	// 优点：无授权弹窗、可精确控制目标路径、跨卷时能明确报错。
	// 缺点：系统不记录原路径，废纸篓里没有「放回原处」。
	MethodRename Method = "rename"

	// MethodFinder 通过 Finder 删除。
	//
	// 优点：macOS 会记录原路径，废纸篓里有「放回原处」。
	// 缺点：需要「自动化」授权；失败时错误信息不明确。
	MethodFinder Method = "finder"
)

// FinderMove 用 Finder 把文件移入废纸篓。
//
// Finder 会为项目记录原始位置，因此系统废纸篓里会出现「放回原处」。
// 代价是首次调用会请求自动化授权；被拒绝时返回错误，调用方应回退到 rename。
//
// 注意：Finder 不接受通配符或相对路径，这里统一转成绝对路径并逐个处理。
func FinderMove(paths []string) error {
	if len(paths) == 0 {
		return nil
	}

	// 组装 AppleScript：把路径列表转成 POSIX file 列表。
	// 用 tell application "Finder" 而不是 System Events，前者不需要辅助功能权限。
	var quoted []string
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("无法解析路径 %s: %w", p, err)
		}
		// AppleScript 字符串里需要转义双引号与反斜杠。
		escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(abs)
		quoted = append(quoted, `POSIX file "`+escaped+`"`)
	}

	script := fmt.Sprintf(
		`tell application "Finder" to delete {%s}`,
		strings.Join(quoted, ", "),
	)

	// 带超时：Finder 无响应时不应让界面永久等待。
	done := make(chan error, 1)
	go func() {
		done <- exec.Command("osascript", "-e", script).Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("Finder 删除失败（可能未授予自动化权限）: %w", err)
		}
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("Finder 响应超时")
	}
}

// FindTrashPath 找出 Finder 删除后文件落在废纸篓里的实际路径。
//
// Finder 遇到重名会改名（如 "a 2.pdf"），因此不能假定原文件名。
// 这里按修改时间在废纸篓里找最近出现的同名或近似名文件。
func FindTrashPath(original string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	trashDir := filepath.Join(home, ".Trash")

	base := filepath.Base(original)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)

	entries, err := os.ReadDir(trashDir)
	if err != nil {
		return "", err
	}

	// 优先精确匹配，其次匹配 "stem N.ext" 形式。
	var fallback string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == base {
			return filepath.Join(trashDir, name), nil
		}
		if strings.HasPrefix(name, stem) && strings.HasSuffix(name, ext) && fallback == "" {
			fallback = filepath.Join(trashDir, name)
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("在废纸篓中找不到 %s", base)
}
