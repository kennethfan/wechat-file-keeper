package bridge

import (
	"encoding/base64"
	"image"
	"image/color"
	"strings"
	"testing"
)

// solidImage 造一张指定尺寸的纯色图。
func solidImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 120, G: 180, B: 90, A: 255})
		}
	}
	return img
}

func TestScaleDown_ShrinksLargeImage(t *testing.T) {
	got := scaleDown(solidImage(1080, 1920), 336)

	b := got.Bounds()
	if b.Dy() != 336 {
		t.Fatalf("长边应缩到 336，实际 %dx%d", b.Dx(), b.Dy())
	}
	if b.Dx() != 189 { // 1080 * 336 / 1920 = 189
		t.Fatalf("短边应为 189，实际 %d", b.Dx())
	}
}

func TestScaleDown_DoesNotUpscale(t *testing.T) {
	got := scaleDown(solidImage(100, 80), 336)

	if got.Bounds().Dx() != 100 {
		t.Fatalf("小图不应放大，实际宽度 %d", got.Bounds().Dx())
	}
}

func TestEncodeDataURL_UsesJPEG(t *testing.T) {
	url := encodeDataURL(solidImage(400, 400))

	if !strings.HasPrefix(url, "data:image/jpeg;base64,") {
		t.Fatalf("应输出 JPEG data URL，实际前缀 %q", url[:40])
	}
}

func TestEncodeDataURL_ShrinksPayload(t *testing.T) {
	// 1080p 的图编码后应远小于原始像素量
	url := encodeDataURL(solidImage(1080, 1920))

	raw := strings.TrimPrefix(url, "data:image/jpeg;base64,")
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("base64 解码失败: %v", err)
	}
	// 缩放后是 336 长边，纯色 JPEG 应该只有几 KB
	if len(data) > 100*1024 {
		t.Fatalf("编码后 %d 字节，超过 100KB，说明没缩放或没压缩", len(data))
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:          "0 B",
		512:        "512 B",
		1024:       "1.0 KB",
		1048576:    "1.0 MB",
		1073741824: "1.00 GB",
	}
	for in, want := range cases {
		if got := humanBytes(in); got != want {
			t.Fatalf("humanBytes(%d) = %q，期望 %q", in, got, want)
		}
	}
}
