// Package cachestat 汇总图片缓存的年份分布。
package cachestat

import (
	"sort"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// YearStat 是某一年的图片缓存统计。
type YearStat struct {
	// Year 是四位年份。
	Year string
	// Count 是该年的文件数。
	Count int
	// Bytes 是该年的字节数。
	Bytes int64
}

// ByYear 按年份聚合图片缓存条目，年份降序（新的在前）。
//
// 年份取自月份目录名（YYYY-MM）的前四位。无法解析月份的条目
// 归入「未知」，排在最后。
func ByYear(entries []model.Entry) []YearStat {
	acc := make(map[string]*YearStat, 16)
	for _, e := range entries {
		if e.Category != model.CategoryImageCache {
			continue
		}
		year := yearOf(e.Month)
		s, ok := acc[year]
		if !ok {
			s = &YearStat{Year: year}
			acc[year] = s
		}
		s.Count++
		s.Bytes += e.Size
	}

	stats := make([]YearStat, 0, len(acc))
	for _, s := range acc {
		stats = append(stats, *s)
	}
	sort.Slice(stats, func(i, j int) bool {
		// 「未知」排最后，其余按年份降序。
		if stats[i].Year == "未知" {
			return false
		}
		if stats[j].Year == "未知" {
			return true
		}
		return stats[i].Year > stats[j].Year
	})
	return stats
}

// yearOf 从月份名提取年份；无法解析时返回「未知」。
func yearOf(month string) string {
	if len(month) < 4 {
		return "未知"
	}
	year := month[:4]
	for _, r := range year {
		if r < '0' || r > '9' {
			return "未知"
		}
	}
	return year
}

// OrphanTotal 汇总孤儿缩略图的数量与字节数。
func OrphanTotal(entries []model.Entry) (count int, bytes int64) {
	for _, e := range entries {
		if e.IsOrphanThumbnail {
			count++
			bytes += e.Size
		}
	}
	return count, bytes
}

// ByCategory 按分类汇总字节数。
func ByCategory(entries []model.Entry) map[model.Category]int64 {
	out := make(map[model.Category]int64, 8)
	for _, e := range entries {
		out[e.Category] += e.Size
	}
	return out
}
