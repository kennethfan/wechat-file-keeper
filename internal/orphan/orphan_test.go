package orphan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// touch 在 dir 下创建空文件，返回其路径。
func touch(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return path
}

func TestMark_ThumbnailWithVideoIsNotOrphan(t *testing.T) {
	dir := "/lib/msg/video/2024-09"
	video := filepath.Join(dir, "abc.mp4")
	thumb := filepath.Join(dir, "abc_thumb.jpg")

	entries := []model.Entry{
		{Path: video, IsThumbnail: false},
		{Path: thumb, IsThumbnail: true},
	}
	Mark(entries)

	if entries[1].IsOrphanThumbnail {
		t.Fatal("有对应视频的缩略图不应判为孤儿")
	}
}

func TestMark_ThumbnailWithoutVideoIsOrphan(t *testing.T) {
	dir := "/lib/msg/video/2024-09"
	entries := []model.Entry{
		{Path: filepath.Join(dir, "gone_thumb.jpg"), IsThumbnail: true},
	}
	Mark(entries)

	if !entries[0].IsOrphanThumbnail {
		t.Fatal("没有对应视频的缩略图应判为孤儿")
	}
}

func TestMark_RawVideoVariantStillMatchesThumbnail(t *testing.T) {
	// 微信存的是 abc_raw.mp4，封面图却是 abc_thumb.jpg。
	dir := "/lib/msg/video/2024-09"
	entries := []model.Entry{
		{Path: filepath.Join(dir, "abc_raw.mp4")},
		{Path: filepath.Join(dir, "abc_thumb.jpg"), IsThumbnail: true},
	}
	Mark(entries)

	if entries[1].IsOrphanThumbnail {
		t.Fatal("_raw 视频的封面图不应判为孤儿")
	}
}

func TestMark_ThumbnailInDifferentDirIsOrphan(t *testing.T) {
	// 视频与封面图不在同一月份目录时，不得跨目录匹配。
	entries := []model.Entry{
		{Path: "/lib/msg/video/2024-08/abc.mp4"},
		{Path: "/lib/msg/video/2024-09/abc_thumb.jpg", IsThumbnail: true},
	}
	Mark(entries)

	if !entries[1].IsOrphanThumbnail {
		t.Fatal("跨目录的同名文件不应匹配，应判为孤儿")
	}
}

func TestMark_IgnoresNonThumbnails(t *testing.T) {
	entries := []model.Entry{
		{Path: "/lib/msg/file/2025-02/x.pdf", IsThumbnail: false},
	}
	Mark(entries)

	if entries[0].IsOrphanThumbnail {
		t.Fatal("非缩略图条目的孤儿标记应保持 false")
	}
}

func TestMark_RealFilesOnDisk(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "keep.mp4")
	touch(t, dir, "keep_thumb.jpg")
	touch(t, dir, "gone_thumb.jpg")

	entries := []model.Entry{
		{Path: filepath.Join(dir, "keep.mp4")},
		{Path: filepath.Join(dir, "keep_thumb.jpg"), IsThumbnail: true},
		{Path: filepath.Join(dir, "gone_thumb.jpg"), IsThumbnail: true},
	}
	Mark(entries)

	if entries[1].IsOrphanThumbnail {
		t.Fatal("keep_thumb.jpg 有对应视频，不应为孤儿")
	}
	if !entries[2].IsOrphanThumbnail {
		t.Fatal("gone_thumb.jpg 无对应视频，应为孤儿")
	}
}
