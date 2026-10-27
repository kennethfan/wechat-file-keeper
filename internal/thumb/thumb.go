// Package thumb 推导并加载聊天视频的封面图。
package thumb

import (
	"image"
	"os"
	"path/filepath"
	"strings"

	// 注册 JPEG 解码器，供 image.Decode 使用。
	_ "image/jpeg"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// suffix 是封面图后缀。
const suffix = "_thumb.jpg"

// videoExts 是需要封面的视频扩展名。
var videoExts = map[string]struct{}{
	".mp4": {}, ".mov": {}, ".m4v": {},
}

// PathFor 推导某个视频对应的封面图路径。
//
// 微信把视频存为 <基名>.mp4 或 <基名>_raw.mp4，封面统一是
// <基名>_thumb.jpg。第二个返回值表示该条目是否值得去找封面：
// 缩略图条目自身、以及非视频条目都返回 false。
func PathFor(e model.Entry) (string, bool) {
	if e.IsThumbnail {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(e.Path))
	if _, ok := videoExts[ext]; !ok {
		return "", false
	}

	base := strings.TrimSuffix(filepath.Base(e.Path), filepath.Ext(e.Path))
	base = strings.TrimSuffix(base, "_raw")
	return filepath.Join(filepath.Dir(e.Path), base+suffix), true
}

// Loader 按需加载封面并缓存，避免重复解码同一张图。
//
// 缓存以路径为键，存的是**已缩放**的小图而非原始尺寸：原图平均
// 460 KB（RGBA），缩到 150px 宽后约 90 KB，缓存 200 张从 92 MB
// 降到 18 MB。这是必要的——全量缓存原图会让内存轻易破百 MB。
//
// 容量上限防止大账号库把内存吃满：超出时清空重来，这比实现 LRU
// 简单，且对「滚动浏览」这种访问模式足够好。
type Loader struct {
	// MaxEntries 是缓存条目上限，0 表示使用默认值。
	MaxEntries int
	// cache 保存已缩放的小图。
	cache map[string]image.Image
	// failed 记住加载失败的路径，避免反复尝试。
	failed map[string]struct{}
}

// DefaultMaxEntries 是默认的封面缓存上限。
//
// 按单张约 0.7 MB（300px 宽 RGBA）估算，60 张约占 42 MB。
// 上限过高会让内存轻易破百 MB：封面只是滚动时的视觉辅助，
// 用户来回滚动时重新解码的代价远小于常驻几百 MB。
const DefaultMaxEntries = 60

// TargetWidth 是封面缩放后的目标宽度（像素）。
//
// 网格格子约 150px 宽，按 2x 屏幕密度取 300 可以避免模糊，
// 同时把单张内存压到原图的四分之一以下。
const TargetWidth = 300

// NewLoader 创建一个带默认容量的加载器。
func NewLoader() *Loader {
	return &Loader{
		MaxEntries: DefaultMaxEntries,
		cache:      make(map[string]image.Image, DefaultMaxEntries),
		failed:     make(map[string]struct{}),
	}
}

// Load 加载条目对应的封面。
//
// 没有封面、文件不存在或解码失败时返回 false，调用方应显示占位块。
// 失败结果会被记住，滚动来回时不会反复读盘。
func (l *Loader) Load(e model.Entry) (image.Image, bool) {
	path, ok := PathFor(e)
	if !ok {
		return nil, false
	}

	if img, hit := l.cache[path]; hit {
		return img, true
	}
	if _, bad := l.failed[path]; bad {
		return nil, false
	}

	img, err := decode(path)
	if err != nil {
		l.failed[path] = struct{}{}
		return nil, false
	}

	small := ScaleToWidth(img, TargetWidth)
	l.put(path, small)
	return small, true
}

// ScaleToWidth 把图片等比缩放到指定宽度。
//
// 用最近邻采样：封面只是列表里的缩略指示，不需要高质量重采样，
// 而最近邻比双线性快数倍，能让滚动明显更跟手。原图已不大于目标
// 宽度时原样返回，避免无谓的放大。
func ScaleToWidth(src image.Image, width int) image.Image {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 || width <= 0 || srcW <= width {
		return src
	}

	height := int(float64(srcH) * float64(width) / float64(srcW))
	if height < 1 {
		height = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	// 用整数步进代替浮点乘除，这是最近邻缩放的主要开销所在。
	xStep := float64(srcW) / float64(width)
	yStep := float64(srcH) / float64(height)
	for y := 0; y < height; y++ {
		srcY := bounds.Min.Y + int(float64(y)*yStep)
		for x := 0; x < width; x++ {
			srcX := bounds.Min.X + int(float64(x)*xStep)
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

// put 写入缓存，超限时整体清空。
func (l *Loader) put(path string, img image.Image) {
	limit := l.MaxEntries
	if limit <= 0 {
		limit = DefaultMaxEntries
	}
	if len(l.cache) >= limit {
		l.cache = make(map[string]image.Image, limit)
	}
	l.cache[path] = img
}

// decode 读取并解码一张封面。
func decode(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// Cached 只查缓存，不触发解码。
//
// 界面渲染时用它做快速判断：命中就直接画，未命中就发起后台解码，
// 本次先显示占位，避免在渲染线程里做 I/O。
func (l *Loader) Cached(e model.Entry) (image.Image, bool) {
	path, ok := PathFor(e)
	if !ok {
		return nil, false
	}
	img, hit := l.cache[path]
	return img, hit
}

// Failed 查询某个条目是否已知加载失败，避免反复发起后台解码。
func (l *Loader) Failed(e model.Entry) bool {
	path, ok := PathFor(e)
	if !ok {
		return true
	}
	_, bad := l.failed[path]
	return bad
}
