package bridge

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
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

// thumbMaxSide 是缩略图的最长边像素。
//
// 卡片里的缩略图显示区约 168×72，按 2x 屏幕取 336 足够清晰。
// 微信原始封面可达 1080×1920，不缩放会让单张 base64 超过 1MB。
const thumbMaxSide = 336

// encodeDataURL 把图片缩放后编码成 base64 data URL。
//
// 两个关键优化：
//  1. 先缩放到 thumbMaxSide 以内再编码。原始图片动辄 1080p，
//     缩放后数据量能降到 1/10 以下。
//  2. 用 JPEG 而不是 PNG。PNG 是无损格式，对照片类图像压缩率极差，
//     同样内容 JPEG 只有 PNG 的 1/5 左右。
func encodeDataURL(img image.Image) string {
	small := scaleDown(img, thumbMaxSide)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, small, &jpeg.Options{Quality: 82}); err != nil {
		// JPEG 编码失败（如含透明通道的极端情况）退回 PNG
		buf.Reset()
		if err := png.Encode(&buf, small); err != nil {
			return ""
		}
		return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// scaleDown 把图片等比缩放到最长边不超过 maxSide。
//
// 已足够小则原样返回。用最近邻采样：缩略图不需要高质量重采样，
// 最近邻比双线性快数倍，而卡片里看不出差别。
func scaleDown(src image.Image, maxSide int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	if w <= maxSide && h <= maxSide {
		return src
	}

	// 按较长边算缩放比
	var nw, nh int
	if w >= h {
		nw = maxSide
		nh = h * maxSide / w
	} else {
		nh = maxSide
		nw = w * maxSide / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xStep := float64(w) / float64(nw)
	yStep := float64(h) / float64(nh)
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + int(float64(y)*yStep)
		for x := 0; x < nw; x++ {
			sx := b.Min.X + int(float64(x)*xStep)
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
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
