// Package view 承载界面的状态与选择逻辑，不依赖 Fyne。
//
// 把状态从渲染中分离出来，界面层只负责把这里的结果画出来；
// 这样选择、排序、合计等逻辑可以在没有图形环境的情况下测试。
package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kennethfan/wechat-file-keeper/internal/model"
)

// Row 是表格中的一行。
type Row struct {
	// Entry 是该行对应的条目。
	Entry model.Entry
	// Selected 表示该行是否被勾选。
	Selected bool
}

// State 是界面的全部状态。
type State struct {
	// Category 是当前选中的分类，空表示全部。
	Category model.Category
	// Rows 是当前展示的行。
	Rows []Row
	// WeChatRunning 表示微信进程是否在运行。
	WeChatRunning bool
	// Scanning 表示扫描是否进行中。
	Scanning bool
	// LastFreedBytes 是上一次清理实际移动的字节数。
	LastFreedBytes int64
	// StatusMessage 是给用户看的提示。
	StatusMessage string

	// all 是未经筛选的全部条目。
	all []model.Entry

	// collapsed 记录各月份的折叠状态，跨分类切换保留。
	collapsed map[string]bool

	// rowIndex 把条目路径映射到 Rows 下标，避免每次查下标都线性扫描。
	// 界面上每个可见格子都要查一次，线性扫描会退化成 O(n²)。
	rowIndex map[string]int

	// modes 记录用户为各分类显式选择的视图模式。
	modes map[model.Category]ViewMode
}

// NewState 由扫描结果构建初始状态。
//
// 默认展示「视频」分类：它是空间占用的绝对主体，也是最常清理的对象。
func NewState(entries []model.Entry) *State {
	s := &State{Category: model.CategoryVideo}
	s.SetEntries(entries)
	return s
}

// SetEntries 替换条目集合并重建当前视图。
func (s *State) SetEntries(entries []model.Entry) {
	s.all = entries
	s.rebuild()
}

// rebuild 按当前分类筛选并排序。
func (s *State) rebuild() {
	matched := make([]model.Entry, 0, len(s.all))
	for _, e := range s.all {
		if s.Category != "" && e.Category != s.Category {
			continue
		}
		matched = append(matched, e)
	}
	// 默认按大小降序：用户最关心占地最大的。
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].Size > matched[j].Size
	})

	s.Rows = make([]Row, len(matched))
	s.rowIndex = make(map[string]int, len(matched))
	for i, e := range matched {
		s.Rows[i] = Row{Entry: e}
		s.rowIndex[e.Path] = i
	}
}

// IndexOf 返回条目路径在当前视图中的行下标，不存在时返回 -1。
//
// 走 map 查找而非线性扫描：这是渲染热路径，必须 O(1)。
func (s *State) IndexOf(path string) int {
	if i, ok := s.rowIndex[path]; ok {
		return i
	}
	return -1
}

// SetCategory 切换分类并重建视图。
func (s *State) SetCategory(c model.Category) {
	s.Category = c
	s.rebuild()
}

// Toggle 切换某一行的勾选状态。
func (s *State) Toggle(index int) {
	if index < 0 || index >= len(s.Rows) {
		return
	}
	s.Rows[index].Selected = !s.Rows[index].Selected
}

// SelectAll 全选或全不选当前视图。
func (s *State) SelectAll(selected bool) {
	for i := range s.Rows {
		s.Rows[i].Selected = selected
	}
}

// SelectedEntries 返回已勾选的条目。
func (s *State) SelectedEntries() []model.Entry {
	var out []model.Entry
	for _, r := range s.Rows {
		if r.Selected {
			out = append(out, r.Entry)
		}
	}
	return out
}

// SelectionSummary 返回「已选 N 项，合计 X」里的两个数字。
func (s *State) SelectionSummary() (count int, bytes int64) {
	for _, r := range s.Rows {
		if r.Selected {
			count++
			bytes += r.Entry.Size
		}
	}
	return count, bytes
}

// CanClean 表示当前是否允许执行清理。
//
// 微信运行时不再拦截任何类别：清理动作是「移入废纸篓 + 写清单」，
// 随时可 undo，本身就是安全网。风险通过 CleanBlockReason 与
// RunningWarning 以提示形式呈现，由用户知情后自行决定。
func (s *State) CanClean() bool {
	if s.Scanning {
		return false
	}
	count, _ := s.SelectionSummary()
	return count > 0
}

