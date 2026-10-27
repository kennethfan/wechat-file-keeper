//go:build !darwin

package scan

import "os"

// linkInfo 在非 darwin 平台不可用，退化为「每个文件独立 inode」。
func linkInfo(os.FileInfo) (inode, nlink uint64) {
	return 0, 1
}
