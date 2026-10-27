package view

import (
	"strings"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/account"
)

// libs 造一组账号库选项。
func libs() []LibraryOption {
	return []LibraryOption{
		{Library: account.Library{WXID: "wxid_small", Path: "/l/small"}, Bytes: 100},
		{Library: account.Library{WXID: "wxid_big", Path: "/l/big"}, Bytes: 5000},
		{Library: account.Library{WXID: "wxid_mid", Path: "/l/mid"}, Bytes: 900},
	}
}

func TestNewLibraries_SortsBySizeDescending(t *testing.T) {
	l := NewLibraries(libs())

	want := []string{"wxid_big", "wxid_mid", "wxid_small"}
	for i, w := range want {
		if l.Options[i].Library.WXID != w {
			t.Fatalf("第 %d 项应为 %s，实际 %s", i, w, l.Options[i].Library.WXID)
		}
	}
}

func TestNewLibraries_SelectsLargestByDefault(t *testing.T) {
	l := NewLibraries(libs())

	cur, ok := l.CurrentOption()
	if !ok {
		t.Fatal("应有当前选中项")
	}
	if cur.Library.WXID != "wxid_big" {
		t.Fatalf("默认应选中最大的账号库，实际 %s", cur.Library.WXID)
	}
}

func TestNewLibraries_EmptyGivesNoCurrent(t *testing.T) {
	l := NewLibraries(nil)

	if _, ok := l.CurrentOption(); ok {
		t.Fatal("空列表不应有当前项")
	}
	if l.Current != -1 {
		t.Fatalf("空列表 Current 应为 -1，实际 %d", l.Current)
	}
	if l.Switchable() {
		t.Fatal("空列表不应可切换")
	}
}

func TestSelect_SwitchesAndReportsChange(t *testing.T) {
	l := NewLibraries(libs())

	// 默认是下标 0（最大），切到下标 2（最小）。
	if !l.Select(2) {
		t.Fatal("切换应返回 true")
	}
	cur, _ := l.CurrentOption()
	if cur.Library.WXID != "wxid_small" {
		t.Fatalf("应切到 wxid_small，实际 %s", cur.Library.WXID)
	}
}

func TestSelect_SameIndexReportsNoChange(t *testing.T) {
	l := NewLibraries(libs())

	// 重新选当前项不应触发重新扫描。
	if l.Select(0) {
		t.Fatal("选中当前项应返回 false，避免无谓重扫")
	}
}

func TestSelect_InvalidIndexIgnored(t *testing.T) {
	l := NewLibraries(libs())

	for _, idx := range []int{-1, 99} {
		if l.Select(idx) {
			t.Fatalf("非法下标 %d 应返回 false", idx)
		}
	}
	// 当前项不应被破坏。
	if cur, _ := l.CurrentOption(); cur.Library.WXID != "wxid_big" {
		t.Fatalf("当前项不应改变，实际 %s", cur.Library.WXID)
	}
}

func TestSwitchable_SingleLibraryIsNot(t *testing.T) {
	l := NewLibraries([]LibraryOption{
		{Library: account.Library{WXID: "wxid_only", Path: "/l/only"}, Bytes: 100},
	})

	if l.Switchable() {
		t.Fatal("只有一个账号库时不应可切换")
	}
	// 但当前项仍然存在，让用户知道识别到了哪个账号。
	if _, ok := l.CurrentOption(); !ok {
		t.Fatal("单账号库仍应有当前项")
	}
}

func TestSwitchable_MultipleIs(t *testing.T) {
	if !NewLibraries(libs()).Switchable() {
		t.Fatal("多个账号库应可切换")
	}
}

func TestLabel_ContainsWXIDAndSize(t *testing.T) {
	o := LibraryOption{
		Library: account.Library{WXID: "wxid_abc", Path: "/l/abc"},
		Bytes:   4 * 1024 * 1024 * 1024,
	}

	label := o.Label()
	if !strings.Contains(label, "wxid_abc") {
		t.Fatalf("标签应含账号标识，实际 %q", label)
	}
	if !strings.Contains(label, "4.00 GB") {
		t.Fatalf("标签应含实占大小，实际 %q", label)
	}
}

func TestLabels_MatchesOptionsOrder(t *testing.T) {
	l := NewLibraries(libs())

	labels := l.Labels()
	if len(labels) != 3 {
		t.Fatalf("应有 3 个标签，实际 %d 个", len(labels))
	}
	if !strings.Contains(labels[0], "wxid_big") {
		t.Fatalf("首个标签应是最小账号库，实际 %q", labels[0])
	}
}
