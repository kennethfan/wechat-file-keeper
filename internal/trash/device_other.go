//go:build !darwin

package trash

import "os"

// deviceOf 在非 darwin 平台不可用，同卷检查退化为不做判断。
func deviceOf(os.FileInfo) (uint64, bool) {
	return 0, false
}
