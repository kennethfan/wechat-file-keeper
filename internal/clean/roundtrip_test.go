package clean_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/clean"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/trash"
	"github.com/kennethfan/wechat-file-keeper/internal/undo"
)

// TestRoundTrip_CleanThenUndoRestoresFiles 验证完整回路：
// 清理 → 写清单 → 按清单撤销 → 文件回到原位且内容不变。
//
// 这是票 04 的核心验收标准。全部路径都在临时目录内，
// 绝不触碰真实的 ~/.Trash。
func TestRoundTrip_CleanThenUndoRestoresFiles(t *testing.T) {
	base := t.TempDir()
	msgRoot := filepath.Join(base, "msg")
	trashDir := filepath.Join(base, ".Trash")
	manifestDir := filepath.Join(base, "manifests")
	for _, d := range []string{msgRoot, trashDir, manifestDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}

	// 造两个文件，内容各不相同以便验证恢复后未被串改。
	videoPath := filepath.Join(msgRoot, "video/2024-09/abc.mp4")
	docPath := filepath.Join(msgRoot, "file/2025-02/简历.pdf")
	for path, content := range map[string]string{
		videoPath: "video-content-original",
		docPath:   "pdf-content-original",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
	}

	entries := []model.Entry{
		{Path: videoPath, Size: 22, Category: model.CategoryVideo},
		{Path: docPath, Size: 20, Category: model.CategoryDocument},
	}

	cleaner := trash.NewForTest(trashDir, manifestDir,
		func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) })
	cleaner.Library = filepath.Join(base, "wxid_e2e")

	// 第一步：清理。
	plan := clean.Build(clean.Options{Entries: entries, MsgRoot: msgRoot})
	m, err := clean.Apply(plan, msgRoot, false, cleaner)
	if err != nil {
		t.Fatalf("清理不应报错: %v", err)
	}
	if len(m.Records) != 2 {
		t.Fatalf("应移动 2 个文件，实际 %d 个", len(m.Records))
	}

	// 确认文件确实离开了原目录。
	for _, e := range entries {
		if _, statErr := os.Stat(e.Path); !os.IsNotExist(statErr) {
			t.Fatalf("%s 应已离开原目录", e.Path)
		}
	}

	// 第二步：从磁盘读回清单（验证清单可被真正解析，而非内存对象）。
	latest, err := trash.Latest(manifestDir)
	if err != nil {
		t.Fatalf("应能找到清单: %v", err)
	}
	loaded, err := trash.Load(latest)
	if err != nil {
		t.Fatalf("加载清单失败: %v", err)
	}

	// 第三步：撤销。
	res, err := undo.Apply(loaded)
	if err != nil {
		t.Fatalf("撤销不应报错: %v", err)
	}
	if len(res.Restored) != 2 {
		t.Fatalf("应恢复 2 个文件，实际 %d 个", len(res.Restored))
	}

	// 第四步：验证内容完好。
	want := map[string]string{
		videoPath: "video-content-original",
		docPath:   "pdf-content-original",
	}
	for path, wantContent := range want {
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("%s 应已恢复: %v", path, readErr)
		}
		if string(got) != wantContent {
			t.Fatalf("%s 内容不符，期望 %q，实际 %q", path, wantContent, got)
		}
	}

	// 废纸篓应为空。
	leftover, _ := os.ReadDir(trashDir)
	if len(leftover) != 0 {
		t.Fatalf("废纸篓应为空，实际残留 %d 项", len(leftover))
	}
}

// TestRoundTrip_TwoCleanRoundsUndoOneByOne 验证多轮清理可逐次回滚。
func TestRoundTrip_TwoCleanRoundsUndoOneByOne(t *testing.T) {
	base := t.TempDir()
	msgRoot := filepath.Join(base, "msg")
	trashDir := filepath.Join(base, ".Trash")
	manifestDir := filepath.Join(base, "manifests")
	for _, d := range []string{msgRoot, trashDir, manifestDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}

	clock := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	cleaner := trash.NewForTest(trashDir, manifestDir, func() time.Time { return clock })

	// 两轮清理，时间不同以产生两份清单。
	paths := make([]string, 0, 2)
	for i, name := range []string{"first.mp4", "second.mp4"} {
		path := filepath.Join(msgRoot, "video/2024-09", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
		paths = append(paths, path)

		clock = clock.Add(time.Hour)
		plan := clean.Build(clean.Options{
			Entries: []model.Entry{{Path: path, Size: int64(len(name)), Category: model.CategoryVideo}},
		})
		if _, err := clean.Apply(plan, msgRoot, false, cleaner); err != nil {
			t.Fatalf("第 %d 轮清理失败: %v", i+1, err)
		}
	}

	// 第一份清单应能恢复 first.mp4。
	names, err := os.ReadDir(manifestDir)
	if err != nil {
		t.Fatalf("读取清单目录失败: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("应产生 2 份清单，实际 %d 份", len(names))
	}

	first, err := trash.Load(filepath.Join(manifestDir, names[0].Name()))
	if err != nil {
		t.Fatalf("加载第一份清单失败: %v", err)
	}
	if _, err := undo.Apply(first); err != nil {
		t.Fatalf("撤销第一份清单失败: %v", err)
	}
	if _, statErr := os.Stat(paths[0]); statErr != nil {
		t.Fatalf("first.mp4 应已恢复: %v", statErr)
	}
	// second.mp4 仍在废纸篓，不该被第一份清单恢复。
	if _, statErr := os.Stat(paths[1]); !os.IsNotExist(statErr) {
		t.Fatal("second.mp4 不应被第一份清单恢复")
	}
}
