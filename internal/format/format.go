// Package format 提供人可读的字节与数量格式化。
package format

import "fmt"

// Bytes 把字节数格式化为带单位的可读字符串。
//
// 采用 1024 进制并保留一位小数，例如 "4.77 GB"。
// 小于 1 KB 时直接显示字节数，避免出现 "0.0 KB" 这种无信息量的输出。
func Bytes(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n < 0:
		return "0 B"
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	case n < gb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/gb)
	}
}

// Count 格式化条目数，千分位分隔。
func Count(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}
