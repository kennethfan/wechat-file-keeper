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
		base := strings.TrimSuffix(filepath.Base(e.Path), thumbnailSuffix)
		key := filepath.Join(filepath.Dir(e.Path), base)
		_, found := videoBase[key]
		e.IsOrphanThumbnail = !found
	}
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
