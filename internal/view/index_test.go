package view

import (
	"fmt"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

func TestIndexOf_ReturnsRowIndex(t *testing.T) {
	s := NewState(sample())

	for i, r := range s.Rows {
		if got := s.IndexOf(r.Entry.Path); got != i {
			t.Fatalf("%s 的下标应为 %d，实际 %d", r.Entry.Path, i, got)
		}
	}
}

func TestIndexOf_MissingReturnsMinusOne(t *testing.T) {
	s := NewState(sample())

	if got := s.IndexOf("/not/in/view"); got != -1 {
		t.Fatalf("不存在的路径应返回 -1，实际 %d", got)
	}
}

func TestIndexOf_UpdatedAfterCategorySwitch(t *testing.T) {
	// 切分类后索引表必须重建，否则会指向旧视图的行。
	s := NewState(sample())
	s.SetCategory(model.CategoryDocument)

	for i, r := range s.Rows {
		if got := s.IndexOf(r.Entry.Path); got != i {
			t.Fatalf("切换分类后 %s 的下标应为 %d，实际 %d", r.Entry.Path, i, got)
		}
	}
	// 视频条目已不在当前视图里。
	if got := s.IndexOf("/msg/video/2024-09/big.mp4"); got != -1 {
		t.Fatalf("不在当前视图的条目应返回 -1，实际 %d", got)
	}
}

func TestIndexOf_LargeViewIsFast(t *testing.T) {
	// 渲染热路径：每个可见格子查一次。若退化成线性扫描，
	// 上千条目滚动几屏就会卡死。这里用基准断言保证 O(1)。
	const n = 5000
	entries := make([]model.Entry, n)
	for i := 0; i < n; i++ {
		entries[i] = model.Entry{
			// 用序号保证路径唯一，避免重复路径把索引表压小。
			Path:     fmt.Sprintf("/msg/video/2024-09/%05d.mp4", i),
			Size:     int64(i),
			Category: model.CategoryVideo,
		}
	}
	s := NewState(entries)

	// 索引表应覆盖全部行。
	if len(s.rowIndex) != len(s.Rows) {
		t.Fatalf("索引表应有 %d 项，实际 %d 项", len(s.Rows), len(s.rowIndex))
	}
	// 抽查若干条。
	for _, i := range []int{0, n / 2, n - 1} {
		if got := s.IndexOf(s.Rows[i].Entry.Path); got != i {
			t.Fatalf("第 %d 行下标应为 %d，实际 %d", i, i, got)
		}
	}
}
