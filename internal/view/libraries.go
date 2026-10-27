package view

import (
	"sort"

	"github.com/kennethfan/wechat-file-keeper/internal/account"
	"github.com/kennethfan/wechat-file-keeper/internal/format"
)

// LibraryOption 是账号库下拉里的一项。
type LibraryOption struct {
	// Library 是账号库本身。
	Library account.Library
	// Bytes 是该账号库的实占字节数。
	Bytes int64
}

// Label 生成下拉显示文本，形如「wxid_abc  4.77 GB」。
func (o LibraryOption) Label() string {
	return o.Library.WXID + "  " + format.Bytes(o.Bytes)
}

// Libraries 承载账号库列表与当前选择。
type Libraries struct {
	// Options 是全部账号库，按实占大小降序。
	Options []LibraryOption
	// Current 是当前选中项的下标；列表为空时为 -1。
	Current int
}

// NewLibraries 由账号库列表构建选择器。
//
// 按实占大小降序排列，默认选中最大的那个——用户通常只关心占空间最多的账号。
func NewLibraries(options []LibraryOption) *Libraries {
	sorted := make([]LibraryOption, len(options))
	copy(sorted, options)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Bytes > sorted[j].Bytes
	})

	l := &Libraries{Options: sorted, Current: -1}
	if len(sorted) > 0 {
		l.Current = 0
	}
	return l
}

// CurrentOption 返回当前选中的账号库；没有账号库时返回零值与 false。
func (l *Libraries) CurrentOption() (LibraryOption, bool) {
	if l.Current < 0 || l.Current >= len(l.Options) {
		return LibraryOption{}, false
	}
	return l.Options[l.Current], true
}

// Select 切换到指定下标的账号库，返回是否真的发生了变化。
//
// 返回 false 表示无需重新扫描：下标非法，或点的就是当前项。
func (l *Libraries) Select(index int) bool {
	if index < 0 || index >= len(l.Options) || index == l.Current {
		return false
	}
	l.Current = index
	return true
}

// Switchable 表示是否可以在多个账号库之间切换。
//
// 只有一个账号库时下拉仍然显示（让用户知道识别到了哪个账号），但不可切换。
func (l *Libraries) Switchable() bool {
	return len(l.Options) > 1
}

// Labels 返回全部下拉文本，供界面直接使用。
func (l *Libraries) Labels() []string {
	out := make([]string, len(l.Options))
	for i, o := range l.Options {
		out[i] = o.Label()
	}
	return out
}
