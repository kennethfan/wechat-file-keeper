package preview

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// writeWithMagic 写一个带指定文件头的文件，返回路径。
func writeWithMagic(t *testing.T, dir, name string, magic []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	content := append(append([]byte{}, magic...), []byte("padding-data")...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return path
}

func TestDetect_JpegDatIsImage(t *testing.T) {
	dir := t.TempDir()
	path := writeWithMagic(t, dir, "abc_M.dat", jpegMagic)

	if got := Detect(model.Entry{Path: path}); got.Kind != KindImage {
		t.Fatalf("JPEG 文件头的 .dat 应可预览，实际 %q", got.Kind)
	}
}

func TestDetect_PngDatIsImage(t *testing.T) {
	dir := t.TempDir()
	path := writeWithMagic(t, dir, "abc_M.dat", pngMagic)

	if got := Detect(model.Entry{Path: path}); got.Kind != KindImage {
		t.Fatalf("PNG 文件头的 .dat 应可预览，实际 %q", got.Kind)
	}
}

func TestDetect_GifDatIsImage(t *testing.T) {
	dir := t.TempDir()
	path := writeWithMagic(t, dir, "abc_M.dat", gifMagic)

	if got := Detect(model.Entry{Path: path}); got.Kind != KindImage {
		t.Fatalf("GIF 文件头的 .dat 应可预览，实际 %q", got.Kind)
	}
}

func TestDetect_WeixinPrivateDatFallsBackToIcon(t *testing.T) {
	dir := t.TempDir()
	path := writeWithMagic(t, dir, "abc_h.dat", weixinMagic)

	got := Detect(model.Entry{Path: path})

	if got.Kind != KindIcon {
		t.Fatalf("微信私有编码应降级为图标，实际 %q", got.Kind)
	}
	// 仍要说明原因，让用户知道不是工具坏了。
	if got.Reason == "" {
		t.Fatal("应说明为何不能预览")
	}
}

func TestDetect_MissingDatFallsBackToIcon(t *testing.T) {
	got := Detect(model.Entry{Path: "/not/exist/abc.dat"})

	if got.Kind != KindIcon {
		t.Fatalf("读不到文件时应降级为图标，实际 %q", got.Kind)
	}
}

func TestDetect_VideoShowsThumbnail(t *testing.T) {
	for _, path := range []string{
		"/msg/video/2024-09/abc.mp4",
		"/msg/video/2024-09/abc_raw.mp4",
		"/msg/video/2024-09/abc.mov",
	} {
		if got := Detect(model.Entry{Path: path}); got.Kind != KindVideoThumb {
			t.Fatalf("%s 应显示视频封面，实际 %q", path, got.Kind)
		}
	}
}

func TestDetect_StandardImages(t *testing.T) {
	for _, path := range []string{
		"/msg/video/2024-09/abc_thumb.jpg",
		"/msg/file/2025-08/screenshot.png",
		"/msg/file/2025-08/photo.HEIC",
	} {
		if got := Detect(model.Entry{Path: path}); got.Kind != KindImage {
			t.Fatalf("%s 应可预览为图片，实际 %q", path, got.Kind)
		}
	}
}

func TestDetect_NonImageTypesFallBackToIcon(t *testing.T) {
	// 这些类型只显示图标 + 文件名，不做内容级预览。
	for _, path := range []string{
		"/msg/file/2025-02/x.pdf",
		"/msg/file/2025-09/x.docx",
		"/msg/file/2025-08/x.html",
		"/msg/file/2025-08/nginx.conf",
		"/msg/file/2025-08/x.zip",
		"/msg/file/2025-08/setup.exe",
	} {
		if got := Detect(model.Entry{Path: path}); got.Kind != KindIcon {
			t.Fatalf("%s 应降级为图标，实际 %q", path, got.Kind)
		}
	}
}

func TestDetect_ThumbnailFlagIsImage(t *testing.T) {
	e := model.Entry{Path: "/msg/video/2024-09/abc_thumb.jpg", IsThumbnail: true}

	if got := Detect(e); got.Kind != KindImage {
		t.Fatalf("缩略图应可预览为图片，实际 %q", got.Kind)
	}
}
