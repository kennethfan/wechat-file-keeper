package ui

import "os"

// userHome 返回用户主目录，抽出来便于测试替换。
func userHome() (string, error) {
	return os.UserHomeDir()
}
