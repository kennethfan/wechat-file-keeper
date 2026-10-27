// Package ui 是 Fyne 界面层，只负责渲染与转发用户操作。
//
// 所有状态与判断都在 internal/view 里，这里不重复实现业务逻辑，
// 以保证 GUI 与 CLI 的行为一致。
package ui

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/kennethfan/wechat-file-keeper/internal/account"
	"github.com/kennethfan/wechat-file-keeper/internal/clean"
	"github.com/kennethfan/wechat-file-keeper/internal/format"
	"github.com/kennethfan/wechat-file-keeper/internal/guard"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/scan"
	"github.com/kennethfan/wechat-file-keeper/internal/trash"
	"github.com/kennethfan/wechat-file-keeper/internal/view"
)

// 分类树的顺序：按用户关心程度排列，图片缓存放最后（风险最高）。
var categoryOrder = []model.Category{
	model.CategoryVideo,
	model.CategoryDocument,
	model.CategoryArchive,
	model.CategoryOther,
	model.CategoryImageCache,
}

// App 是界面实例。
type App struct {
	window fyne.Window
	state  *view.State
	lib    account.Library

	// 账号库下拉。
	librarySelect *widget.Select
	// libraries 承载账号库列表与当前选择。
	libraries *view.Libraries

	// 左侧分类树。
	categoryList *widget.List
	categories   []model.Category

	// 右侧条目表。
	entryList *widget.List

	// 底部状态栏。
	selectionLabel *widget.Label
	statusLabel    *widget.Label
	cleanButton    *widget.Button

	// manifestDir 是清理清单目录。
	manifestDir string
}

// Run 启动界面。
//
// libPath 非空时锁定该账号库（不提供切换）；否则枚举全部账号库，
// 默认打开实占最大的那个。
func Run(root, libPath string) error {
	dataRoot := root
	if libPath == "" {
		if dataRoot == "" {
			defaultRoot, err := account.Root()
			if err != nil {
				return err
			}
			dataRoot = defaultRoot
		}
	}

	a := app.New()
	a.Settings().SetTheme(nil)
	w := a.NewWindow("微信文件管家")
	w.Resize(fyne.NewSize(1000, 680))

	ui := &App{window: w, manifestDir: defaultManifestDir()}
	ui.build()

	if libPath != "" {
		// 显式指定账号库：跳过枚举，直接扫这一个。
		ui.lib = account.Library{Path: libPath, WXID: filepath.Base(libPath)}
		ui.librarySelect.SetOptions([]string{ui.lib.WXID})
		ui.librarySelect.SetSelectedIndex(0)
		ui.librarySelect.Disable()
		ui.refresh()
	} else {
		ui.loadLibraries(dataRoot)
	}

	w.ShowAndRun()
	return nil
}

// defaultManifestDir 返回清理清单目录，与 CLI 保持一致。
func defaultManifestDir() string {
	home, err := userHome()
	if err != nil {
		return ".wechat-file-keeper"
	}
	return filepath.Join(home, "Library", "Application Support", "wechat-file-keeper", "manifests")
}

// build 组装界面。
func (a *App) build() {
	a.state = view.NewState(nil)

	// 左侧：分类树。
	a.categories = categoryOrder
	a.categoryList = widget.NewList(
		func() int { return len(a.categories) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(a.categories) {
				return
			}
			c := a.categories[id]
			totals := a.state.CategoryTotals()
			obj.(*widget.Label).SetText(
				view.CategoryLabel(c, totals[c], format.Bytes))
		},
	)
	a.categoryList.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(a.categories) {
			return
		}
		a.state.SetCategory(a.categories[id])
		a.entryList.Refresh()
		a.updateFooter()
	}

	// 右侧：按月分组的条目列表。
	// 组头行与条目行混在同一个 List 里，靠 a.displayRows 展开得到。
	a.entryList = widget.NewList(
		func() int { return len(a.displayRows()) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewCheck("", nil),
				widget.NewLabel(""),
				widget.NewButton("", nil),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			rows := a.displayRows()
			if id < 0 || id >= len(rows) {
				return
			}
			item := rows[id]
			box := obj.(*fyne.Container)
			check := box.Objects[0].(*widget.Check)
			label := box.Objects[1].(*widget.Label)
			toggle := box.Objects[2].(*widget.Button)

			// 先解绑回调再设状态，避免 SetChecked 触发回调造成反复切换。
			check.OnChanged = nil
			toggle.OnTapped = nil

			if item.isHeader {
				a.renderGroupHeader(check, label, toggle, item.group)
				return
			}
			a.renderEntryRow(check, label, toggle, item.rowIndex, item.row)
		},
	)

	// 顶部：账号库下拉与刷新。
	a.librarySelect = widget.NewSelect(nil, func(choice string) {
		a.onLibrarySelected(choice)
	})
	refreshBtn := widget.NewButton("重新扫描", func() {
		a.refresh()
	})

	// 底部：选择摘要、状态、清理按钮。
	a.selectionLabel = widget.NewLabel("已选 0 项，合计 0 B")
	a.statusLabel = widget.NewLabel("")
	a.cleanButton = widget.NewButton("移入废纸篓", a.onClean)

	footer := container.NewBorder(nil, nil,
		a.selectionLabel,
		container.NewHBox(a.statusLabel, a.cleanButton),
	)

	split := container.NewHSplit(
		container.NewBorder(widget.NewLabel("分类"), nil, nil, nil, a.categoryList),
		a.entryList,
	)
	split.SetOffset(0.26)

	// 顶部工具条放账号库下拉与刷新按钮，底部放状态栏。
	toolbar := container.NewHBox(
		widget.NewLabel("账号库"),
		a.librarySelect,
		refreshBtn,
	)
	footerBox := container.NewVBox(widget.NewSeparator(), footer)

	a.window.SetContent(container.NewBorder(toolbar, footerBox, nil, nil, split))
}

