// Package orphan 识别孤儿缩略图：对应聊天视频已不存在的封面图。
package orphan

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// thumbnailSuffix 是封面图后缀。
const thumbnailSuffix = "_thumb.jpg"

// Mark 就地标记孤儿缩略图。
//
// 微信把封面图命名为 <基名>_thumb.jpg，对应视频为 <基名>.mp4。
// 视频也可能带 _raw 后缀（<基名>_raw.mp4），此时封面图仍是
// <基名>_thumb.jpg，所以判定时必须把 _raw 剥掉再比对。
//
// 只检查同目录下的兄弟文件：微信把同一段视频的 mp4 与封面图
// 放在同一个月份目录里。
func Mark(entries []model.Entry) {
	// 先建立「同目录下已存在的视频基名」集合。
	videoBase := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		base, ok := videoBaseName(e.Path)
		if !ok {
			continue
		}
		key := filepath.Join(filepath.Dir(e.Path), base)
		videoBase[key] = struct{}{}
	}

	for i := range entries {
		e := &entries[i]
		if !e.IsThumbnail {
			continue
		}
		key := filepath.Join(filepath.Dir(e.Path), thumbBaseName(e.Path))
		_, found := videoBase[key]
		e.IsOrphanThumbnail = !found
	}
}

// thumbBaseName 从封面图路径提取基名（不含扩展名与 _thumb 后缀）。
//
// 微信 4.x 有两种封面命名：<基名>_thumb.jpg 与 <基名>.jpg。
// 只剥 _thumb.jpg 会让新版封面留下 ".jpg" 尾巴，基名对不上视频，
// 于是被误判成孤儿。
func thumbBaseName(path string) string {
	base := filepath.Base(path)
	// 先剥 _thumb 后缀（含扩展名）
	if strings.HasSuffix(strings.ToLower(base), thumbnailSuffix) {
		base = base[:len(base)-len(thumbnailSuffix)]
		return base
	}
	// 否则只剥扩展名
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// videoBaseName 从视频路径提取基名（剥掉 .mp4 与可选的 _raw）。
func videoBaseName(path string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".mp4" && ext != ".mov" && ext != ".m4v" {
		return "", false
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.TrimSuffix(base, "_raw")
	return base, true
}

// Exists 判断路径是否存在，供调用方在需要时复核。
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
