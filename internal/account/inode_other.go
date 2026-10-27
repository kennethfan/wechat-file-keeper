//go:build !darwin

package account

import "os"

// inodeOf 在非 darwin 平台不可用，调用方退化为按路径累加。
func inodeOf(os.FileInfo) (uint64, bool) {
	return 0, false
}
