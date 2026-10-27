package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// writeFile 在 dir 下写一个文件，返回其路径。
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return path
}

func TestRun_EmptyLibraryReturnsEmptyResult(t *testing.T) {
	s := New(t.TempDir())

	res, err := s.Run()
	if err != nil {
		t.Fatalf("空账号库不应报错: %v", err)
	}
	if len(res.Entries) != 0 {
		t.Fatalf("空账号库应产出 0 条，实际 %d 条", len(res.Entries))
	}
	if res.Space.Actual != 0 || res.Space.Nominal != 0 {
		t.Fatalf("空账号库空间应为 0，实际 actual=%d nominal=%d",
			res.Space.Actual, res.Space.Nominal)
	}
}

func TestRun_SkipsEmptyMonthDirectories(t *testing.T) {
	lib := t.TempDir()
	// 微信会留下空月份目录，它们不产生条目。
	if err := os.MkdirAll(filepath.Join(lib, "msg/video/2023-07"), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	writeFile(t, lib, "msg/file/2024-07/a.pdf", "pdf")

	res, err := s_run(t, lib)
	if err != nil {
		t.Fatalf("扫描不应报错: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("应只产出 1 条（空目录不计），实际 %d 条", len(res.Entries))
	}
}

func TestRun_ExtractsMonthFromNestedPath(t *testing.T) {
	lib := t.TempDir()
	// attach 下的层级更深：attach/<指纹>/YYYY-MM/Img/x.dat
	writeFile(t, lib,
		"msg/attach/5f408c53f33d835e9ea0eeb4b35b0e5e/2025-08/Img/abc_t_M.dat", "img")

	res, err := s_run(t, lib)
	if err != nil {
		t.Fatalf("扫描不应报错: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("应产出 1 条，实际 %d 条", len(res.Entries))
	}
	if res.Entries[0].Month != "2025-08" {
		t.Fatalf("月份应为 2025-08，实际 %q", res.Entries[0].Month)
	}
}

func TestSummarize_HardlinksCountedOnce(t *testing.T) {
	lib := t.TempDir()
	original := writeFile(t, lib, "msg/file/2024-07/范春雨.pdf", "same-content")
	linked := filepath.Join(lib, "msg/file/2025-02/范春雨.pdf")
	if err := os.MkdirAll(filepath.Dir(linked), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.Link(original, linked); err != nil {
		t.Skipf("文件系统不支持硬链接，跳过: %v", err)
	}

	res, err := s_run(t, lib)
	if err != nil {
		t.Fatalf("扫描不应报错: %v", err)
	}

	fi, statErr := os.Stat(original)
	if statErr != nil {
		t.Fatalf("stat 失败: %v", statErr)
	}
	one := fi.Size()

	if res.Space.Nominal != one*2 {
		t.Fatalf("名义大小应为 %d（两条路径各算一次），实际 %d", one*2, res.Space.Nominal)
	}
	if res.Space.Actual != one {
		t.Fatalf("实占大小应为 %d（inode 归并），实际 %d", one, res.Space.Actual)
	}
	if res.Space.HardlinkOvercount != one {
		t.Fatalf("硬链接虚增应为 %d，实际 %d", one, res.Space.HardlinkOvercount)
	}
	if res.Space.EntryCount != 2 {
		t.Fatalf("条目数应为 2，实际 %d", res.Space.EntryCount)
	}
	if res.Space.FileCount != 1 {
		t.Fatalf("按 inode 归并后文件数应为 1，实际 %d", res.Space.FileCount)
	}
}

func TestSummarize_NoHardlinksMeansNoOvercount(t *testing.T) {
	entries := []model.Entry{
		{Path: "/a", Inode: 1, Size: 100},
		{Path: "/b", Inode: 2, Size: 200},
	}
	got := Summarize(entries)

	if got.Nominal != 300 || got.Actual != 300 {
		t.Fatalf("无硬链接时两者应相等，实际 nominal=%d actual=%d", got.Nominal, got.Actual)
	}
	if got.HardlinkOvercount != 0 {
		t.Fatalf("无硬链接时虚增应为 0，实际 %d", got.HardlinkOvercount)
	}
}

func TestSummarize_ZeroInodeNotMerged(t *testing.T) {
	// 取不到 inode 的平台会把 Inode 置 0；这些条目必须各自独立。
	entries := []model.Entry{
		{Path: "/a", Inode: 0, Size: 100},
		{Path: "/b", Inode: 0, Size: 200},
	}
	got := Summarize(entries)

	if got.Actual != 300 {
		t.Fatalf("Inode 为 0 的条目不应被归并，实占应为 300，实际 %d", got.Actual)
	}
}

func TestSortBySizeDesc_OrdersLargestFirst(t *testing.T) {
	entries := []model.Entry{
		{Path: "/small", Size: 10},
		{Path: "/huge", Size: 1000},
		{Path: "/mid", Size: 100},
	}
	SortBySizeDesc(entries)

	want := []string{"/huge", "/mid", "/small"}
	for i, w := range want {
		if entries[i].Path != w {
			t.Fatalf("第 %d 项应为 %s，实际 %s", i, w, entries[i].Path)
		}
	}
}

// s_run 是便捷包装，减少测试样板。
func s_run(t *testing.T, lib string) (Result, error) {
	t.Helper()
	return New(lib).Run()
}
