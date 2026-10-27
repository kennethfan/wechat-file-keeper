//go:build darwin

package scan

import (
	"os"
	"syscall"
)

// linkInfo 取出 inode 与硬链接数。
func linkInfo(fi os.FileInfo) (inode, nlink uint64) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 1
	}
	return st.Ino, uint64(st.Nlink)
}