// refresh 重新扫描并刷新界面。
//
// 扫描放在后台 goroutine 里，避免大账号库把界面卡住。Fyne 要求所有
// 控件操作都在主线程执行，因此后台协程里的界面更新一律经 fyne.Do 派发。
func (a *App) refresh() {
	// 账号库尚未确定时（枚举还没完成）不扫描，避免扫一个空路径。
	if a.lib.Path == "" {
		a.statusLabel.SetText("正在查找微信账号库…")
		return
	}

	a.state.Scanning = true
	a.state.WeChatRunning = guard.IsWeChatRunning()
	a.updateFooter()

	go func() {
		res, err := scan.New(a.lib.Path).Run()

		fyne.Do(func() {
			a.state.Scanning = false
			if err != nil {
				a.statusLabel.SetText(fmt.Sprintf("扫描失败：%v", err))
				a.updateFooter()
				return
			}
			a.state.SetEntries(res.Entries)
			a.statusLabel.SetText(fmt.Sprintf("扫描完成，%d 条，用时 %s",
				len(res.Entries), res.Duration.Round(1e6)))
			a.categoryList.Refresh()
			a.entryList.Refresh()
			a.updateFooter()
		})
	}()
}

// updateFooter 刷新底部摘要与按钮可用状态。
func (a *App) updateFooter() {
	count, bytes := a.state.SelectionSummary()
	a.selectionLabel.SetText(fmt.Sprintf("已选 %d 项，合计 %s",
		count, format.Bytes(bytes)))

	a.cleanButton.Disable()
	if a.state.CanClean() {
		a.cleanButton.Enable()
	}
	if reason := a.state.CleanBlockReason(); reason != "" {
		a.statusLabel.SetText(reason)
	}
}

// onClean 执行清理：确认后移入废纸篓并刷新。
func (a *App) onClean() {
	selected := a.state.SelectedEntries()
	if len(selected) == 0 {
		return
	}

	count, bytes := a.state.SelectionSummary()
	message := fmt.Sprintf("将 %d 项（%s）移入废纸篓？\n\n可在命令行用 undo 恢复。",
		count, format.Bytes(bytes))

	// 图片缓存是本地唯一副本，需要额外警告。
	if a.state.Category == model.CategoryImageCache {
		message += "\n\n注意：图片缓存是本地唯一副本，微信服务器有过期策略，" +
			"较早的缓存删除后可能无法重新下载。"
	}

	dialog.ShowConfirm("确认清理", message, func(confirmed bool) {
		if !confirmed {
			return
		}
		a.doClean(selected)
	}, a.window)
}

// doClean 真正执行清理。
func (a *App) doClean(selected []model.Entry) {
	msgRoot := filepath.Join(a.lib.Path, "msg")
	includeCache := a.state.Category == model.CategoryImageCache

	plan := clean.Build(clean.Options{
		Entries:      selected,
		MsgRoot:      msgRoot,
		IncludeCache: includeCache,
	})

	cleaner := trash.New(a.manifestDir, a.lib.Path)
	m, err := clean.Apply(plan, msgRoot, a.state.WeChatRunning, cleaner)
	if err != nil {
		dialog.ShowError(err, a.window)
		a.updateFooter()
		return
	}

	a.state.LastFreedBytes = m.TotalBytes()
	dialog.ShowInformation("清理完成",
		fmt.Sprintf("已移入废纸篓 %d 项，释放 %s", len(m.Records), format.Bytes(m.TotalBytes())),
		a.window)
	a.refresh()
}

// loadLibraries 枚举账号库并填充下拉，随后扫描默认账号库。
//
// 枚举要统计每个账号库的大小，可能耗时，因此放在后台协程；
// 界面更新经 fyne.Do 派发回主线程。
func (a *App) loadLibraries(root string) {
	go func() {
		options, err := enumerateLibraries(root)
		fyne.Do(func() {
			if err != nil {
				a.statusLabel.SetText(fmt.Sprintf("未找到微信账号库：%v", err))
				a.updateFooter()
				return
			}

			a.libraries = view.NewLibraries(options)
			labels := a.libraries.Labels()
			a.librarySelect.Options = labels
			a.librarySelect.Refresh()

			if !a.libraries.Switchable() {
				// 单账号库时禁用下拉，但仍显示识别到的账号。
				a.librarySelect.Disable()
			}
			if current, ok := a.libraries.CurrentOption(); ok {
				a.librarySelect.SetSelectedIndex(a.libraries.Current)
				a.lib = current.Library
				a.refresh()
				return
			}

			// 没有任何账号库：显示空状态而不是崩溃。
			a.state.SetEntries(nil)
			a.statusLabel.SetText("未找到微信账号库，请确认微信已登录并产生过聊天文件")
			a.entryList.Refresh()
			a.categoryList.Refresh()
			a.updateFooter()
		})
	}()
}

