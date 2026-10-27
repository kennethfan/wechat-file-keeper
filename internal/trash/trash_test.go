package trash

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// fixedNow 让测试时间可预期，避免依赖真实时钟。
func fixedNow() time.Time {
	return time.Date(2026, 10, 27, 18, 30, 0, 0, time.UTC)
}

// setup 造一个隔离的沙盒：源目录与废纸篓都在临时目录里，
// 保证测试绝不触碰真实的 ~/.Trash。
func setup(t *testing.T) (src, trashDir, manifestDir string, c *Cleaner) {
	t.Helper()
	base := t.TempDir()
	src = filepath.Join(base, "msg")
	trashDir = filepath.Join(base, ".Trash")
	manifestDir = filepath.Join(base, "manifests")

	for _, d := range []string{src, trashDir, manifestDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}
	c = &Cleaner{
		TrashDir:    trashDir,
		ManifestDir: manifestDir,
		Library:     filepath.Join(base, "wxid_test"),
		Now:         fixedNow,
	}
	return src, trashDir, manifestDir, c
}

// makeFile 在 src 下创建文件，返回条目。
func makeFile(t *testing.T, src, rel, content string) model.Entry {
	t.Helper()
	path := filepath.Join(src, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return model.Entry{
		Path:     path,
		Size:     int64(len(content)),
		Category: model.CategoryVideo,
	}
}

func TestMove_FileLeavesOriginalAndLandsInTrash(t *testing.T) {
	src, trashDir, _, c := setup(t)
	e := makeFile(t, src, "video/2024-09/abc.mp4", "video bytes")

	m, err := c.Move([]model.Entry{e})
	if err != nil {
		t.Fatalf("Move 不应报错: %v", err)
	}

	if _, statErr := os.Stat(e.Path); !os.IsNotExist(statErr) {
		t.Fatal("文件应已离开原目录")
	}
	if _, statErr := os.Stat(filepath.Join(trashDir, "abc.mp4")); statErr != nil {
		t.Fatalf("文件应出现在废纸篓: %v", statErr)
	}
	if len(m.Records) != 1 {
		t.Fatalf("清单应有 1 条记录，实际 %d 条", len(m.Records))
	}
	if m.TotalBytes() != e.Size {
		t.Fatalf("清单总字节应为 %d，实际 %d", e.Size, m.TotalBytes())
	}
}

func TestMove_WritesManifestFile(t *testing.T) {
	src, _, manifestDir, c := setup(t)
	e := makeFile(t, src, "file/2025-02/中智信简介.pdf", "pdf")

	if _, err := c.Move([]model.Entry{e}); err != nil {
		t.Fatalf("Move 不应报错: %v", err)
	}

	latest, err := Latest(manifestDir)
	if err != nil {
		t.Fatalf("应能找到清单: %v", err)
	}
	m, err := Load(latest)
	if err != nil {
		t.Fatalf("加载清单失败: %v", err)
	}
	if len(m.Records) != 1 {
		t.Fatalf("清单应有 1 条记录，实际 %d 条", len(m.Records))
	}
	if m.Records[0].OriginalPath != e.Path {
		t.Fatalf("记录的原路径应为 %s，实际 %s", e.Path, m.Records[0].OriginalPath)
	}
	if m.Records[0].Category != string(model.CategoryVideo) {
		t.Fatalf("记录应保留分类，实际 %q", m.Records[0].Category)
	}
}

func TestMove_RenamesOnNameCollision(t *testing.T) {
	src, trashDir, _, c := setup(t)
	// 废纸篓里已有一个同名文件。
	if err := os.WriteFile(filepath.Join(trashDir, "abc.mp4"), []byte("old"), 0o644); err != nil {
		t.Fatalf("预置文件失败: %v", err)
	}
	e := makeFile(t, src, "video/2024-09/abc.mp4", "new bytes")

	m, err := c.Move([]model.Entry{e})
	if err != nil {
		t.Fatalf("Move 不应报错: %v", err)
	}

	// 既有文件不得被覆盖。
	old, readErr := os.ReadFile(filepath.Join(trashDir, "abc.mp4"))
	if readErr != nil {
		t.Fatalf("既有文件应仍存在: %v", readErr)
	}
	if string(old) != "old" {
		t.Fatalf("既有文件被覆盖了，内容为 %q", old)
	}

	// 新文件应改名为 "abc (2).mp4"，且清单记录的是改名后的路径。
	want := filepath.Join(trashDir, "abc (2).mp4")
	if _, statErr := os.Stat(want); statErr != nil {
		t.Fatalf("新文件应改名为 %s: %v", want, statErr)
	}
	if m.Records[0].TrashedPath != want {
		t.Fatalf("清单应记录改名后的路径 %s，实际 %s", want, m.Records[0].TrashedPath)
	}
}

func TestMove_ContinuesAfterSingleFailure(t *testing.T) {
	src, _, _, c := setup(t)
	good := makeFile(t, src, "file/2025-02/ok.pdf", "ok")
	// 这个条目的父目录不存在，stat 会失败。
	bad := model.Entry{Path: filepath.Join(src, "missing/dir/x.pdf"), Size: 1}

	m, err := c.Move([]model.Entry{bad, good})
	if err == nil {
		t.Fatal("有失败条目时应返回错误")
	}
	if len(m.Records) != 1 {
		t.Fatalf("成功的条目应仍被记录，实际 %d 条", len(m.Records))
	}
	if m.Records[0].OriginalPath != good.Path {
		t.Fatalf("记录应是成功的那条，实际 %s", m.Records[0].OriginalPath)
	}
}

func TestMove_EmptyEntriesWritesNoManifest(t *testing.T) {
	_, _, manifestDir, c := setup(t)

	m, err := c.Move(nil)
	if err != nil {
		t.Fatalf("空输入不应报错: %v", err)
	}
	if len(m.Records) != 0 {
		t.Fatalf("不应有记录，实际 %d 条", len(m.Records))
	}
	if _, err := Latest(manifestDir); err == nil {
		t.Fatal("空清理不应写出清单")
	}
}

func TestMove_NoUnlink(t *testing.T) {
	// 反向验证：清理后文件必须仍存在于废纸篓，而不是被删除。
	src, trashDir, _, c := setup(t)
	e := makeFile(t, src, "video/2024-09/keep.mp4", "must survive")

	if _, err := c.Move([]model.Entry{e}); err != nil {
		t.Fatalf("Move 不应报错: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(trashDir, "keep.mp4"))
	if err != nil {
		t.Fatalf("文件内容应完整保留在废纸篓: %v", err)
	}
	if string(data) != "must survive" {
		t.Fatalf("文件内容被改动，实际 %q", data)
	}
}

func TestManifestName_ReplacesIllegalChars(t *testing.T) {
	got := ManifestName("2026-10-27T18:30:00+08:00")
	want := "clean-2026-10-27T18-30-00+08-00.json"
	if got != want {
		t.Fatalf("ManifestName = %q，期望 %q", got, want)
	}
}

func TestLatest_PicksNewestManifest(t *testing.T) {
	_, _, manifestDir, _ := setup(t)
	for _, name := range []string{
		"clean-2026-10-27T10-00-00Z.json",
		"clean-2026-10-27T18-00-00Z.json",
		"clean-2026-10-26T09-00-00Z.json",
	} {
		if err := os.WriteFile(filepath.Join(manifestDir, name), []byte("{}"), 0o644); err != nil {
			t.Fatalf("写清单失败: %v", err)
		}
	}

	got, err := Latest(manifestDir)
	if err != nil {
		t.Fatalf("Latest 不应报错: %v", err)
	}
	if filepath.Base(got) != "clean-2026-10-27T18-00-00Z.json" {
		t.Fatalf("应取最新清单，实际 %s", filepath.Base(got))
	}
}

func TestLatest_EmptyDirReportsError(t *testing.T) {
	_, _, manifestDir, _ := setup(t)

	if _, err := Latest(manifestDir); err == nil {
		t.Fatal("空清单目录应报错")
	}
}

func TestLoad_CorruptManifestReportsError(t *testing.T) {
	_, _, manifestDir, _ := setup(t)
	bad := filepath.Join(manifestDir, "clean-bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("写坏清单失败: %v", err)
	}

	if _, err := Load(bad); err == nil {
		t.Fatal("损坏的清单应报错")
	}
}
