package cachestat

import (
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

func TestByYear_GroupsAndSortsDescending(t *testing.T) {
	entries := []model.Entry{
		{Category: model.CategoryImageCache, Month: "2025-08", Size: 100},
		{Category: model.CategoryImageCache, Month: "2026-10", Size: 300},
		{Category: model.CategoryImageCache, Month: "2025-09", Size: 50},
	}
	got := ByYear(entries)

	if len(got) != 2 {
		t.Fatalf("应聚合出 2 年，实际 %d 年: %+v", len(got), got)
	}
	if got[0].Year != "2026" {
		t.Fatalf("第一项应为 2026（降序），实际 %q", got[0].Year)
	}
	if got[0].Bytes != 300 {
		t.Fatalf("2026 年字节应为 300，实际 %d", got[0].Bytes)
	}
	if got[1].Year != "2025" || got[1].Bytes != 150 || got[1].Count != 2 {
		t.Fatalf("2025 年应为 150 字节 / 2 个，实际 %+v", got[1])
	}
}

func TestByYear_IgnoresNonCacheCategories(t *testing.T) {
	entries := []model.Entry{
		{Category: model.CategoryDocument, Month: "2025-08", Size: 999},
		{Category: model.CategoryImageCache, Month: "2025-08", Size: 100},
	}
	got := ByYear(entries)

	if len(got) != 1 || got[0].Bytes != 100 {
		t.Fatalf("只应统计图片缓存，实际 %+v", got)
	}
}

func TestByYear_UnknownMonthGoesLast(t *testing.T) {
	entries := []model.Entry{
		{Category: model.CategoryImageCache, Month: "", Size: 10},
		{Category: model.CategoryImageCache, Month: "2025-08", Size: 100},
	}
	got := ByYear(entries)

	if len(got) != 2 {
		t.Fatalf("应有 2 项，实际 %d", len(got))
	}
	if got[len(got)-1].Year != "未知" {
		t.Fatalf("「未知」应排最后，实际 %q", got[len(got)-1].Year)
	}
}

func TestOrphanTotal_CountsOnlyOrphans(t *testing.T) {
	entries := []model.Entry{
		{IsOrphanThumbnail: true, Size: 100},
		{IsOrphanThumbnail: true, Size: 200},
		{IsOrphanThumbnail: false, Size: 999},
	}
	count, bytes := OrphanTotal(entries)

	if count != 2 || bytes != 300 {
		t.Fatalf("孤儿应为 2 个 / 300 字节，实际 %d 个 / %d 字节", count, bytes)
	}
}

func TestByCategory_SumsPerCategory(t *testing.T) {
	entries := []model.Entry{
		{Category: model.CategoryVideo, Size: 100},
		{Category: model.CategoryVideo, Size: 50},
		{Category: model.CategoryDocument, Size: 30},
	}
	got := ByCategory(entries)

	if got[model.CategoryVideo] != 150 {
		t.Fatalf("视频应为 150，实际 %d", got[model.CategoryVideo])
	}
	if got[model.CategoryDocument] != 30 {
		t.Fatalf("文档应为 30，实际 %d", got[model.CategoryDocument])
	}
}
