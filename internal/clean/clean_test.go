package clean

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/guard"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/trash"
)

// env 是隔离的测试环境：源目录、废纸篓、清单目录都在临时目录内。
type env struct {
	msgRoot     string
	trashDir    string
	manifestDir string
	cleaner     *trash.Cleaner
}

func newEnv(t *testing.T) env {
	t.Helper()
	base := t.TempDir()
	e := env{
		msgRoot:     filepath.Join(base, "msg"),
		trashDir:    filepath.Join(base, ".Trash"),
		manifestDir: filepath.Join(base, "manifests"),
	}
	for _, d := range []string{e.msgRoot, e.trashDir, e.manifestDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}
	e.cleaner = &trash.Cleaner{
		TrashDir:    e.trashDir,
		ManifestDir: e.manifestDir,
		Now:         func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) },
	}
	return e
}

// file 在消息区内创建文件并返回条目。
func (e env) file(t *testing.T, rel, content string, cat model.Category) model.Entry {
	t.Helper()
	path := filepath.Join(e.msgRoot, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return model.Entry{Path: path, Size: int64(len(content)), Category: cat}
}

func TestBuild_SkipsImageCacheByDefault(t *testing.T) {
	cache := model.Entry{Path: "/msg/attach/a/2025-08/Img/x.dat", Size: 100, Category: model.CategoryImageCache}
	video := model.Entry{Path: "/msg/video/2024-09/a.mp4", Size: 200, Category: model.CategoryVideo}

	plan := Build(Options{Entries: []model.Entry{cache, video}})

	if len(plan.Selected) != 1 || plan.Selected[0].Path != video.Path {
		t.Fatalf("默认应只选中视频，实际 %+v", plan.Selected)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Entry.Path != cache.Path {
		t.Fatalf("图片缓存应被跳过，实际 %+v", plan.Skipped)
	}
	if plan.TotalBytes != 200 {
		t.Fatalf("合计字节应为 200，实际 %d", plan.TotalBytes)
	}
}

func TestBuild_IncludesCacheWhenAuthorized(t *testing.T) {
	cache := model.Entry{Path: "/msg/attach/a/2025-08/Img/x.dat", Size: 100, Category: model.CategoryImageCache}

	plan := Build(Options{Entries: []model.Entry{cache}, IncludeCache: true})

	if len(plan.Selected) != 1 {
		t.Fatalf("授权后应选中图片缓存，实际 %+v", plan.Selected)
	}
	if len(plan.Skipped) != 0 {
		t.Fatalf("不应有跳过项，实际 %+v", plan.Skipped)
	}
}

func TestApply_ProceedsWhenWeChatRunning(t *testing.T) {
	// 微信运行不再阻止清理：确认环节在界面层，执行层照常工作。
	e := newEnv(t)
	entry := e.file(t, "video/2024-09/a.mp4", "bytes", model.CategoryVideo)

	m, err := Apply(Plan{Selected: []model.Entry{entry}}, e.msgRoot, true, e.cleaner)
	if err != nil {
		t.Fatalf("微信运行时也应能清理，实际: %v", err)
	}
	if len(m.Records) != 1 {
		t.Fatalf("应有 1 条记录，实际 %d 条", len(m.Records))
	}
	if _, statErr := os.Stat(entry.Path); !os.IsNotExist(statErr) {
		t.Fatal("文件应已移走")
	}
}

func TestApply_RefusesOutOfBoundsBeforeTouchingAnything(t *testing.T) {
	e := newEnv(t)
	inside := e.file(t, "video/2024-09/ok.mp4", "bytes", model.CategoryVideo)
	outside := model.Entry{Path: "/Users/k/Desktop/重要.pdf", Size: 10, Category: model.CategoryDocument}

	_, err := Apply(Plan{Selected: []model.Entry{inside, outside}}, e.msgRoot, false, e.cleaner)
	if !errors.Is(err, guard.ErrOutsideRoot) {
		t.Fatalf("越界应被拒绝，实际: %v", err)
	}
	// 关键：越界检查先于任何移动，合法的那条也不得被删。
	if _, statErr := os.Stat(inside.Path); statErr != nil {
		t.Fatal("整体拒绝时合法文件不应被移动")
	}
}

func TestApply_MovesSelectedFiles(t *testing.T) {
	e := newEnv(t)
	entry := e.file(t, "video/2024-09/a.mp4", "video bytes", model.CategoryVideo)

	m, err := Apply(Plan{Selected: []model.Entry{entry}}, e.msgRoot, false, e.cleaner)
	if err != nil {
		t.Fatalf("Apply 不应报错: %v", err)
	}
	if len(m.Records) != 1 {
		t.Fatalf("应有 1 条记录，实际 %d", len(m.Records))
	}
	if _, statErr := os.Stat(filepath.Join(e.trashDir, "a.mp4")); statErr != nil {
		t.Fatalf("文件应在废纸篓: %v", statErr)
	}
}

func TestApply_EmptyPlanSucceeds(t *testing.T) {
	e := newEnv(t)

	m, err := Apply(Plan{}, e.msgRoot, false, e.cleaner)
	if err != nil {
		t.Fatalf("空计划不应报错: %v", err)
	}
	if len(m.Records) != 0 {
		t.Fatalf("不应有记录，实际 %d", len(m.Records))
	}
}

func TestDescribe_MentionsSkipped(t *testing.T) {
	cache := model.Entry{Path: "/msg/attach/a/x.dat", Size: 100, Category: model.CategoryImageCache}
	plan := Build(Options{Entries: []model.Entry{cache}})

	desc := plan.Describe()
	if desc == "" {
		t.Fatal("摘要不应为空")
	}
	if !contains(desc, "跳过") {
		t.Fatalf("摘要应说明跳过项，实际: %s", desc)
	}
}

// contains 是简单的子串判断，避免引入额外依赖。
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
