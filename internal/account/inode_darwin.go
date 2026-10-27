//go:build darwin

package account

import (
	"os"
	"syscall"
)

// inodeOf 从 FileInfo 中取出 inode 号。
//
// 只在 darwin 上实现：本工具依赖 macOS 微信的数据布局，
// 其他平台不提供 inode 归并，会退化为按路径累加。
func inodeOf(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return st.Ino, true
}
