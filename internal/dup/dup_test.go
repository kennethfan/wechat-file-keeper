package dup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// write 在 dir 下写文件，返回路径与字节数。
func write(t *testing.T, dir, name, content string) (string, int64) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}
	return path, int64(len(content))
}

func TestFind_SameContentDifferentInodeIsDuplicate(t *testing.T) {
	dir := t.TempDir()
	p1, size := write(t, dir, "a/简历.pdf", "same bytes here")
	p2, _ := write(t, dir, "b/简历(1).pdf", "same bytes here")

	entries := []model.Entry{
		{Path: p1, Inode: 1, Size: size},
		{Path: p2, Inode: 2, Size: size},
	}
	groups, err := Finder{}.Find(entries)
	if err != nil {
		t.Fatalf("Find 不应报错: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("应找到 1 组重复，实际 %d 组", len(groups))
	}
	if groups[0].Reclaimable() != size {
		t.Fatalf("可回收应为 %d 字节，实际 %d", size, groups[0].Reclaimable())
	}
}

func TestFind_SameSizeDifferentContentIsNotDuplicate(t *testing.T) {
	// 这是关键反例：大小相同但内容不同，绝不能判为重复。
	dir := t.TempDir()
	p1, size := write(t, dir, "a.txt", "aaaa")
	p2, _ := write(t, dir, "b.txt", "bbbb")

	entries := []model.Entry{
		{Path: p1, Inode: 1, Size: size},
		{Path: p2, Inode: 2, Size: size},
	}
	groups, err := Finder{}.Find(entries)
	if err != nil {
		t.Fatalf("Find 不应报错: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("同大小不同内容不应判为重复，实际找到 %d 组", len(groups))
	}
}

func TestFind_HardlinksAreNotDuplicates(t *testing.T) {
	// 同 inode 的多条路径是硬链接，删除不释放空间，不算真重复。
	dir := t.TempDir()
	original, size := write(t, dir, "orig.pdf", "hardlinked content")
	linked := filepath.Join(dir, "link.pdf")
	if err := os.Link(original, linked); err != nil {
		t.Skipf("文件系统不支持硬链接，跳过: %v", err)
	}

	entries := []model.Entry{
		{Path: original, Inode: 100, Size: size},
		{Path: linked, Inode: 100, Size: size},
	}
	groups, err := Finder{}.Find(entries)
	if err != nil {
		t.Fatalf("Find 不应报错: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("硬链接不应判为真重复，实际找到 %d 组", len(groups))
	}
}

func TestFind_MixedHardlinkAndRealDuplicate(t *testing.T) {
	// 一组里既有硬链接又有独立副本：只把独立副本算作可回收。
	dir := t.TempDir()
	a, size := write(t, dir, "a.pdf", "mixed content")
	b, _ := write(t, dir, "b.pdf", "mixed content")

	entries := []model.Entry{
		{Path: a, Inode: 1, Size: size},
		{Path: a + ".link", Inode: 1, Size: size}, // 同 inode，硬链接
		{Path: b, Inode: 2, Size: size},           // 不同 inode，真重复
	}
	groups, err := Finder{}.Find(entries)
	if err != nil {
		t.Fatalf("Find 不应报错: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("应找到 1 组，实际 %d 组", len(groups))
	}
	// 3 条路径、2 个不同 inode：可回收 = (3-1)*size 会高估，
	// 实际应保留 2 份（每 inode 一份），可回收 = 1*size。
	if got := groups[0].Reclaimable(); got != size {
		t.Fatalf("可回收应为 %d 字节（保留每 inode 一份），实际 %d", size, got)
	}
}

func TestFind_EmptyAndSingleEntries(t *testing.T) {
	groups, err := Finder{}.Find(nil)
	if err != nil {
		t.Fatalf("Find 不应报错: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("空输入应返回 0 组，实际 %d 组", len(groups))
	}
}

func TestFind_SkipsMissingFiles(t *testing.T) {
	// 扫描后文件被删是正常现象，不应让整个分析失败。
	dir := t.TempDir()
	p1, size := write(t, dir, "exists.txt", "content")

	entries := []model.Entry{
		{Path: p1, Inode: 1, Size: size},
		{Path: filepath.Join(dir, "gone.txt"), Inode: 2, Size: size},
	}
	groups, err := Finder{}.Find(entries)
	if err != nil {
		t.Fatalf("Find 不应报错: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("缺失文件不应产生分组，实际 %d 组", len(groups))
	}
}

func TestTotalReclaimable(t *testing.T) {
	groups := []Group{
		{Size: 100, Entries: []model.Entry{{}, {}}},
		{Size: 50, Entries: []model.Entry{{}, {}, {}}},
	}
	// 100*1 + 50*2 = 200
	if got := TotalReclaimable(groups); got != 200 {
		t.Fatalf("总可回收应为 200，实际 %d", got)
	}
}
