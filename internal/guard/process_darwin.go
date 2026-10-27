//go:build darwin

package guard

import (
	"os/exec"
	"strings"
)

// wechatProcessPattern 是微信主进程的命令行特征。
//
// 只匹配主进程路径，避免把 WeChatAppEx（小程序容器）也算进去：
// 后者随主进程退出，单独存在不影响清理。
const wechatProcessPattern = "WeChat.app/Contents/MacOS/WeChat"

// IsWeChatRunning 判断微信主进程是否在运行。
//
// 用 pgrep -f 匹配完整命令行。pgrep 在 macOS 上随系统提供，
// 无需额外依赖；匹配不到时返回 false 而非报错——检测不到
// 进程不等于微信在运行，不应因此阻塞用户。
func IsWeChatRunning() bool {
	out, err := exec.Command("pgrep", "-f", wechatProcessPattern).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}
