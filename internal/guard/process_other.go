//go:build !darwin

package guard

// IsWeChatRunning 在非 darwin 平台恒为 false：本工具只针对 macOS 微信。
func IsWeChatRunning() bool {
	return false
}
