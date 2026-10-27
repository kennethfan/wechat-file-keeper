package view

import (
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

func TestDefaultModeFor(t *testing.T) {
	cases := map[model.Category]ViewMode{
		model.CategoryVideo:      ModeGrid,
		model.CategoryImageCache: ModeGrid,
		model.CategoryDocument:   ModeTable,
		model.CategoryArchive:    ModeList,
		model.CategoryOther:      ModeList,
	}
	for cat, want := range cases {
		if got := DefaultModeFor(cat); got != want {
			t.Fatalf("%s 的默认视图应为 %s，实际 %s", cat, want, got)
		}
	}
}

func TestModeFor_UsesDefaultBeforeUserChoice(t *testing.T) {
	s := NewState(sample())

	// 默认分类是视频，应回退到网格。
	if got := s.CurrentMode(); got != ModeGrid {
		t.Fatalf("视频默认应为网格，实际 %s", got)
	}
}

func TestSetCurrentMode_OverridesDefault(t *testing.T) {
	s := NewState(sample())
	s.SetCurrentMode(ModeList)

	if got := s.CurrentMode(); got != ModeList {
		t.Fatalf("用户选择后应为列表，实际 %s", got)
	}
}

func TestSetMode_RememberedPerCategory(t *testing.T) {
	s := NewState(sample())

	// 视频改成列表，文档保持默认表格。
	s.SetCategory(model.CategoryVideo)
	s.SetCurrentMode(ModeList)

	s.SetCategory(model.CategoryDocument)
	if got := s.CurrentMode(); got != ModeTable {
		t.Fatalf("文档应保持默认表格，实际 %s", got)
	}
	s.SetCurrentMode(ModeList)

	// 切回视频仍是用户选的列表。
	s.SetCategory(model.CategoryVideo)
	if got := s.CurrentMode(); got != ModeList {
		t.Fatalf("视频应记住列表，实际 %s", got)
	}
	// 文档也记住了。
	s.SetCategory(model.CategoryDocument)
	if got := s.CurrentMode(); got != ModeList {
		t.Fatalf("文档应记住列表，实际 %s", got)
	}
}

func TestSetMode_OtherCategoryUnaffected(t *testing.T) {
	s := NewState(sample())
	s.SetMode(model.CategoryArchive, ModeGrid)

	// 视频仍是默认网格，档案类被改成网格。
	if got := s.ModeFor(model.CategoryVideo); got != ModeGrid {
		t.Fatalf("视频应为默认网格，实际 %s", got)
	}
	if got := s.ModeFor(model.CategoryArchive); got != ModeGrid {
		t.Fatalf("档案类应为网格，实际 %s", got)
	}
	// 未被设置过的分类仍走默认。
	if got := s.ModeFor(model.CategoryDocument); got != ModeTable {
		t.Fatalf("文档应走默认表格，实际 %s", got)
	}
}

func TestIsValidMode(t *testing.T) {
	for _, m := range AllModes {
		if !IsValidMode(string(m)) {
			t.Fatalf("%s 应为合法模式", m)
		}
	}
	for _, bad := range []string{"", "card", "GRID"} {
		if IsValidMode(bad) {
			t.Fatalf("%q 不应为合法模式", bad)
		}
	}
}

func TestAllModes_HasThreeDistinctModes(t *testing.T) {
	if len(AllModes) != 3 {
		t.Fatalf("应有 3 种模式，实际 %d 种", len(AllModes))
	}
	seen := map[ViewMode]bool{}
	for _, m := range AllModes {
		if seen[m] {
			t.Fatalf("模式 %s 重复", m)
		}
		seen[m] = true
	}
}
