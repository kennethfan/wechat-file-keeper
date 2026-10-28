package view

import (
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// sample 造一组跨分类的条目。
func sample() []model.Entry {
	return []model.Entry{
		{Path: "/msg/video/2024-09/big.mp4", Size: 1000, Month: "2024-09", Category: model.CategoryVideo},
		{Path: "/msg/video/2024-09/small.mp4", Size: 10, Month: "2024-09", Category: model.CategoryVideo},
		{Path: "/msg/file/2025-02/a.pdf", Size: 500, Month: "2025-02", Category: model.CategoryDocument},
		{Path: "/msg/attach/x/2025-08/Img/c.dat", Size: 100, Month: "2025-08", Category: model.CategoryImageCache},
	}
}

func TestNewState_DefaultsToVideoSortedBySize(t *testing.T) {
	s := NewState(sample())

	if s.Category != model.CategoryVideo {
		t.Fatalf("默认分类应为视频，实际 %q", s.Category)
	}
	if len(s.Rows) != 2 {
		t.Fatalf("视频应有 2 行，实际 %d 行", len(s.Rows))
	}
	if s.Rows[0].Entry.Size != 1000 {
		t.Fatalf("首行应为最大文件，实际 %d 字节", s.Rows[0].Entry.Size)
	}
}

func TestSetCategory_FiltersRows(t *testing.T) {
	s := NewState(sample())

	s.SetCategory(model.CategoryDocument)
	if len(s.Rows) != 1 {
		t.Fatalf("文档应有 1 行，实际 %d 行", len(s.Rows))
	}
	if s.Rows[0].Entry.Category != model.CategoryDocument {
		t.Fatalf("行分类应为文档，实际 %q", s.Rows[0].Entry.Category)
	}
}

func TestSelectionSummary_SumsSelectedOnly(t *testing.T) {
	s := NewState(sample())
	s.Toggle(0) // 选中 1000 字节那条

	count, bytes := s.SelectionSummary()
	if count != 1 || bytes != 1000 {
		t.Fatalf("应为 1 项 / 1000 字节，实际 %d 项 / %d 字节", count, bytes)
	}
}

func TestToggle_FlipsSelection(t *testing.T) {
	s := NewState(sample())

	s.Toggle(0)
	if !s.Rows[0].Selected {
		t.Fatal("第一次应选中")
	}
	s.Toggle(0)
	if s.Rows[0].Selected {
		t.Fatal("第二次应取消选中")
	}
}

func TestToggle_IgnoresOutOfRange(t *testing.T) {
	s := NewState(sample())
	// 不应 panic。
	s.Toggle(-1)
	s.Toggle(999)
}

func TestSelectAll_SelectsEveryRow(t *testing.T) {
	s := NewState(sample())

	s.SelectAll(true)
	count, bytes := s.SelectionSummary()
	if count != 2 || bytes != 1010 {
		t.Fatalf("应为 2 项 / 1010 字节，实际 %d 项 / %d 字节", count, bytes)
	}

	s.SelectAll(false)
	if count, _ := s.SelectionSummary(); count != 0 {
		t.Fatalf("取消全选后应为 0 项，实际 %d 项", count)
	}
}

func TestSelectedEntries_ReturnsSelected(t *testing.T) {
	s := NewState(sample())
	s.Toggle(0)

	got := s.SelectedEntries()
	if len(got) != 1 || got[0].Size != 1000 {
		t.Fatalf("应返回被选中的那条，实际 %+v", got)
	}
}

func TestCanClean_RequiresSelection(t *testing.T) {
	s := NewState(sample())

	if s.CanClean() {
		t.Fatal("未选中时不应允许清理")
	}
	s.Toggle(0)
	if !s.CanClean() {
		t.Fatal("选中后应允许清理")
	}
}

func TestCanClean_BlockedWhenWeChatRunning(t *testing.T) {
	s := NewState(sample())
	s.Toggle(0)
	s.WeChatRunning = true

	if s.CanClean() {
		t.Fatal("微信运行时应禁止清理")
	}
	if s.CleanBlockReason() == "" {
		t.Fatal("应给出禁止原因")
	}
}

func TestCanClean_BlockedWhileScanning(t *testing.T) {
	s := NewState(sample())
	s.Toggle(0)
	s.Scanning = true

	if s.CanClean() {
		t.Fatal("扫描中应禁止清理")
	}
}

func TestCleanBlockReason_EmptyWhenAllowed(t *testing.T) {
	s := NewState(sample())
	s.Toggle(0)

	if reason := s.CleanBlockReason(); reason != "" {
		t.Fatalf("可清理时原因应为空，实际 %q", reason)
	}
}

func TestCategoryTotals_SumsAllEntries(t *testing.T) {
	s := NewState(sample())

	totals := s.CategoryTotals()
	if totals[model.CategoryVideo] != 1010 {
		t.Fatalf("视频合计应为 1010，实际 %d", totals[model.CategoryVideo])
	}
	if totals[model.CategoryDocument] != 500 {
		t.Fatalf("文档合计应为 500，实际 %d", totals[model.CategoryDocument])
	}
	if totals[model.CategoryImageCache] != 100 {
		t.Fatalf("图片缓存合计应为 100，实际 %d", totals[model.CategoryImageCache])
	}
}

func TestSetEntries_ResetsRowsAndKeepsCategory(t *testing.T) {
	s := NewState(sample())
	s.SetCategory(model.CategoryDocument)

	s.SetEntries([]model.Entry{
		{Path: "/msg/file/2025-03/b.pdf", Size: 700, Category: model.CategoryDocument},
	})

	if s.Category != model.CategoryDocument {
		t.Fatalf("分类应保持不变，实际 %q", s.Category)
	}
	if len(s.Rows) != 1 || s.Rows[0].Entry.Size != 700 {
		t.Fatalf("应重建为新的文档行，实际 %+v", s.Rows)
	}
}

func TestNewState_EmptyEntriesDoesNotPanic(t *testing.T) {
	s := NewState(nil)

	if len(s.Rows) != 0 {
		t.Fatalf("空输入应无行，实际 %d 行", len(s.Rows))
	}
	if s.CanClean() {
		t.Fatal("空状态不应允许清理")
	}
}

func TestMatchName(t *testing.T) {
	e := model.Entry{Path: "/msg/file/2025-02/中智信简介.pdf"}

	if !MatchName(e, "") {
		t.Fatal("空关键字应匹配全部")
	}
	if !MatchName(e, "简介") {
		t.Fatal("应匹配中文关键字")
	}
	if MatchName(e, "不存在的关键字") {
		t.Fatal("不应匹配")
	}
}

func TestCanClean_AllowsOrphanThumbnailWhileWeChatRunning(t *testing.T) {
	// 孤儿缩略图无风险：微信运行时也应允许清理。
	s := NewState([]model.Entry{
		{Path: "/msg/video/2024-09/gone_thumb.jpg", Size: 10,
			Category: model.CategoryOther, IsOrphanThumbnail: true},
	})
	// 孤儿缩略图归「其他」分类，需先切过去才能看到该行。
	s.SetCategory(model.CategoryOther)
	s.Toggle(0)
	s.WeChatRunning = true

	if !s.CanClean() {
		t.Fatal("微信运行时孤儿缩略图应可清理")
	}
	if reason := s.CleanBlockReason(); reason != "" {
		t.Fatalf("可清理时原因应为空，实际 %q", reason)
	}
}

func TestCanClean_BlocksChatFileWhileWeChatRunning(t *testing.T) {
	s := NewState([]model.Entry{
		{Path: "/msg/file/2025-02/x.pdf", Size: 10, Category: model.CategoryDocument},
	})
	s.SetCategory(model.CategoryDocument)
	s.Toggle(0)
	s.WeChatRunning = true

	if s.CanClean() {
		t.Fatal("微信运行时聊天文件不应可清理")
	}
	if s.CleanBlockReason() == "" {
		t.Fatal("应给出原因")
	}
}

func TestCanClean_BlocksMixedSelectionWhileWeChatRunning(t *testing.T) {
	// 同一分类里既有孤儿缩略图又有其他文件时，整体不放行——
	// 只要选中项里有需要退出微信的，就不能因为部分无风险而放行。
	s := NewState([]model.Entry{
		{Path: "/msg/video/2024-09/gone_thumb.jpg", Size: 10,
			Category: model.CategoryOther, IsOrphanThumbnail: true},
		{Path: "/msg/file/2025-02/setup.exe", Size: 20, Category: model.CategoryOther},
	})
	s.SetCategory(model.CategoryOther)
	s.SelectAll(true)
	s.WeChatRunning = true

	if s.CanClean() {
		t.Fatal("混合选择时不应放行")
	}
}

func TestCanClean_AllowsEverythingWhenWeChatNotRunning(t *testing.T) {
	s := NewState([]model.Entry{
		{Path: "/msg/file/2025-02/x.pdf", Size: 10, Category: model.CategoryDocument},
	})
	s.SetCategory(model.CategoryDocument)
	s.Toggle(0)
	s.WeChatRunning = false

	if !s.CanClean() {
		t.Fatal("微信未运行时应可清理")
	}
}
