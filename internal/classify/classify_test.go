package classify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

func TestCategory(t *testing.T) {
	cases := []struct {
		name string
		path string
		want model.Category
	}{
		{"聊天文件 PDF", "/lib/msg/file/2025-02/中智信简介.pdf", model.CategoryDocument},
		{"聊天文件 docx", "/lib/msg/file/2025-09/薪资材料.docx", model.CategoryDocument},
		{"聊天文件 zip", "/lib/msg/file/2025-08/滴滴发票.zip", model.CategoryArchive},
		{"聊天视频", "/lib/msg/video/2024-09/7b395a8ce575ecd4e3e12b0aa53367c4.mp4", model.CategoryVideo},
		{"视频封面图", "/lib/msg/video/2024-09/560ffcd11afc32b990cf3b3da5ad3ac4_thumb.jpg", model.CategoryVideo},
		{"图片缓存缩略", "/lib/msg/attach/5f408c53/2025-08/Img/619f2dc1_t_M.dat", model.CategoryImageCache},
		{"图片缓存高清", "/lib/msg/attach/5f408c53/2025-08/Img/abc_h.dat", model.CategoryImageCache},
		{"未知扩展名", "/lib/msg/file/2025-08/setup.exe", model.CategoryOther},
		{"无扩展名", "/lib/msg/file/2025-08/noext", model.CategoryOther},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Category(c.path); got != c.want {
				t.Fatalf("Category(%q) = %q，期望 %q", c.path, got, c.want)
			}
		})
	}
}

func TestCategory_ImageCacheBeatsJpgExtension(t *testing.T) {
	// attach 下的 .dat 里有一部分实际是 JPEG，但归类必须按缓存走，
	// 否则会被当成聊天文件混进文档类。
	path := "/lib/msg/attach/5f408c53/2025-08/Img/abc_M.dat"
	if got := Category(path); got != model.CategoryImageCache {
		t.Fatalf("attach 下的 .dat 应归为图片缓存，实际 %q", got)
	}
}

func TestCategory_ThumbnailIsVideoAtFileLevel(t *testing.T) {
	// Category 是文件类型层面的判定：缩略图确实是视频的附属产物。
	path := "/lib/msg/video/2024-09/abc_thumb.jpg"
	if got := Category(path); got != model.CategoryVideo {
		t.Fatalf("缩略图在文件类型层面应归为视频，实际 %q", got)
	}
}

func TestCategoryFor_ThumbnailGoesToVideo(t *testing.T) {
	// 封面图归「视频」：它是视频的附属产物，按类型筛选时应与视频一起出现。
	path := "/lib/msg/video/2024-09/abc_thumb.jpg"
	if got := CategoryFor(path); got != model.CategoryVideo {
		t.Fatalf("缩略图应归入视频分类，实际 %q", got)
	}
}

func TestCategoryFor_PlainJpgInVideoDirIsVideo(t *testing.T) {
	// 微信 4.x 新版封面直接是 <基名>.jpg，不带 _thumb 后缀。
	// 实测 msg/video 下有 304 个这样的文件，不能漏成「其他」。
	path := "/lib/msg/video/2026-10/9df687cfc7694058480d6716b983b163.jpg"
	if got := CategoryFor(path); got != model.CategoryVideo {
		t.Fatalf("video 目录下的 jpg 应归入视频，实际 %q", got)
	}
}

func TestCategoryFor_JpgOutsideVideoDirIsNotVideo(t *testing.T) {
	// 只有 video 目录下的图片才算封面，别处的 jpg 不该被误判。
	path := "/lib/msg/file/2025-08/screenshot.jpg"
	if got := CategoryFor(path); got == model.CategoryVideo {
		t.Fatalf("file 目录下的 jpg 不应归为视频，实际 %q", got)
	}
}

func TestCategoryFor_NonThumbnailUnchanged(t *testing.T) {
	// 非缩略图的判定不受影响。
	cases := map[string]model.Category{
		"/lib/msg/video/2024-09/abc.mp4":      model.CategoryVideo,
		"/lib/msg/file/2025-02/x.pdf":         model.CategoryDocument,
		"/lib/msg/file/2025-08/x.zip":         model.CategoryArchive,
		"/lib/msg/attach/a/2025-08/Img/x.dat": model.CategoryImageCache,
	}
	for path, want := range cases {
		if got := CategoryFor(path); got != want {
			t.Fatalf("CategoryFor(%q) = %q，期望 %q", path, got, want)
		}
	}
}

func TestIsImageCache_RequiresAttachParent(t *testing.T) {
	// 只有 attach 目录下的 .dat 才是图片缓存。
	if IsImageCache("/lib/msg/file/2025-08/data.dat") {
		t.Fatal("非 attach 目录下的 .dat 不应算图片缓存")
	}
}

func TestApply_FillsCategoryAndThumbnailFlag(t *testing.T) {
	entries := []model.Entry{
		{Path: "/lib/msg/video/2024-09/abc_thumb.jpg"},
		{Path: "/lib/msg/file/2025-02/x.pdf"},
	}
	Apply(entries)

	if entries[0].Category != model.CategoryVideo || !entries[0].IsThumbnail {
		t.Fatalf("缩略图条目应归视频且 IsThumbnail=true，实际 %+v", entries[0])
	}
	if entries[1].Category != model.CategoryDocument || entries[1].IsThumbnail {
		t.Fatalf("PDF 条目应为文档且非缩略图，实际 %+v", entries[1])
	}
}

func TestCategoryWithHeader_JpegDatIsImage(t *testing.T) {
	// attach 下实际是 JPEG 的 .dat 应归「图片」，不是「图片缓存」。
	dir := t.TempDir()
	path := filepath.Join(dir, "attach", "abc", "2025-08", "Img", "x_M.dat")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	content := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("jpeg-payload")...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	if got := ApplyAndGet(path); got != model.CategoryImage {
		t.Fatalf("JPEG 文件头的 .dat 应归图片，实际 %q", got)
	}
}

func TestCategoryWithHeader_PrivateDatIsCache(t *testing.T) {
	// 微信私有编码（07 08 56 32）应归「图片缓存」。
	dir := t.TempDir()
	path := filepath.Join(dir, "attach", "abc", "2025-08", "Img", "x_h.dat")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	content := append([]byte{0x07, 0x08, 0x56, 0x32}, []byte("private")...)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	if got := ApplyAndGet(path); got != model.CategoryImageCache {
		t.Fatalf("私有编码的 .dat 应归图片缓存，实际 %q", got)
	}
}

func TestIsDecodableImage(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name  string
		magic []byte
		want  bool
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, true},
		{"png", []byte{0x89, 0x50, 0x4E, 0x47}, true},
		{"gif", []byte{0x47, 0x49, 0x46, 0x38}, true},
		{"weixin", []byte{0x07, 0x08, 0x56, 0x32}, false},
	}
	for _, c := range cases {
		p := filepath.Join(dir, c.name+".dat")
		if err := os.WriteFile(p, append(c.magic, []byte("x")...), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
		if got := IsDecodableImage(p); got != c.want {
			t.Fatalf("%s: IsDecodableImage = %v，期望 %v", c.name, got, c.want)
		}
	}
}

// ApplyAndGet 跑一遍 Apply 后取出该路径的分类，便于测试。
func ApplyAndGet(path string) model.Category {
	entries := []model.Entry{{Path: path}}
	Apply(entries)
	return entries[0].Category
}
