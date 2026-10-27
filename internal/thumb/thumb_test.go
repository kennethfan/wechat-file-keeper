package thumb

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// writeJPEG 在 dir 下写一张纯色 JPEG，返回路径。
func writeJPEG(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建文件失败: %v", err)
	}
	defer f.Close()

	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("编码 JPEG 失败: %v", err)
	}
	return path
}

func TestPathFor_PlainVideo(t *testing.T) {
	e := model.Entry{Path: "/msg/video/2024-09/abc.mp4"}

	got, ok := PathFor(e)
	if !ok {
		t.Fatal("普通视频应能找到封面路径")
	}
	want := "/msg/video/2024-09/abc_thumb.jpg"
	if got != want {
		t.Fatalf("封面路径应为 %s，实际 %s", want, got)
	}
}

func TestPathFor_RawVideo(t *testing.T) {
	// 微信存 abc_raw.mp4，封面仍是 abc_thumb.jpg。
	e := model.Entry{Path: "/msg/video/2024-09/abc_raw.mp4"}

	got, ok := PathFor(e)
	if !ok {
		t.Fatal("_raw 视频应能找到封面路径")
	}
	want := "/msg/video/2024-09/abc_thumb.jpg"
	if got != want {
		t.Fatalf("封面路径应为 %s，实际 %s", want, got)
	}
}

func TestPathFor_ThumbnailItselfHasNoCover(t *testing.T) {
	// 缩略图条目不需要再找封面，否则会推出 abc_thumb_thumb.jpg。
	e := model.Entry{Path: "/msg/video/2024-09/abc_thumb.jpg", IsThumbnail: true}

	if _, ok := PathFor(e); ok {
		t.Fatal("缩略图条目不应再推导封面")
	}
}

func TestPathFor_NonVideoHasNoCover(t *testing.T) {
	for _, path := range []string{
		"/msg/file/2025-02/x.pdf",
		"/msg/attach/a/2025-08/Img/x.dat",
	} {
		if _, ok := PathFor(model.Entry{Path: path}); ok {
			t.Fatalf("%s 不应推导封面", path)
		}
	}
}

func TestLoader_LoadsExistingThumbnail(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, dir, "abc_thumb.jpg")
	e := model.Entry{Path: filepath.Join(dir, "abc.mp4")}

	l := NewLoader()
	img, ok := l.Load(e)
	if !ok {
		t.Fatal("应加载到封面")
	}
	if img == nil {
		t.Fatal("封面图不应为 nil")
	}
	if img.Bounds().Dx() != 8 {
		t.Fatalf("封面宽度应为 8，实际 %d", img.Bounds().Dx())
	}
}

func TestLoader_MissingThumbnailReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	e := model.Entry{Path: filepath.Join(dir, "abc.mp4")}

	if _, ok := NewLoader().Load(e); ok {
		t.Fatal("缺封面时应返回 false")
	}
}

func TestLoader_CachesResult(t *testing.T) {
	dir := t.TempDir()
	writeJPEG(t, dir, "abc_thumb.jpg")
	e := model.Entry{Path: filepath.Join(dir, "abc.mp4")}

	l := NewLoader()
	first, ok1 := l.Load(e)
	// 删掉磁盘上的文件，若仍能取到说明走的是缓存。
	if err := os.Remove(filepath.Join(dir, "abc_thumb.jpg")); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	second, ok2 := l.Load(e)

	if !ok1 || !ok2 {
		t.Fatal("两次都应成功")
	}
	if first != second {
		t.Fatal("第二次应命中缓存，返回同一张图")
	}
}

func TestLoader_RemembersFailure(t *testing.T) {
	dir := t.TempDir()
	e := model.Entry{Path: filepath.Join(dir, "abc.mp4")}

	l := NewLoader()
	l.Load(e)
	// 失败应被记住，避免每次滚动都重试读盘。
	if _, ok := l.failed[filepath.Join(dir, "abc_thumb.jpg")]; !ok {
		t.Fatal("失败的路径应被记住")
	}
}

func TestLoader_CorruptImageReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	// 写一个扩展名是 jpg 但内容不是图片的文件。
	path := filepath.Join(dir, "abc_thumb.jpg")
	if err := os.WriteFile(path, []byte("not an image"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	e := model.Entry{Path: filepath.Join(dir, "abc.mp4")}

	if _, ok := NewLoader().Load(e); ok {
		t.Fatal("损坏的封面应返回 false 而不是崩溃")
	}
}

func TestLoader_EvictsWhenOverLimit(t *testing.T) {
	dir := t.TempDir()
	l := NewLoader()
	l.MaxEntries = 2

	for _, name := range []string{"a", "b", "c"} {
		writeJPEG(t, dir, name+"_thumb.jpg")
		l.Load(model.Entry{Path: filepath.Join(dir, name+".mp4")})
	}

	// 超过上限后缓存被清空重来，不应无限增长。
	if len(l.cache) > l.MaxEntries {
		t.Fatalf("缓存不应超过上限 %d，实际 %d", l.MaxEntries, len(l.cache))
	}
}

func TestScaleToWidth_ShrinksLargeImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1000, 2000))

	got := ScaleToWidth(src, 300)

	b := got.Bounds()
	if b.Dx() != 300 {
		t.Fatalf("宽度应为 300，实际 %d", b.Dx())
	}
	// 等比：1000x2000 → 300x600。
	if b.Dy() != 600 {
		t.Fatalf("高度应为 600，实际 %d", b.Dy())
	}
}

func TestScaleToWidth_DoesNotUpscale(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 200))

	got := ScaleToWidth(src, 300)

	if got.Bounds().Dx() != 100 {
		t.Fatalf("小图不应被放大，实际宽度 %d", got.Bounds().Dx())
	}
}

func TestScaleToWidth_KeepsAspectRatio(t *testing.T) {
	// 竖版封面是常见情形：304x540。
	src := image.NewRGBA(image.Rect(0, 0, 304, 540))

	got := ScaleToWidth(src, 150)

	b := got.Bounds()
	srcH, srcW, targetW := 540, 304, 150
	wantH := srcH * targetW / srcW
	if b.Dx() != 150 || b.Dy() != wantH {
		t.Fatalf("应为 150x%d，实际 %dx%d", wantH, b.Dx(), b.Dy())
	}
}

func TestScaleToWidth_HandlesDegenerateSize(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 0, 0))

	// 不应 panic 或除零。
	got := ScaleToWidth(src, 150)
	if got == nil {
		t.Fatal("不应返回 nil")
	}
}

func TestLoader_CachesScaledImage(t *testing.T) {
	dir := t.TempDir()
	// 造一张 1000x1000 的大封面。
	path := filepath.Join(dir, "big_thumb.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("编码失败: %v", err)
	}
	f.Close()

	l := NewLoader()
	got, ok := l.Load(model.Entry{Path: filepath.Join(dir, "big.mp4")})
	if !ok {
		t.Fatal("应加载成功")
	}
	// 缓存里应是缩放后的小图，不是原图。
	if got.Bounds().Dx() != TargetWidth {
		t.Fatalf("缓存应为缩放后的 %d 宽，实际 %d", TargetWidth, got.Bounds().Dx())
	}
}
