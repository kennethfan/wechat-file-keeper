package view

import (
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

func TestSetSort_BySizeDescending(t *testing.T) {
	s := NewState(sample())
	s.SetSort(SortBySize, true)

	for i := 1; i < len(s.Rows); i++ {
		if s.Rows[i-1].Entry.Size < s.Rows[i].Entry.Size {
			t.Fatalf("应按大小降序，第 %d 行 %d < 第 %d 行 %d",
				i-1, s.Rows[i-1].Entry.Size, i, s.Rows[i].Entry.Size)
		}
	}
}

func TestSetSort_BySizeAscending(t *testing.T) {
	s := NewState(sample())
	s.SetSort(SortBySize, false)

	for i := 1; i < len(s.Rows); i++ {
		if s.Rows[i-1].Entry.Size > s.Rows[i].Entry.Size {
			t.Fatalf("应按大小升序，第 %d 行 %d > 第 %d 行 %d",
				i-1, s.Rows[i-1].Entry.Size, i, s.Rows[i].Entry.Size)
		}
	}
}

func TestSetSort_ByMonth(t *testing.T) {
	s := NewState(sample())
	s.SetSort(SortByMonth, true)

	for i := 1; i < len(s.Rows); i++ {
		if s.Rows[i-1].Entry.Month < s.Rows[i].Entry.Month {
			t.Fatalf("应按月份降序，第 %d 行 %q < 第 %d 行 %q",
				i-1, s.Rows[i-1].Entry.Month, i, s.Rows[i].Entry.Month)
		}
	}
}

func TestSetSort_ByName(t *testing.T) {
	s := NewState(sample())
	s.SetSort(SortByName, false)

	for i := 1; i < len(s.Rows); i++ {
		if s.Rows[i-1].Entry.Path > s.Rows[i].Entry.Path {
			t.Fatalf("应按文件名升序，第 %d 行 %q > 第 %d 行 %q",
				i-1, s.Rows[i-1].Entry.Path, i, s.Rows[i].Entry.Path)
		}
	}
}

func TestSetSort_RebuildsIndex(t *testing.T) {
	// 这是关键正确性检查：排序打乱行顺序后，索引表若不重建，
	// IndexOf 会返回错误下标，导致勾选到别的条目。
	s := NewState(sample())
	s.SetSort(SortBySize, false) // 升序，与初始的降序相反

	for i, r := range s.Rows {
		if got := s.IndexOf(r.Entry.Path); got != i {
			t.Fatalf("排序后 %s 的下标应为 %d，实际 %d", r.Entry.Path, i, got)
		}
	}
}

func TestSetSort_PreservesSelection(t *testing.T) {
	s := NewState(sample())
	// 选中 1000 字节那条。
	s.Toggle(0)
	target := s.Rows[0].Entry.Path

	s.SetSort(SortBySize, false) // 改升序

	// 被选中的条目应仍然被选中，只是位置变了。
	idx := s.IndexOf(target)
	if idx < 0 {
		t.Fatal("条目应仍在视图中")
	}
	if !s.Rows[idx].Selected {
		t.Fatal("排序不应丢失勾选状态")
	}
}

func TestSetSort_EmptyViewDoesNotPanic(t *testing.T) {
	s := NewState(nil)
	s.SetSort(SortBySize, true)

	if len(s.Rows) != 0 {
		t.Fatalf("空视图不应产生行，实际 %d 行", len(s.Rows))
	}
}

func TestSetSort_StableForEqualValues(t *testing.T) {
	entries := []model.Entry{
		{Path: "/a.mp4", Size: 100, Month: "2024-09", Category: model.CategoryVideo},
		{Path: "/b.mp4", Size: 100, Month: "2024-09", Category: model.CategoryVideo},
		{Path: "/c.mp4", Size: 100, Month: "2024-09", Category: model.CategoryVideo},
	}
	s := NewState(entries)
	s.SetSort(SortBySize, true)

	// 同大小时保持原顺序。
	want := []string{"/a.mp4", "/b.mp4", "/c.mp4"}
	for i, w := range want {
		if s.Rows[i].Entry.Path != w {
			t.Fatalf("同值时应保持稳定顺序，第 %d 项应为 %s，实际 %s",
				i, w, s.Rows[i].Entry.Path)
		}
	}
}
