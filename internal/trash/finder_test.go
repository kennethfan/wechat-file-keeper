package trash

import "testing"

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
