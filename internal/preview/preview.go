// Package preview 判定条目能否预览。
//
// 只支持两类真实预览：图片直接显示，视频显示封面。
// 其余类型（PDF、docx、文本、压缩包等）一律显示文件类型图标 + 文件名，
// 不做内容级预览——这避免了调外部工具带来的超时、权限与兼容性问题。
package preview

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// Kind 是预览方式。
type Kind string

const (
	// KindImage 直接解码显示图片。
	KindImage Kind = "图片"
	// KindVideoThumb 显示视频封面图。
	KindVideoThumb Kind = "视频封面"
	// KindIcon 只显示文件类型图标与文件名。
	KindIcon Kind = "图标"
)

// 微信私有编码的文件头：`07 08 56 32`（"V2" 标记）。
// 本机 7077 个 `_h.dat` 都是这种格式，无法解码成图片。
var weixinMagic = []byte{0x07, 0x08, 0x56, 0x32}

// 标准图片格式的文件头。
var (
	jpegMagic = []byte{0xFF, 0xD8, 0xFF}
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47}
	gifMagic  = []byte{0x47, 0x49, 0x46}
)

// Result 是一个条目的预览判定结果。
type Result struct {
	// Kind 是预览方式。
	Kind Kind
	// Reason 在无法显示图片时说明原因，供界面提示。
	Reason string
}

// Detect 判定条目该怎么预览。
//
// 图片缓存（.dat）扩展名没有信息量，必须看文件头：
// 标准图片格式可解码，微信私有编码只能显示图标。
func Detect(e model.Entry) Result {
	ext := strings.ToLower(filepath.Ext(e.Path))

	if ext == ".dat" {
		return detectDat(e.Path)
	}

	if e.IsThumbnail || isImageExt(ext) {
		return Result{Kind: KindImage}
	}
	if isVideo(ext) {
		return Result{Kind: KindVideoThumb}
	}
	return Result{Kind: KindIcon}
}

// detectDat 按文件头判定 .dat 的预览方式。
func detectDat(path string) Result {
	header, err := readHeader(path, 4)
	if err != nil {
		return Result{
			Kind:   KindIcon,
			Reason: "无法读取文件内容",
		}
	}

	switch {
	case hasPrefix(header, jpegMagic), hasPrefix(header, pngMagic), hasPrefix(header, gifMagic):
		return Result{Kind: KindImage}
	case hasPrefix(header, weixinMagic):
		return Result{
			Kind:   KindIcon,
			Reason: "微信私有编码，无法预览",
		}
	default:
		return Result{
			Kind:   KindIcon,
			Reason: "未知的图片编码格式",
		}
	}
}

// readHeader 读取文件开头的 n 个字节。
func readHeader(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, n)
	read, err := f.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:read], nil
}

// hasPrefix 判断 data 是否以 prefix 开头。
func hasPrefix(data, prefix []byte) bool {
	if len(data) < len(prefix) {
		return false
	}
	for i := range prefix {
		if data[i] != prefix[i] {
			return false
		}
	}
	return true
}

// isVideo 判断是否为视频扩展名。
func isVideo(ext string) bool {
	switch ext {
	case ".mp4", ".mov", ".m4v", ".avi", ".mkv":
		return true
	}
	return false
}

// isImageExt 判断是否为标准图片扩展名。
func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".tiff", ".heic":
		return true
	}
	return false
}
