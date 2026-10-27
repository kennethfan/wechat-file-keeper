package guard

import (
	"errors"
	"path/filepath"
	"testing"
)

// testRoot 是测试用的消息区根目录。
const testRoot = "/Users/k/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/wxid_abc/msg"

func TestCheck_AllowsPathInsideMsgRoot(t *testing.T) {
	c := Checker{MsgRoot: testRoot}
	target := filepath.Join(testRoot, "video/2024-09/abc.mp4")

	if err := c.Check(target); err != nil {
		t.Fatalf("消息区内的路径应被允许，实际报错: %v", err)
	}
}

func TestCheck_RejectsWhenWeChatRunning(t *testing.T) {
	c := Checker{MsgRoot: testRoot, WeChatRunning: true}
	target := filepath.Join(testRoot, "video/2024-09/abc.mp4")

	err := c.Check(target)
	if !errors.Is(err, ErrWeChatRunning) {
		t.Fatalf("微信运行时应返回 ErrWeChatRunning，实际: %v", err)
	}
}

func TestCheck_RejectsEmptyPath(t *testing.T) {
	c := Checker{MsgRoot: testRoot}

	if err := c.Check(""); !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("空路径应返回 ErrEmptyPath，实际: %v", err)
	}
}

func TestCheck_RejectsRelativePath(t *testing.T) {
	c := Checker{MsgRoot: testRoot}

	if err := c.Check("video/2024-09/abc.mp4"); !errors.Is(err, ErrRelativePath) {
		t.Fatalf("相对路径应返回 ErrRelativePath，实际: %v", err)
	}
}

func TestCheck_RejectsTraversal(t *testing.T) {
	c := Checker{MsgRoot: testRoot}
	// 穿越到消息区之外。
	target := testRoot + "/../../../etc/passwd"

	if err := c.Check(target); !errors.Is(err, ErrTraversal) {
		t.Fatalf("含 .. 的路径应返回 ErrTraversal，实际: %v", err)
	}
}

func TestCheck_RejectsMsgRootItself(t *testing.T) {
	c := Checker{MsgRoot: testRoot}

	if err := c.Check(testRoot); !errors.Is(err, ErrRootItself) {
		t.Fatalf("消息区根目录本身应被拒绝，实际: %v", err)
	}
}

func TestCheck_RejectsPathOutsideRoot(t *testing.T) {
	c := Checker{MsgRoot: testRoot}

	for _, p := range []string{
		"/Users/k/Desktop/重要文件.pdf",
		"/Users/k/Library/Containers/com.tencent.xinWeChat/Data/Documents/xwechat_files/wxid_abc/db_storage/msg.db",
		"/tmp/x.mp4",
	} {
		if err := c.Check(p); !errors.Is(err, ErrOutsideRoot) {
			t.Fatalf("%s 应返回 ErrOutsideRoot，实际: %v", p, err)
		}
	}
}

func TestCheck_SiblingPrefixIsNotInside(t *testing.T) {
	// /a/msgfoo 不能被误判为在 /a/msg 之下。
	c := Checker{MsgRoot: "/a/msg"}

	if err := c.Check("/a/msgfoo/x.mp4"); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("同前缀的兄弟目录应被拒绝，实际: %v", err)
	}
}

func TestCheckAll_StopsAtFirstFailure(t *testing.T) {
	c := Checker{MsgRoot: testRoot}
	paths := []string{
		filepath.Join(testRoot, "file/2025-02/ok.pdf"),
		"/Users/k/Desktop/坏路径.pdf",
	}

	if err := c.CheckAll(paths); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("应返回 ErrOutsideRoot，实际: %v", err)
	}
}

func TestCheckAll_EmptyListPasses(t *testing.T) {
	c := Checker{MsgRoot: testRoot}

	if err := c.CheckAll(nil); err != nil {
		t.Fatalf("空列表应通过，实际: %v", err)
	}
}
