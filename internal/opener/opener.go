// Package opener 用系统默认程序打开文件，或在 Finder 中定位。
//
// 图片缓存是 `.dat` 扩展名，系统不认识。实测把 `xxx_t_M.dat` 改成
// `.jpg` 后 `file` 能识别为 JPEG、`sips` 能读出尺寸，所以打开这类文件
// 时必须先复制到临时目录并改成正确扩展名。
package opener

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/preview"
)

// TempDirName 是图片缓存复制到临时目录时使用的子目录名。
const TempDirName = "wechat-file-keeper-open"

// Open 用系统默认程序打开文件。
//
// .dat 文件会先复制到临时目录并按文件头改成正确扩展名，
// 否则系统无法识别。私有编码无法打开时返回明确错误。
func Open(e model.Entry) error {
	path := e.Path
	if filepath.Ext(path) == ".dat" {
		converted, err := prepareDat(path)
		if err != nil {
			return err
		}
		path = converted
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("文件不存在或无法访问: %w", err)
	}
	return runOpen(path)
}

// Reveal 在 Finder 中定位到原文件。
//
// 直接对原路径调用 open -R，不需要复制。
func Reveal(e model.Entry) error {
	if _, err := os.Stat(e.Path); err != nil {
		return fmt.Errorf("文件不存在或无法访问: %w", err)
	}
	return runOpen("-R", e.Path)
}

// prepareDat 把 .dat 复制到临时目录并改成正确扩展名。
//
// 扩展名按文件头判定，不靠猜。私有编码返回错误而非静默失败。
func prepareDat(path string) (string, error) {
	ext := preview.ImageExt(path)
	if ext == "" {
		return "", fmt.Errorf("微信私有编码，无法用系统程序打开")
	}

	dir, err := tempDir()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	// 用原文件名的基名 + 正确扩展名，保留可读性。
	base := baseWithoutExt(path)
	target := filepath.Join(dir, base+ext)
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	return target, nil
}

// tempDir 返回并创建临时目录。
func tempDir() (string, error) {
	dir := filepath.Join(os.TempDir(), TempDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("无法创建临时目录: %w", err)
	}
	return dir, nil
}

// baseWithoutExt 返回去掉扩展名的文件名。
func baseWithoutExt(path string) string {
	name := filepath.Base(path)
	return name[:len(name)-len(filepath.Ext(name))]
}

// runOpen 调用系统 open 命令。
//
// 加超时：open 偶尔会因 LaunchServices 卡住，不应让界面跟着挂死。
func runOpen(args ...string) error {
	cmd := exec.Command("open", args...)
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("无法打开: %w", err)
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("打开超时，系统未响应")
	}
}

// CleanTemp 清理打开图片缓存时产生的临时文件。
//
// 应用退出时调用。只删本工具创建的目录，不碰系统临时目录的其他内容。
func CleanTemp() error {
	dir := filepath.Join(os.TempDir(), TempDirName)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("清理临时文件失败: %w", err)
	}
	return nil
}
