//go:build darwin

package trash

import (
	"os"
	"syscall"
)

// deviceOf 取出文件所在设备号，用于判断是否同卷。
func deviceOf(fi os.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}
