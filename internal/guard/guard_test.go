package guard

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
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

func TestCheck_IgnoresWeChatState(t *testing.T) {
	// Check 只看路径安全性，微信运行状态由 CheckEntry 判断。
	c := Checker{MsgRoot: testRoot, WeChatRunning: true}
	target := filepath.Join(testRoot, "video/2024-09/abc.mp4")

	if err := c.Check(target); err != nil {
		t.Fatalf("Check 不应因微信运行而拒绝，实际: %v", err)
	}
}

func TestCheckEntry_AllowsOrphanThumbnailWhileRunning(t *testing.T) {
	// 孤儿缩略图无风险：微信运行时也应放行。
	c := Checker{MsgRoot: testRoot, WeChatRunning: true}
	e := model.Entry{
		Path:              filepath.Join(testRoot, "video/2024-09/gone_thumb.jpg"),
		IsOrphanThumbnail: true,
	}

	if err := c.CheckEntry(e); err != nil {
		t.Fatalf("孤儿缩略图在微信运行时应可清理，实际: %v", err)
	}
}

func TestCheckEntry_AllowsImageCacheWhileRunning(t *testing.T) {
	// 微信运行不再拦截，风险由界面二次确认呈现。
	c := Checker{MsgRoot: testRoot, WeChatRunning: true}
	e := model.Entry{
		Path:     filepath.Join(testRoot, "attach/a/2025-08/Img/x.dat"),
		Category: model.CategoryImageCache,
	}

	if err := c.CheckEntry(e); err != nil {
		t.Fatalf("图片缓存应可清理（需二次确认），实际: %v", err)
	}
}

func TestCheckEntry_AllowsChatFileWhileRunning(t *testing.T) {
	c := Checker{MsgRoot: testRoot, WeChatRunning: true}
	e := model.Entry{
		Path:     filepath.Join(testRoot, "file/2025-02/x.pdf"),
		Category: model.CategoryDocument,
	}

	if err := c.CheckEntry(e); err != nil {
		t.Fatalf("聊天文件应可清理（需二次确认），实际: %v", err)
	}
}

func TestCheckEntry_AllowsEverythingWhenNotRunning(t *testing.T) {
	c := Checker{MsgRoot: testRoot, WeChatRunning: false}
	for _, e := range []model.Entry{
		{Path: filepath.Join(testRoot, "video/a.mp4"), Category: model.CategoryVideo},
		{Path: filepath.Join(testRoot, "attach/x.dat"), Category: model.CategoryImageCache},
	} {
		if err := c.CheckEntry(e); err != nil {
			t.Fatalf("微信未运行时 %s 应可清理，实际: %v", e.Path, err)
		}
	}
}

func TestCheckEntry_StillRejectsOutOfBounds(t *testing.T) {
	// 路径安全校验不受微信状态影响。
	c := Checker{MsgRoot: testRoot, WeChatRunning: false}
	e := model.Entry{Path: "/Users/k/Desktop/x.pdf"}

	if err := c.CheckEntry(e); !errors.Is(err, ErrOutsideRoot) {
		t.Fatalf("越界路径应被拒绝，实际: %v", err)
	}
}

func TestRiskOf(t *testing.T) {
	cases := []struct {
		name string
		e    model.Entry
		want Risk
	}{
		{"孤儿缩略图", model.Entry{IsOrphanThumbnail: true}, RiskNone},
		{"图片缓存", model.Entry{Category: model.CategoryImageCache}, RiskLow},
		{"聊天视频", model.Entry{Category: model.CategoryVideo}, RiskMedium},
		{"聊天文档", model.Entry{Category: model.CategoryDocument}, RiskMedium},
		{"压缩包", model.Entry{Category: model.CategoryArchive}, RiskMedium},
	}
	for _, c := range cases {
		if got := RiskOf(c.e); got != c.want {
			t.Fatalf("%s 的风险应为 %v，实际 %v", c.name, c.want, got)
		}
	}
}

func TestRiskReason_NonEmpty(t *testing.T) {
	for _, r := range []Risk{RiskNone, RiskLow, RiskMedium} {
		if RiskReason(r) == "" {
			t.Fatalf("风险 %v 应有说明文案", r)
		}
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
