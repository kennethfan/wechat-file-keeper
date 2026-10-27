// Package model 定义跨层共享的领域类型。
package model

// Category 是用户可见的顶层归类。
type Category string

const (
	// CategoryVideo 聊天视频（含缩略图）。
	CategoryVideo Category = "视频"
	// CategoryDocument 文档类聊天文件。
	CategoryDocument Category = "文档"
	// CategoryArchive 压缩包类聊天文件。
	CategoryArchive Category = "压缩包"
	// CategoryOther 未归入以上各类的聊天文件。
	CategoryOther Category = "其他"
	// CategoryImageCache 图片缓存（attach 下的 .dat）。
	CategoryImageCache Category = "图片缓存"
)

// Entry 是扫描产出的单条记录。
//
// 一条 Entry 对应磁盘上的一条路径。同一份内容可能以硬链接形式产生多条
// Entry，它们共享同一个 Inode；空间计量时必须按 Inode 归并，
// 否则会把硬链接重复计入占用（见 ADR-0001）。
type Entry struct {
	// Path 是绝对路径。
	Path string
	// Inode 是文件在所在卷上的 inode 号。
	Inode uint64
	// Nlink 是硬链接数。
	Nlink uint64
	// Size 是该路径的文件字节数。
	Size int64
	// ModTime 是文件修改时间的 Unix 秒。
	ModTime int64
	// Month 是所在月份目录名，形如 "2025-02"；无法解析时为空。
	Month string
	// Category 是该条目所属的用户可见归类。
	Category Category
	// IsThumbnail 标记这是聊天视频的封面图。
	IsThumbnail bool
	// IsOrphanThumbnail 标记这是孤儿缩略图（对应视频已不存在）。
	IsOrphanThumbnail bool
}

// SpaceReport 是一次扫描的空间统计结果。
//
// Nominal 与 Actual 的差值即硬链接造成的虚增：
// 同一 inode 挂了 n 条路径时，Nominal 会把它算 n 次，Actual 只算 1 次。
type SpaceReport struct {
	// Nominal 是按路径累加的字节数。
	Nominal int64
	// Actual 是按 inode 归并后的字节数，反映磁盘真实占用。
	Actual int64
	// HardlinkOvercount 是 Nominal 与 Actual 的差值。
	HardlinkOvercount int64
	// EntryCount 是条目总数（含硬链接产生的重复路径）。
	EntryCount int
	// FileCount 是按 inode 归并后的文件数。
	FileCount int
}
