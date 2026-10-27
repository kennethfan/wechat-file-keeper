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

// Apply 就地填充条目的分类与缩略图标记。
func Apply(entries []model.Entry) {
	for i := range entries {
		e := &entries[i]
		e.Category = Category(e.Path)
		e.IsThumbnail = IsThumbnail(e.Path)
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
