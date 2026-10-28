// Package guard 是清理操作的安全闸门。
//
// 设计原则：默认拒绝。只有同时满足以下条件的目标才被允许操作：
//  1. 路径是绝对路径，且不含 ".." 穿越成分；
//  2. 路径位于当前账号库的消息区之下，且不等于消息区根目录本身；
//  3. 微信运行时的额外限制——见 Checker.AllowWhileRunning 的说明。
//
// 关于「微信运行时是否禁止清理」：早期版本一律禁止，理由是担心
// 微信的 hardlink.db 账本与实际文件不一致。但实测证据不支持这个担心：
//
//   - 微信运行时并不持有 msg/ 下任何聊天文件的句柄（lsof 证实），
//     它打开的是 db_storage 与系统文件；
//   - 用户用 Finder 删除过 msg/file 里的硬链接副本（链接数从 5 变 4），
//     微信没有异常，32 MB 的运行日志里没有任何 hardlink 或
//     file-not-found 错误；
//   - 删除孤儿缩略图这类文件后微信只会按需重建。
//
// 因此微信运行时不再一律拒绝，改为按文件类别区分风险（见 RiskOf）。
package guard

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

var (
	// ErrWeChatRunning 表示该文件在微信运行时不宜清理。
	//
	// 不再是「微信在跑就全拒」，而是针对具体类别给出原因，
	// 详见 RiskOf 与 Checker.CheckEntry。
	ErrWeChatRunning = errors.New("微信正在运行，此类文件建议先退出微信再清理")
	// ErrEmptyPath 路径为空。
	ErrEmptyPath = errors.New("路径为空")
	// ErrRelativePath 路径不是绝对路径。
	ErrRelativePath = errors.New("路径必须是绝对路径")
	// ErrTraversal 路径包含 .. 穿越成分。
	ErrTraversal = errors.New("路径包含 .. 穿越成分")
	// ErrOutsideRoot 路径不在消息区之下。
	ErrOutsideRoot = errors.New("路径不在允许的清理范围内")
	// ErrRootItself 目标就是消息区根目录本身。
	ErrRootItself = errors.New("拒绝操作消息区根目录本身")
	// ErrNotFile 目标不是普通文件。
	ErrNotFile = errors.New("目标不是普通文件")
)

// Checker 校验清理目标是否安全。
type Checker struct {
	// MsgRoot 是当前账号库的消息区绝对路径。
	MsgRoot string
	// WeChatRunning 表示微信进程是否在运行。
	WeChatRunning bool
}

// Check 校验单个目标路径的安全性。
//
// 只看路径本身：是否绝对、是否含穿越成分、是否在消息区之下。
// 微信运行状态不在这里判断——那需要知道文件类别，见 CheckEntry。
// 需要同时考虑两者的调用方应使用 CheckEntry。
func (c Checker) Check(path string) error {
	if strings.TrimSpace(path) == "" {
		return ErrEmptyPath
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%w: %s", ErrRelativePath, path)
	}
	// 先查穿越成分，再比对根目录，避免 "a/../b" 这类写法绕过检查。
	if hasDotDot(path) {
		return fmt.Errorf("%w: %s", ErrTraversal, path)
	}
	clean := filepath.Clean(path)

	root := filepath.Clean(c.MsgRoot)
	if root == "" {
		return fmt.Errorf("%w: 消息区根目录未设置", ErrOutsideRoot)
	}
	if clean == root {
		return fmt.Errorf("%w: %s", ErrRootItself, path)
	}
	// 必须位于 root 之下：加分隔符避免 /a/msgfoo 被误判为在 /a/msg 之下。
	if !strings.HasPrefix(clean, root+string(filepath.Separator)) {
		return fmt.Errorf("%w: %s", ErrOutsideRoot, path)
	}
	return nil
}

// CheckAll 校验一批目标，任一失败即返回该错误。
func (c Checker) CheckAll(paths []string) error {
	for _, p := range paths {
		if err := c.Check(p); err != nil {
			return err
		}
	}
	return nil
}

// hasDotDot 判断路径中是否含 ".." 成分。
func hasDotDot(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return true
		}
	}
	return false
}

// Risk 描述某类文件在微信运行时被清理的风险等级。
type Risk int

const (
	// RiskNone 表示微信运行时可以安全清理。
	//
	// 孤儿缩略图属于这一类：对应视频已不存在，微信不会再读它，
	// 也不在 hardlink.db 的账本里；将来若又需要，微信会重新生成。
	RiskNone Risk = iota

	// RiskLow 表示微信运行时可清理，但可能被立即重建。
	//
	// 图片缓存属于这一类：删掉后如果用户滚到对应聊天记录，
	// 微信会重新下载写回，出现「刚清完又占满」的现象。
	RiskLow

	// RiskMedium 表示清理后可能影响微信自己的统计。
	//
	// 聊天文件与视频属于这一类：它们可能被 hardlink.db 追踪，
	// 微信运行中删除虽实测无异常，但可能出现账本与磁盘不一致，
	// 影响微信自己的「存储空间管理」统计。不阻止清理，只做提示。
	RiskMedium
)

// RiskOf 返回条目在微信运行时被清理的风险等级。
//
// 判定依据是实测证据，而不是笼统的「微信在跑就别动」：
// 孤儿缩略图无风险，图片缓存低风险（会被重建），
// 聊天文件与视频中风险（可能影响微信的硬链接账本）。
func RiskOf(e model.Entry) Risk {
	if e.IsOrphanThumbnail {
		return RiskNone
	}
	if e.Category == model.CategoryImageCache {
		return RiskLow
	}
	return RiskMedium
}

// RiskReason 返回某风险等级的中文说明，供界面与 CLI 提示。
//
// 这些是提示文案，不是拒绝理由：清理照常进行，用户知情即可。
func RiskReason(r Risk) string {
	switch r {
	case RiskNone:
		return "可安全清理，微信会按需重建"
	case RiskLow:
		return "清理后若滚到对应聊天记录，微信可能重新下载"
	default:
		return "可能影响微信自己的空间统计，不影响文件本身"
	}
}

// CheckEntry 校验单个条目。
//
// 只做路径安全校验，不再因微信运行而拒绝——这与用户的实际使用习惯一致：
// 用户长期用 Finder 直接删微信目录里的文件，没有出现过问题；而且本工具
// 的删除动作是「移入废纸篓 + 写清单」，随时可 undo，本身就是安全网。
//
// 微信运行时的风险改由 RiskOf/RiskReason 提示给用户，由用户在界面上
// 知情后自行决定，而不是由工具替用户做决定。
func (c Checker) CheckEntry(e model.Entry) error {
	return c.Check(e.Path)
}
