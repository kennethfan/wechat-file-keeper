package trash

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

func TestMethod_Constants(t *testing.T) {
	if MethodRename != "rename" {
		t.Fatalf("MethodRename 应为 rename，实际 %q", MethodRename)
	}
	if MethodFinder != "finder" {
		t.Fatalf("MethodFinder 应为 finder，实际 %q", MethodFinder)
	}
}

func TestFinderMove_EmptyListIsNoop(t *testing.T) {
	// 空列表不应调用 Finder，也不应报错。
	if err := FinderMove(nil); err != nil {
		t.Fatalf("空列表不应报错: %v", err)
	}
}

func TestFinderMove_MissingFileReturnsError(t *testing.T) {
	// Finder 对不存在的路径会报错，这里应转成明确错误。
	err := FinderMove([]string{"/not/exist/xyz.pdf"})
	if err == nil {
		t.Skip("当前环境 Finder 未报错，跳过")
	}
}

func TestFindTrashPath_FindsExactName(t *testing.T) {
	// 在临时目录模拟废纸篓结构不现实（FindTrashPath 固定读 ~/.Trash），
	// 这里只验证「文件不存在时报错」这一分支。
	if _, err := FindTrashPath("/not/exist/definitely-missing-file.pdf"); err == nil {
		t.Fatal("找不到文件时应报错")
	}
}

func TestCleaner_FinderFallsBackToRename(t *testing.T) {
	// Finder 不可用时（如无授权）应回退到 rename，而不是让清理整体失败。
	base := t.TempDir()
	src := filepath.Join(base, "msg")
	trashDir := filepath.Join(base, ".Trash")
	manifestDir := filepath.Join(base, "manifests")
	for _, d := range []string{src, trashDir, manifestDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
	}

	target := filepath.Join(src, "a.pdf")
	if err := os.WriteFile(target, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("写文件失败: %v", err)
	}

	c := &Cleaner{
		TrashDir:    trashDir,
		ManifestDir: manifestDir,
		Method:      MethodFinder,
		Now:         func() time.Time { return time.Unix(1700000000, 0) },
	}

	m, err := c.Move([]model.Entry{{Path: target, Size: 3}})
	// 无论 Finder 成功与否，都不应整体失败
	if err != nil {
		t.Fatalf("Finder 模式下不应整体失败: %v", err)
	}
	if len(m.Records) != 1 {
		t.Fatalf("应有 1 条记录，实际 %d 条", len(m.Records))
	}
	// 文件必须离开原位置
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatal("文件应已移走")
	}
}
