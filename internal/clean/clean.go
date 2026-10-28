// Package clean 把护栏、选择与清理器串成一次完整的清理动作。
package clean

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kennethfan/wechat-file-keeper/internal/guard"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/trash"
)

// Options 描述一次清理请求。
type Options struct {
	// Entries 是用户选中的条目。
	Entries []model.Entry
	// MsgRoot 是当前账号库的消息区路径。
	MsgRoot string
	// IncludeCache 表示允许清理图片缓存。
	IncludeCache bool
	// WeChatRunning 由调用方探测后传入，便于测试注入。
	WeChatRunning bool
}

// Plan 是清理前的计划，供确认界面与 CLI 预览使用。
type Plan struct {
	// Selected 是实际会被移动的条目。
	Selected []model.Entry
	// Skipped 是被跳过的条目及原因。
	Skipped []SkipReason
	// TotalBytes 是 Selected 的字节数合计。
	TotalBytes int64
}

// SkipReason 说明某条被跳过的原因。
type SkipReason struct {
	Entry  model.Entry
	Reason string
}

// Build 生成清理计划，不触碰磁盘。
//
// 图片缓存需要显式授权：它是本地唯一副本，微信服务器有过期策略，
// 删除后可能永久丢失，因此默认跳过并要求调用方明确传入 IncludeCache。
func Build(opts Options) Plan {
	var plan Plan
	for _, e := range opts.Entries {
		if e.Category == model.CategoryImageCache && !opts.IncludeCache {
			plan.Skipped = append(plan.Skipped, SkipReason{
				Entry:  e,
				Reason: "图片缓存需显式授权（--include-cache）",
			})
			continue
		}
		plan.Selected = append(plan.Selected, e)
		plan.TotalBytes += e.Size
	}
	return plan
}

// Apply 执行清理计划。
//
// 先逐条过护栏再动文件：任一目标越界、或在微信运行时属于不宜清理的
// 类别，就整体拒绝，保证不会出现「删了一半才发现不该删」的局面。
//
// 微信运行时的判定按文件类别区分（见 guard.RiskOf）：
// 孤儿缩略图照常清理，图片缓存与聊天文件则要求先退出微信。
func Apply(plan Plan, msgRoot string, weChatRunning bool, cleaner *trash.Cleaner) (trash.Manifest, error) {
	checker := guard.Checker{MsgRoot: msgRoot, WeChatRunning: weChatRunning}

	for _, e := range plan.Selected {
		if err := checker.CheckEntry(e); err != nil {
			return trash.Manifest{}, fmt.Errorf("%s: %w", filepath.Base(e.Path), err)
		}
	}

	return cleaner.Move(plan.Selected)
}

// Describe 生成计划的人可读摘要，供 CLI 预览。
func (p Plan) Describe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "将移入废纸篓 %d 项，合计 %d 字节\n", len(p.Selected), p.TotalBytes)
	if len(p.Skipped) > 0 {
		fmt.Fprintf(&b, "跳过 %d 项：\n", len(p.Skipped))
		for _, s := range p.Skipped {
			fmt.Fprintf(&b, "  - %s：%s\n", filepath.Base(s.Entry.Path), s.Reason)
		}
	}
	return b.String()
}