// onLibrarySelected 处理账号库切换。
func (a *App) onLibrarySelected(choice string) {
	if a.libraries == nil {
		return
	}
	index := -1
	for i, label := range a.libraries.Labels() {
		if label == choice {
			index = i
			break
		}
	}
	// 选择未真正变化时不做任何事，避免无谓重扫。
	if !a.libraries.Select(index) {
		return
	}

	current, ok := a.libraries.CurrentOption()
	if !ok {
		return
	}
	a.lib = current.Library

	// 切换账号必须清空选择：上个账号的勾选对新账号毫无意义，
	// 留着会让用户在不知情下删掉另一个账号的文件。
	a.state.SelectAll(false)
	a.state.SetEntries(nil)
	a.entryList.Refresh()
	a.statusLabel.SetText("已切换账号，正在扫描…")
	a.refresh()
}

// displayItem 是列表里的一行：要么是组头，要么是条目。
type displayItem struct {
	// isHeader 为 true 表示这是组头行。
	isHeader bool
	// group 在组头行有效。
	group view.Group
	// row 在条目行有效。
	row view.Row
	// rowIndex 是该条目在 state.Rows 中的下标，勾选时用。
	rowIndex int
}

// displayRows 把分组结果展开成扁平的行列表。
//
// 折叠的组只贡献组头一行；展开的组贡献组头 + 全部条目。
// 条目行携带它在 state.Rows 里的真实下标，这样勾选操作的
// 语义与逐行勾选完全一致。
func (a *App) displayRows() []displayItem {
	groups := a.state.Groups()
	items := make([]displayItem, 0, len(a.state.Rows)+len(groups))
	for _, g := range groups {
		items = append(items, displayItem{isHeader: true, group: g})
		if g.Collapsed {
			continue
		}
		for _, r := range g.Rows {
			items = append(items, displayItem{
				row:      r,
				rowIndex: a.indexOfRow(r),
			})
		}
	}
	return items
}

// indexOfRow 在 state.Rows 中定位某条记录的下标。
//
// 用路径匹配而非指针比较：Row 是值类型，分组快照里的是副本。
func (a *App) indexOfRow(target view.Row) int {
	for i, r := range a.state.Rows {
		if r.Entry.Path == target.Entry.Path {
			return i
		}
	}
	return -1
}

// renderGroupHeader 渲染组头：折叠按钮 + 三态勾选 + 月份与统计。
func (a *App) renderGroupHeader(check *widget.Check, label *widget.Label, toggle *widget.Button, g view.Group) {
	arrow := "▾"
	if g.Collapsed {
		arrow = "▸"
	}
	// Fyne 的 Check 没有三态，部分选中时同样打勾，但文字标出「部分」，
	// 避免用户误以为整月已全选。
	check.SetChecked(g.Selection() != view.SelectionNone)
	partial := ""
	if g.Selection() == view.SelectionPartial {
		partial = "（部分）"
	}
	label.SetText(fmt.Sprintf("%s  ·  %d 项  ·  %s%s",
		g.Month, g.Count(), format.Bytes(g.Bytes()), partial))
	label.TextStyle = fyne.TextStyle{Bold: true}

	month := g.Month
	toggle.SetText(arrow)
	toggle.OnTapped = func() {
		a.state.ToggleGroup(month)
		a.entryList.Refresh()
	}
	check.OnChanged = func(checked bool) {
		a.state.SelectGroup(month, checked)
		a.entryList.Refresh()
		a.updateFooter()
	}
}

// renderEntryRow 渲染一个条目行。
func (a *App) renderEntryRow(check *widget.Check, label *widget.Label, toggle *widget.Button, rowIndex int, row view.Row) {
	toggle.SetText("")
	toggle.Hide()
	label.TextStyle = fyne.TextStyle{}

	if rowIndex < 0 {
		check.SetChecked(false)
		label.SetText("")
		return
	}
	check.SetChecked(row.Selected)

	idx := rowIndex
	check.OnChanged = func(checked bool) {
		if checked != a.state.Rows[idx].Selected {
			a.state.Toggle(idx)
			a.updateFooter()
		}
	}

	marker := ""
	if row.Entry.IsOrphanThumbnail {
		marker = "  [孤儿缩略图]"
	}
	label.SetText(fmt.Sprintf("      %-10s %s%s",
		format.Bytes(row.Entry.Size),
		filepath.Base(row.Entry.Path),
		marker))
}
