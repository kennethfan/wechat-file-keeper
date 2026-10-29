package bridge

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
)

// humanBytes 把字节数格式化成可读文本。
//
// 用 1024 进制，与 Finder 的显示习惯一致。
func humanBytes(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n < 0:
		return "0 B"
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	case n < gb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	}
}

// encodeDataURL 把图片编码成 base64 data URL，供前端 <img> 直接使用。
//
// 统一转 PNG：微信缓存里有 JPEG/PNG 混合，统一编码让前端不用判断类型。
func encodeDataURL(img image.Image) string {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// decodeFile 解码图片文件。
func decodeFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("无法解码图片: %w", err)
	}
	return img, nil
}

// previewIsThumb 判断路径是否是缩略图。
func previewIsThumb(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), "_thumb.jpg")
}
