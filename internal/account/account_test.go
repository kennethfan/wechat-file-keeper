package account

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeFile 在 dir 下写一个指定内容的文件，返回其路径。
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

// makeRoot 造一个假的微信数据根目录。
func makeRoot(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

func TestDiscover_EmptyRootReturnsNoLibraries(t *testing.T) {
	root := makeRoot(t)

	libs, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover 不应报错: %v", err)
	}
	if len(libs) != 0 {
		t.Fatalf("空目录应返回 0 个账号库，实际 %d 个", len(libs))
	}
}

func TestDiscover_MissingRootReportsMissing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-exist")

	_, err := Discover(root)
	if !errors.Is(err, ErrRootMissing) {
		t.Fatalf("根目录不存在时应返回 ErrRootMissing，实际: %v", err)
	}
}

func TestDiscover_IgnoresNonAccountDirectories(t *testing.T) {
	root := makeRoot(t)
	// 微信会在 xwechat_files 下放这些非账号目录。
	for _, name := range []string{"all_users", "Backup", "config"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "wxid_aaa"), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	libs, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover 不应报错: %v", err)
	}
	if len(libs) != 1 {
		t.Fatalf("应只识别 1 个账号库，实际 %d 个: %+v", len(libs), libs)
	}
	if libs[0].WXID != "wxid_aaa" {
		t.Fatalf("账号库 WXID 应为 wxid_aaa，实际 %q", libs[0].WXID)
	}
}

func TestSizeOf_CountsHardlinksOnce(t *testing.T) {
	root := makeRoot(t)
	libDir := filepath.Join(root, "wxid_hard")
	original := writeFile(t, libDir, "msg/file/2024-07/简历.pdf", "same-content-100-bytes-placeholder")

	// 造硬链接：同一 inode 挂在另一个月份目录下。
	linked := filepath.Join(libDir, "msg/file/2024-12/简历(1).pdf")
	if err := os.MkdirAll(filepath.Dir(linked), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.Link(original, linked); err != nil {
		t.Skipf("文件系统不支持硬链接，跳过: %v", err)
	}

	size, err := SizeOf(Library{WXID: "wxid_hard", Path: libDir})
	if err != nil {
		t.Fatalf("SizeOf 不应报错: %v", err)
	}

	fi, statErr := os.Stat(original)
	if statErr != nil {
		t.Fatalf("stat 失败: %v", statErr)
	}
	want := fi.Size()
	if size != want {
		t.Fatalf("硬链接应只计一次: 期望 %d 字节，实际 %d 字节", want, size)
	}
}

func TestSizeOf_CountsDistinctFilesSeparately(t *testing.T) {
	root := makeRoot(t)
	libDir := filepath.Join(root, "wxid_two")
	writeFile(t, libDir, "msg/file/2024-07/a.txt", "aaaa")
	writeFile(t, libDir, "msg/file/2024-07/b.txt", "bbbbbb")

	size, err := SizeOf(Library{WXID: "wxid_two", Path: libDir})
	if err != nil {
		t.Fatalf("SizeOf 不应报错: %v", err)
	}
	if size != 10 {
		t.Fatalf("两个独立文件应为 10 字节，实际 %d 字节", size)
	}
}

func TestLargest_PicksBiggestLibrary(t *testing.T) {
	root := makeRoot(t)
	writeFile(t, filepath.Join(root, "wxid_small"), "msg/file/a.txt", "small")
	writeFile(t, filepath.Join(root, "wxid_big"), "msg/file/big.bin", "this one is clearly the bigger library")

	lib, err := Largest(root)
	if err != nil {
		t.Fatalf("Largest 不应报错: %v", err)
	}
	if lib.WXID != "wxid_big" {
		t.Fatalf("应选中 wxid_big，实际 %q", lib.WXID)
	}
}

func TestLargest_NoLibraryReportsNoAccount(t *testing.T) {
	root := makeRoot(t)

	_, err := Largest(root)
	if !errors.Is(err, ErrNoAccount) {
		t.Fatalf("没有账号库时应返回 ErrNoAccount，实际: %v", err)
	}
}
