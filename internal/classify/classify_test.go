package classify

import (
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

func TestCategory_ThumbnailNotDocument(t *testing.T) {
	// 缩略图是 .jpg，若不先判定会落进「其他」，必须归入视频。
	path := "/lib/msg/video/2024-09/abc_thumb.jpg"
	if got := Category(path); got != model.CategoryVideo {
		t.Fatalf("缩略图应归为视频，实际 %q", got)
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
		t.Fatalf("缩略图条目应为视频且 IsThumbnail=true，实际 %+v", entries[0])
	}
	if entries[1].Category != model.CategoryDocument || entries[1].IsThumbnail {
		t.Fatalf("PDF 条目应为文档且非缩略图，实际 %+v", entries[1])
	}
}
