package view

import "sort"

// UnknownMonth 是无法解析月份的条目所属的组名。
const UnknownMonth = "未知月份"

// Group 是一个按月份聚合的组。
type Group struct {
	// Month 是月份名，形如 "2025-08"；无法解析时为 UnknownMonth。
	Month string
	// Rows 是组内条目，按大小降序。
	Rows []Row
	// Collapsed 表示该组是否被折叠。
	Collapsed bool
}

// Count 返回组内条目数。
func (g Group) Count() int { return len(g.Rows) }

// Bytes 返回组内字节数合计。
func (g Group) Bytes() int64 {
	var total int64
	for _, r := range g.Rows {
		total += r.Entry.Size
	}
	return total
}

// SelectedCount 返回组内已勾选的条目数。
func (g Group) SelectedCount() int {
	n := 0
	for _, r := range g.Rows {
		if r.Selected {
			n++
		}
	}
	return n
}

// SelectionState 描述组头的勾选状态。
type SelectionState int

const (
	// SelectionNone 表示组内没有条目被选中。
	SelectionNone SelectionState = iota
	// SelectionPartial 表示组内部分条目被选中。
	SelectionPartial
	// SelectionAll 表示组内全部条目被选中。
	SelectionAll
)

// Selection 返回组头的三态勾选状态。
func (g Group) Selection() SelectionState {
	if len(g.Rows) == 0 {
		return SelectionNone
	}
	n := g.SelectedCount()
	switch {
	case n == 0:
		return SelectionNone
	case n == len(g.Rows):
		return SelectionAll
	default:
		return SelectionPartial
	}
}

// GroupByMonth 把行按月份分组。
//
// 组按月份降序（新的在前），「未知月份」固定排最后；组内保持传入顺序，
// 调用方负责先按大小排序。折叠状态从 previous 里继承，这样切换分类
// 再切回来时折叠状态仍然保留。
func GroupByMonth(rows []Row, previous map[string]bool) []Group {
	byMonth := make(map[string][]Row, 16)
	for _, r := range rows {
		month := r.Entry.Month
		if month == "" {
			month = UnknownMonth
		}
		byMonth[month] = append(byMonth[month], r)
	}

	groups := make([]Group, 0, len(byMonth))
	for month, rs := range byMonth {
		groups = append(groups, Group{
			Month:     month,
			Rows:      rs,
			Collapsed: previous[month],
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		// 「未知月份」排最后。
		if groups[i].Month == UnknownMonth {
			return false
		}
		if groups[j].Month == UnknownMonth {
			return true
		}
		return groups[i].Month > groups[j].Month
	})
	return groups
}

// ToggleGroup 折叠或展开一个组。
func (s *State) ToggleGroup(month string) {
	if s.collapsed == nil {
		s.collapsed = make(map[string]bool, 8)
	}
	s.collapsed[month] = !s.collapsed[month]
}

// GroupCollapsed 查询某个组是否折叠。
func (s *State) GroupCollapsed(month string) bool {
	return s.collapsed[month]
}

// SelectGroup 全选或全不选某个组的条目。
//
// 直接改 State.Rows 里对应条目的 Selected，保证组操作与逐行勾选
// 作用于同一份状态，不会出现两套选择互相打架。
func (s *State) SelectGroup(month string, selected bool) {
	for i := range s.Rows {
		m := s.Rows[i].Entry.Month
		if m == "" {
			m = UnknownMonth
		}
		if m == month {
			s.Rows[i].Selected = selected
		}
	}
}

// Groups 返回当前视图按月份分组的快照。
func (s *State) Groups() []Group {
	return GroupByMonth(s.Rows, s.collapsed)
}
