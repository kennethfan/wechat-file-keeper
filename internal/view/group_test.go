package view

import (
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// monthSample 造一组跨月份的条目，组内已按大小降序。
func monthSample() []model.Entry {
	return []model.Entry{
		{Path: "/msg/video/2026-08/a.mp4", Size: 500, Month: "2026-08", Category: model.CategoryVideo},
		{Path: "/msg/video/2026-08/b.mp4", Size: 100, Month: "2026-08", Category: model.CategoryVideo},
		{Path: "/msg/video/2025-02/c.mp4", Size: 300, Month: "2025-02", Category: model.CategoryVideo},
		{Path: "/msg/video/nomonth/d.mp4", Size: 50, Month: "", Category: model.CategoryVideo},
	}
}

func TestGroupByMonth_GroupsAndSortsDescending(t *testing.T) {
	s := NewState(monthSample())
	groups := s.Groups()

	if len(groups) != 3 {
		t.Fatalf("应有 3 个组，实际 %d 个", len(groups))
	}
	if groups[0].Month != "2026-08" {
		t.Fatalf("首组应为 2026-08，实际 %q", groups[0].Month)
	}
	if groups[1].Month != "2025-02" {
		t.Fatalf("次组应为 2025-02，实际 %q", groups[1].Month)
	}
	if groups[2].Month != UnknownMonth {
		t.Fatalf("末组应为未知月份，实际 %q", groups[2].Month)
	}
}

func TestGroupByMonth_ComputesCountAndBytes(t *testing.T) {
	s := NewState(monthSample())
	groups := s.Groups()

	// 2026-08 有 2 条，合计 600。
	if groups[0].Count() != 2 {
		t.Fatalf("2026-08 应有 2 条，实际 %d 条", groups[0].Count())
	}
	if groups[0].Bytes() != 600 {
		t.Fatalf("2026-08 合计应为 600，实际 %d", groups[0].Bytes())
	}
}

func TestGroupByMonth_PreservesRowOrderWithinGroup(t *testing.T) {
	s := NewState(monthSample())
	groups := s.Groups()

	// 组内应保持按大小降序（NewState 已排过序）。
	if groups[0].Rows[0].Entry.Size != 500 {
		t.Fatalf("组内首条应为 500，实际 %d", groups[0].Rows[0].Entry.Size)
	}
}

func TestSelection_ThreeStates(t *testing.T) {
	s := NewState(monthSample())

	// Rows 是全局按大小降序，2026-08 的两条在下标 0 和 2。
	if s.Rows[0].Entry.Month != "2026-08" || s.Rows[2].Entry.Month != "2026-08" {
		t.Fatalf("前置假设不成立：%s / %s", s.Rows[0].Entry.Month, s.Rows[2].Entry.Month)
	}

	// 未选。
	if s.Groups()[0].Selection() != SelectionNone {
		t.Fatal("未选时应为 SelectionNone")
	}

	// 部分选。
	s.Toggle(0)
	if s.Groups()[0].Selection() != SelectionPartial {
		t.Fatal("选一条时应为 SelectionPartial")
	}

	// 全选该月剩余的那条。
	s.Toggle(2)
	if s.Groups()[0].Selection() != SelectionAll {
		t.Fatal("全选时应为 SelectionAll")
	}
}

func TestSelectGroup_SelectsWholeMonth(t *testing.T) {
	s := NewState(monthSample())

	s.SelectGroup("2026-08", true)

	count, bytes := s.SelectionSummary()
	if count != 2 || bytes != 600 {
		t.Fatalf("应选中 2026-08 的 2 条 / 600 字节，实际 %d 条 / %d 字节", count, bytes)
	}
}

func TestSelectGroup_DeselectsWholeMonth(t *testing.T) {
	s := NewState(monthSample())
	s.SelectGroup("2026-08", true)
	s.SelectGroup("2026-08", false)

	if count, _ := s.SelectionSummary(); count != 0 {
		t.Fatalf("取消后应为 0 条，实际 %d 条", count)
	}
}

func TestSelectGroup_DoesNotTouchOtherMonths(t *testing.T) {
	s := NewState(monthSample())

	s.SelectGroup("2026-08", true)

	// 2025-02 的条目不应被选中。
	for _, r := range s.Rows {
		if r.Entry.Month == "2025-02" && r.Selected {
			t.Fatal("其他月份的条目不应被选中")
		}
	}
}

func TestToggleGroup_FlipsCollapsed(t *testing.T) {
	s := NewState(monthSample())

	if s.GroupCollapsed("2026-08") {
		t.Fatal("默认应展开")
	}
	s.ToggleGroup("2026-08")
	if !s.GroupCollapsed("2026-08") {
		t.Fatal("切换后应收起")
	}
	s.ToggleGroup("2026-08")
	if s.GroupCollapsed("2026-08") {
		t.Fatal("再次切换后应展开")
	}
}

func TestToggleGroup_SurvivesCategorySwitch(t *testing.T) {
	entries := append(monthSample(),
		model.Entry{Path: "/msg/file/2026-08/x.pdf", Size: 10, Month: "2026-08", Category: model.CategoryDocument},
	)
	s := NewState(entries)
	s.ToggleGroup("2026-08")

	// 切到文档再切回视频。
	s.SetCategory(model.CategoryDocument)
	s.SetCategory(model.CategoryVideo)

	if !s.GroupCollapsed("2026-08") {
		t.Fatal("折叠状态应在切换分类后保留")
	}
}

func TestGroups_ReflectsCollapsedState(t *testing.T) {
	s := NewState(monthSample())
	s.ToggleGroup("2026-08")

	groups := s.Groups()
	if !groups[0].Collapsed {
		t.Fatal("Groups() 应反映折叠状态")
	}
}

func TestGroupByMonth_EmptyRowsGivesNoGroups(t *testing.T) {
	s := NewState(nil)

	if got := s.Groups(); len(got) != 0 {
		t.Fatalf("空视图不应有组，实际 %d 个", len(got))
	}
}

func TestGroupByMonth_UnknownMonthGroupSelectable(t *testing.T) {
	s := NewState(monthSample())

	s.SelectGroup(UnknownMonth, true)

	count, _ := s.SelectionSummary()
	if count != 1 {
		t.Fatalf("未知月份组应选中 1 条，实际 %d 条", count)
	}
}