// CleanBlockReason 解释为什么不能清理；可以清理时返回空串。
//
// 只在真的做不了时返回内容（未选条目、正在扫描），
// 微信运行不再作为拒绝理由。
func (s *State) CleanBlockReason() string {
	switch {
	case s.Scanning:
		return "正在扫描，请稍候"
	default:
		if count, _ := s.SelectionSummary(); count == 0 {
			return "请先勾选要清理的条目"
		}
		return ""
	}
}

// allNoRisk 判断是否全部条目都属于无风险类别。
//
// 只认孤儿缩略图：对应视频已不存在，微信不会再读它。
func allNoRisk(entries []model.Entry) bool {
	for _, e := range entries {
		if !e.IsOrphanThumbnail {
			return false
		}
	}
	return true
}

// NeedsConfirmWhileRunning 判断当前选择是否需要二次确认。
//
// 微信运行时，只要选中项里有非无风险类别（视频、图片缓存、聊天文件），
// 就在执行前弹窗确认。工具不替用户决定，只确保用户知情。
func (s *State) NeedsConfirmWhileRunning() bool {
	if !s.WeChatRunning {
		return false
	}
	selected := s.SelectedEntries()
	if len(selected) == 0 {
		return false
	}
	return !allNoRisk(selected)
}

// RunningWarning 生成微信运行时的风险提示文案；无需提示时返回空串。
//
// 按类别给出具体后果，而不是笼统的「有风险」。
func (s *State) RunningWarning() string {
	if !s.WeChatRunning {
		return ""
	}
	selected := s.SelectedEntries()
	if len(selected) == 0 {
		return ""
	}

	var hasCache, hasChat bool
	for _, e := range selected {
		if e.IsOrphanThumbnail {
			continue
		}
		if e.Category == model.CategoryImageCache {
			hasCache = true
			continue
		}
		hasChat = true
	}

	var parts []string
	if hasChat {
		parts = append(parts,
			"聊天文件与视频可能被微信的硬链接记录追踪，删除后微信的「存储空间管理」统计可能对不上（文件本身不受影响）")
	}
	if hasCache {
		parts = append(parts,
			"图片缓存删除后，如果你滚到对应聊天记录，微信会重新下载写回")
	}
	if len(parts) == 0 {
		return ""
	}

	out := "微信正在运行：\n"
	for i, p := range parts {
		out += fmt.Sprintf("%d. %s\n", i+1, p)
	}
	return out
}

// CategoryTotals 按分类汇总字节数，用于左侧分类树。
func (s *State) CategoryTotals() map[model.Category]int64 {
	out := make(map[model.Category]int64, 8)
	for _, e := range s.all {
		out[e.Category] += e.Size
	}
	return out
}

// CategoryLabel 生成分类树的显示文本，形如「视频  2.38 GB」。
func CategoryLabel(c model.Category, bytes int64, human func(int64) string) string {
	return string(c) + "  " + human(bytes)
}

// MatchName 判断条目名是否包含关键字，用于搜索框。
func MatchName(e model.Entry, keyword string) bool {
	if keyword == "" {
		return true
	}
	return strings.Contains(strings.ToLower(e.Path), strings.ToLower(keyword))
}

// SortField 是表格视图的排序字段。
type SortField int

const (
	// SortBySize 按字节数排序。
	SortBySize SortField = iota
	// SortByMonth 按月份排序。
	SortByMonth
	// SortByName 按文件名排序。
	SortByName
)

// SetSort 按指定字段重排当前视图。
//
// 切换同一字段时反转方向，这与常见表格的行为一致。
func (s *State) SetSort(field SortField, descending bool) {
	less := func(i, j int) bool {
		switch field {
		case SortByMonth:
			return s.Rows[i].Entry.Month < s.Rows[j].Entry.Month
		case SortByName:
			return s.Rows[i].Entry.Path < s.Rows[j].Entry.Path
		default:
			return s.Rows[i].Entry.Size < s.Rows[j].Entry.Size
		}
	}

	// 用稳定排序，保证同值时顺序可预期。
	sort.SliceStable(s.Rows, func(i, j int) bool {
		if descending {
			return less(j, i)
		}
		return less(i, j)
	})
	// 行顺序变了，索引表必须重建，否则 IndexOf 会指向错误行。
	s.reindex()
}

// reindex 重建路径到行下标的映射。
func (s *State) reindex() {
	s.rowIndex = make(map[string]int, len(s.Rows))
	for i, r := range s.Rows {
		s.rowIndex[r.Entry.Path] = i
	}
}
