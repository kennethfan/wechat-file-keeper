package undo

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/trash"
)

// record 造一条记录，并在废纸篓里放好对应文件。
func record(t *testing.T, trashDir, originalRel, trashedName, content string) trash.Record {
	t.Helper()
	trashed := filepath.Join(trashDir, trashedName)
	if err := os.MkdirAll(filepath.Dir(trashed), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(trashed, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return trash.Record{
		OriginalPath: originalRel,
		TrashedPath:  trashed,
		Size:         int64(len(content)),
	}
}

func TestApply_RestoresFileToOriginalPath(t *testing.T) {
	base := t.TempDir()
	trashDir := filepath.Join(base, ".Trash")
	original := filepath.Join(base, "msg/video/2024-09/abc.mp4")
	r := record(t, trashDir, original, "abc.mp4", "video bytes")

	res, err := Apply(trash.Manifest{Records: []trash.Record{r}})
	if err != nil {
		t.Fatalf("Apply 不应报错: %v", err)
	}

	if len(res.Restored) != 1 {
		t.Fatalf("应恢复 1 条，实际 %d 条", len(res.Restored))
	}
	data, readErr := os.ReadFile(original)
	if readErr != nil {
		t.Fatalf("文件应回到原路径: %v", readErr)
	}
	if string(data) != "video bytes" {
		t.Fatalf("文件内容应保持不变，实际 %q", data)
	}
	if res.RestoredBytes != r.Size {
		t.Fatalf("恢复字节应为 %d，实际 %d", r.Size, res.RestoredBytes)
	}
}

func TestApply_RecreatesMissingParentDirectory(t *testing.T) {
	base := t.TempDir()
	trashDir := filepath.Join(base, ".Trash")
	// 原路径的父目录完全不存在。
	original := filepath.Join(base, "msg/deeply/nested/2024-09/x.pdf")
	r := record(t, trashDir, original, "x.pdf", "pdf")

	if _, err := Apply(trash.Manifest{Records: []trash.Record{r}}); err != nil {
		t.Fatalf("Apply 不应报错: %v", err)
	}

	if _, statErr := os.Stat(original); statErr != nil {
		t.Fatalf("父目录应被重建且文件已恢复: %v", statErr)
	}
}

func TestApply_SkipsMissingTrashedFile(t *testing.T) {
	base := t.TempDir()
	original := filepath.Join(base, "msg/x.pdf")
	r := trash.Record{
		OriginalPath: original,
		TrashedPath:  filepath.Join(base, ".Trash", "gone.pdf"),
		Size:         10,
	}

	res, err := Apply(trash.Manifest{Records: []trash.Record{r}})
	if err != nil {
		t.Fatalf("Apply 不应报错: %v", err)
	}
	if len(res.Restored) != 0 {
		t.Fatalf("不应恢复任何条目，实际 %d 条", len(res.Restored))
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Reason != "已不在废纸篓" {
		t.Fatalf("应跳过并说明原因，实际 %+v", res.Skipped)
	}
}

func TestApply_DoesNotOverwriteExistingFile(t *testing.T) {
	base := t.TempDir()
	trashDir := filepath.Join(base, ".Trash")
	original := filepath.Join(base, "msg/x.pdf")

	// 原位置已有一个不同的文件。
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(original, []byte("later file"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	r := record(t, trashDir, original, "x.pdf", "original")

	res, err := Apply(trash.Manifest{Records: []trash.Record{r}})
	if err != nil {
		t.Fatalf("Apply 不应报错: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("应跳过，实际 %+v", res.Skipped)
	}

	// 关键：既有文件不得被覆盖。
	data, _ := os.ReadFile(original)
	if string(data) != "later file" {
		t.Fatalf("既有文件被覆盖，实际 %q", data)
	}
}

func TestApply_RestoresInReverseOrder(t *testing.T) {
	base := t.TempDir()
	trashDir := filepath.Join(base, ".Trash")
	first := record(t, trashDir, filepath.Join(base, "msg/a.txt"), "a.txt", "first")
	second := record(t, trashDir, filepath.Join(base, "msg/b.txt"), "b.txt", "second")

	res, err := Apply(trash.Manifest{Records: []trash.Record{first, second}})
	if err != nil {
		t.Fatalf("Apply 不应报错: %v", err)
	}
	if len(res.Restored) != 2 {
		t.Fatalf("应恢复 2 条，实际 %d 条", len(res.Restored))
	}
	// 逆序：后清理的 b 先恢复。
	if res.Restored[0].OriginalPath != second.OriginalPath {
		t.Fatalf("应先恢复后清理的条目，实际首条为 %s", res.Restored[0].OriginalPath)
	}
}

func TestApply_ContinuesAfterPartialFailure(t *testing.T) {
	base := t.TempDir()
	trashDir := filepath.Join(base, ".Trash")
	good := record(t, trashDir, filepath.Join(base, "msg/ok.txt"), "ok.txt", "ok")
	missing := trash.Record{
		OriginalPath: filepath.Join(base, "msg/gone.txt"),
		TrashedPath:  filepath.Join(trashDir, "gone.txt"),
		Size:         5,
	}

	res, err := Apply(trash.Manifest{Records: []trash.Record{good, missing}})
	if err != nil {
		t.Fatalf("Apply 不应报错: %v", err)
	}
	if len(res.Restored) != 1 || len(res.Skipped) != 1 {
		t.Fatalf("应为 1 恢复 / 1 跳过，实际 %d / %d", len(res.Restored), len(res.Skipped))
	}
}

func TestValidate_RejectsEmptyManifest(t *testing.T) {
	if err := Validate(trash.Manifest{}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("空清单应被拒绝，实际: %v", err)
	}
}

func TestValidate_RejectsMissingPaths(t *testing.T) {
	m := trash.Manifest{Records: []trash.Record{{OriginalPath: "/a/b", TrashedPath: ""}}}
	if err := Validate(m); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("缺路径应被拒绝，实际: %v", err)
	}
}

func TestValidate_RejectsRelativePaths(t *testing.T) {
	m := trash.Manifest{Records: []trash.Record{{OriginalPath: "a/b", TrashedPath: "/t/b"}}}
	if err := Validate(m); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("相对路径应被拒绝，实际: %v", err)
	}
}

func TestValidate_RejectsDuplicateOriginalPath(t *testing.T) {
	m := trash.Manifest{Records: []trash.Record{
		{OriginalPath: "/a/b", TrashedPath: "/t/1"},
		{OriginalPath: "/a/b", TrashedPath: "/t/2"},
	}}
	if err := Validate(m); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("重复原路径应被拒绝，实际: %v", err)
	}
}

func TestApply_InvalidManifestTouchesNothing(t *testing.T) {
	// 清单无效时整体拒绝，不做部分执行。
	base := t.TempDir()
	trashDir := filepath.Join(base, ".Trash")
	r := record(t, trashDir, filepath.Join(base, "msg/ok.txt"), "ok.txt", "ok")
	m := trash.Manifest{Records: []trash.Record{r, {OriginalPath: "", TrashedPath: "/x"}}}

	if _, err := Apply(m); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("应返回 ErrInvalidManifest，实际: %v", err)
	}
	// 合法那条也不该被移动。
	if _, statErr := os.Stat(r.TrashedPath); statErr != nil {
		t.Fatal("清单无效时不应移动任何文件")
	}
}
