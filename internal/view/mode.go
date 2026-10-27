package view

import "github.com/kennethfan/wechat-file-keeper/internal/model"

// ViewMode 是条目的呈现方式。
type ViewMode string

const (
	// ModeList 是紧凑单行列表，适合快速扫一遍。
	ModeList ViewMode = "列表"
	// ModeTable 是多列表格，适合精确定位。
	ModeTable ViewMode = "表格"
	// ModeGrid 是缩略图网格，图片与视频的主力视图。
	ModeGrid ViewMode = "网格"
)

// AllModes 是全部视图模式，顺序即界面上的展示顺序。
var AllModes = []ViewMode{ModeList, ModeTable, ModeGrid}

// DefaultModeFor 返回某个分类的默认视图。
//
// 视频与图片缓存有缩略图，网格最能发挥；文档需要看列，用表格；
// 其余用紧凑列表。这只是初值——用户手动切换后以用户选择为准。
func DefaultModeFor(c model.Category) ViewMode {
	switch c {
	case model.CategoryVideo, model.CategoryImageCache:
		return ModeGrid
	case model.CategoryDocument:
		return ModeTable
	default:
		return ModeList
	}
}

// ModeFor 返回某分类当前应使用的视图模式。
//
// 优先用户为该分类显式选择过的模式，否则回退到默认值。
func (s *State) ModeFor(c model.Category) ViewMode {
	if s.modes != nil {
		if m, ok := s.modes[c]; ok {
			return m
		}
	}
	return DefaultModeFor(c)
}

// SetMode 记录用户为某分类选择的视图模式。
func (s *State) SetMode(c model.Category, m ViewMode) {
	if s.modes == nil {
		s.modes = make(map[model.Category]ViewMode, 4)
	}
	s.modes[c] = m
}

// CurrentMode 返回当前分类正在使用的视图模式。
func (s *State) CurrentMode() ViewMode {
	return s.ModeFor(s.Category)
}

// SetCurrentMode 为当前分类设置视图模式。
func (s *State) SetCurrentMode(m ViewMode) {
	s.SetMode(s.Category, m)
}

// IsValidMode 判断字符串是否是合法的视图模式。
func IsValidMode(s string) bool {
	for _, m := range AllModes {
		if string(m) == s {
			return true
		}
	}
	return false
}
