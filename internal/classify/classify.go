// Package classify 把扫描条目归入用户可见的分类。
package classify

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

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
//
// 微信 4.x 有两种封面命名：老版 <基名>_thumb.jpg，新版直接是
// <基名>.jpg。实测 msg/video 下有 304 个不带 _thumb 的 jpg，
// 只按后缀判定会把它们漏成「其他」。
//
// 因此判定分两步：后缀匹配，或位于 video 目录下的 jpg/png。
func IsThumbnail(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, thumbnailSuffix) {
		return true
	}
	// video 目录下的图片按封面处理
	if hasPathComponent(path, "video") {
		switch filepath.Ext(lower) {
		case ".jpg", ".jpeg", ".png":
			return true
		}
	}
	return false
}

// Category 判定一条路径的归类（不含文件头探测）。
//
// .dat 一律先当作图片缓存；要区分「能解码的图片」与「私有编码」，
// 用 Apply——它会读文件头并把可解码的改判为「图片」。
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
	case "jpg", "jpeg", "png", "gif", "bmp", "webp", "tiff", "heic":
		return model.CategoryImage
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

// IsDecodableImage 读文件头判断 .dat 是否是标准图片格式。
//
// 无法读取时返回 false（按不可解码处理，归入图片缓存）。
func IsDecodableImage(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	var head [4]byte
	n, err := f.Read(head[:])
	if err != nil || n < 3 {
		return false
	}

	switch {
	case head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF: // JPEG
		return true
	case head[0] == 0x89 && head[1] == 0x50 && head[2] == 0x4E && head[3] == 0x47: // PNG
		return true
	case head[0] == 0x47 && head[1] == 0x49 && head[2] == 0x46: // GIF
		return true
	case head[0] == 0x4D && head[1] == 0x4D && head[2] == 0x00 && head[3] == 0x2A: // TIFF (BE)
		return true
	case head[0] == 0x49 && head[1] == 0x49 && head[2] == 0x2A && head[3] == 0x00: // TIFF (LE)
		return true
	}
	return false
}

// CategoryFor 给出「界面展示用」的归类。
//
// 封面图归入「视频」而不是「其他」——它们确实是视频的附属产物，
// 用户按类型筛选时应该和视频一起看到。之前把它们一律塞进「其他」，
// 导致 300 多个 jpg 散落在「其他」分类里，看起来像分类错了。
func CategoryFor(path string) model.Category {
	return Category(path)
}

// Apply 就地填充条目的分类与缩略图标记。
//
// .dat 需要读文件头才能区分「可解码图片」与「私有编码」，而本机有
// 一万两千多个，串行读要 47 秒。这里用并发：读文件头是 I/O 密集，
// 8 个 goroutine 足够把耗时压到可接受范围。
func Apply(entries []model.Entry) {
	const workers = 8

	// 先做不需要 I/O 的判定
	for i := range entries {
		e := &entries[i]
		e.IsThumbnail = IsThumbnail(e.Path)
		e.Category = Category(e.Path)
	}

	// 收集需要读文件头的条目
	var needHeader []int
	for i := range entries {
		if IsImageCache(entries[i].Path) {
			needHeader = append(needHeader, i)
		}
	}
	if len(needHeader) == 0 {
		return
	}

	jobs := make(chan int, len(needHeader))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if IsDecodableImage(entries[i].Path) {
					entries[i].Category = model.CategoryImage
				}
			}
		}()
	}
	for _, i := range needHeader {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
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
