// Package classify 把扫描条目归入用户可见的分类。
package classify

import (
	"path/filepath"
	"strings"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// 各分类的扩展名集合（小写，不含点）。
var (
	videoExts = map[string]struct{}{
		"mp4": {}, "mov": {}, "m4v": {}, "avi": {}, "mkv": {},
	}
	documentExts = map[string]struct{}{
		"pdf": {}, "doc": {}, "docx": {}, "xls": {}, "xlsx": {}, "ppt": {}, "pptx": {},
		"txt": {}, "md": {}, "csv": {}, "rtf": {}, "pages": {}, "numbers": {}, "key": {},
		"html": {}, "htm": {}, "xml": {}, "json": {}, "conf": {},
	}
	archiveExts = map[string]struct{}{
		"zip": {}, "rar": {}, "7z": {}, "tar": {}, "gz": {}, "bz2": {}, "xz": {},
	}
)

// thumbnailSuffix 是聊天视频封面图的后缀。
const thumbnailSuffix = "_thumb.jpg"

// IsImageCache 判断路径是否为图片缓存。
//
// 判定依据是位于 attach 目录下且扩展名为 .dat：微信把图片缓存的
// 缩略图与高清图都存成 .dat，其中一部分实际是可直接识别的 JPEG。
func IsImageCache(path string) bool {
	if !strings.EqualFold(filepath.Ext(path), ".dat") {
		return false
	}
	return hasPathComponent(path, "attach")
}

// IsThumbnail 判断路径是否为聊天视频封面图。
func IsThumbnail(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), thumbnailSuffix)
}

// Category 判定一条路径的归类。
//
// 判定顺序至关重要：图片缓存与缩略图必须先于扩展名判断，
// 否则缩略图（.jpg）会被误归入视频类，.dat 会落进「其他」。
//
// 注意：缩略图归为视频是「文件类型」层面的归类。界面上是否展示
// 缩略图条目由 CategoryFor 决定——它们通常是视频的附属文件，
// 单独列出只会淹没真正要清理的视频。
func Category(path string) model.Category {
	if IsImageCache(path) {
		return model.CategoryImageCache
	}
	if IsThumbnail(path) {
		return model.CategoryVideo
	}

	switch ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")); ext {
	case "":
		return model.CategoryOther
	default:
		if _, ok := videoExts[ext]; ok {
			return model.CategoryVideo
		}
		if _, ok := documentExts[ext]; ok {
			return model.CategoryDocument
		}
		if _, ok := archiveExts[ext]; ok {
			return model.CategoryArchive
		}
		return model.CategoryOther
	}
}

// CategoryFor 结合孤儿标记给出「界面展示用」的归类。
//
// 缩略图一律归入「其他」，不论是否孤儿：
//   - 孤儿缩略图（对应视频已不存在）需要被看到并清理——本机有 800 多个，
//     是唯一安全的自动可回收项；
//   - 非孤儿缩略图是视频的附属文件，单独列出会让 885 个封面把 123 个
//     真正的视频淹掉。
func CategoryFor(path string) model.Category {
	if IsThumbnail(path) {
		return model.CategoryOther
	}
	return Category(path)
}

// Apply 就地填充条目的分类与缩略图标记。
//
// 用 CategoryFor 而非 Category：缩略图需要归入「其他」而不是「视频」，
// 否则视频分类里会混进几百个封面图条目。
func Apply(entries []model.Entry) {
	for i := range entries {
		e := &entries[i]
		e.IsThumbnail = IsThumbnail(e.Path)
		e.Category = CategoryFor(e.Path)
	}
}

// hasPathComponent 判断路径中是否含指定目录名成分。
func hasPathComponent(path, name string) bool {
	for {
		dir := filepath.Dir(path)
		if dir == path {
			return false
		}
		if filepath.Base(dir) == name {
			return true
		}
		path = dir
	}
}
