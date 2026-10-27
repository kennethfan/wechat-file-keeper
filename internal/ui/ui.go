// Package ui 是 Fyne 界面层，只负责渲染与转发用户操作。
//
// 所有状态与判断都在 internal/view 里，这里不重复实现业务逻辑，
// 以保证 GUI 与 CLI 的行为一致。
package ui

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kennethfan/wechat-file-keeper/internal/account"
	"github.com/kennethfan/wechat-file-keeper/internal/clean"
	"github.com/kennethfan/wechat-file-keeper/internal/format"
	"github.com/kennethfan/wechat-file-keeper/internal/guard"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/opener"
	"github.com/kennethfan/wechat-file-keeper/internal/preview"
	"github.com/kennethfan/wechat-file-keeper/internal/scan"
	"github.com/kennethfan/wechat-file-keeper/internal/thumb"
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

	// 右侧条目表（分组列表）。
	entryList *widget.List
	// detailTable 是多列表格视图。
	detailTable *widget.Table
	// modeSelect 是视图模式切换控件。
	modeSelect *widget.Select
	// lastSortField 记录上次排序字段，用于判断是否反转方向。
	lastSortField view.SortField
	// lastSortDescending 记录上次排序方向。
	lastSortDescending bool
	// syncingMode 标记正在同步切换控件，防止 OnChanged 回调递归。
	syncingMode bool

	// 预览面板控件。
	previewTitle    *widget.Label
	previewMeta     *widget.Label
	previewImage    *canvas.Image
	previewIcon     *canvas.Image
	previewIconName *widget.Label
	previewHint     *widget.Label
	previewOpen     *widget.Button
	previewReveal   *widget.Button
	previewScroll   *container.Scroll
	// previewEntry 是当前正在预览的条目，用于丢弃过期结果。
	previewEntry model.Entry
	// lastTapID 与 lastTapAt 用于判定双击。
	lastTapID widget.TableCellID
	lastTapAt time.Time
	// gridTable 是虚拟化的封面网格：只渲染可见格子。
	gridTable *widget.Table
	// gridCols 是网格列数，随窗口宽度调整。
	gridCols int
	// rightPane 在网格与列表之间切换。
	rightPane *fyne.Container
	// thumbs 是按需加载的封面缓存。
	thumbs *thumb.Loader
	// pending 记录正在解码的封面，避免同一张图重复排队。
	pending map[string]struct{}

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

	// 退出时清理打开图片缓存产生的临时文件，避免长期占用磁盘。
	_ = opener.CleanTemp()
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
		a.rebuildRightPane()
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
	//
	// Fyne 的中文语言包缺少 Select 的默认占位文案，会打出无害的
	// Translation failure 警告。这里显式设一个占位文案绕开它。
	a.librarySelect = widget.NewSelect(nil, func(choice string) {
		a.onLibrarySelected(choice)
	})
	a.librarySelect.PlaceHolder = "选择账号库"
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

	// 右侧是堆叠容器：视频用封面网格，其余分类用分组列表。
	a.gridTable = widget.NewTable(
		func() (int, int) {
			rows := len(a.state.Rows)
			cols := a.gridCols
			if cols < 1 {
				cols = 1
			}
			// 行数按列数向上取整，最后一行不足时留空。
			n := (rows + cols - 1) / cols
			return n, cols
		},
		func() fyne.CanvasObject { return a.newThumbCell() },
		func(id widget.TableCellID, obj fyne.CanvasObject) { a.updateThumbCell(id, obj) },
	)
	a.gridTable.CreateHeader = nil
	// 点击格子即切换勾选。用 Table 自带的选中回调，而不是在格子里放按钮：
	// 按钮的 Show/Hide 会触发容器重新布局，进而在渲染回调里引发重绘循环。
	a.gridTable.OnSelected = func(id widget.TableCellID) {
		row, ok := a.rowAtCell(id)
		if !ok {
			return
		}
		idx := a.state.IndexOf(row.Entry.Path)
		if idx < 0 || idx >= len(a.state.Rows) {
			return
		}
		if a.isDoubleTap(id) {
			a.openEntry(a.state.Rows[idx].Entry)
			return
		}

		a.state.Toggle(idx)
		a.updateFooter()
		// 点格子的同时更新预览面板。
		a.showPreview(a.state.Rows[idx].Entry)
		// 只刷新被点的格子，避免整表重绘打乱滚动位置。
		a.gridTable.RefreshItem(id)
	}
	a.updateGridCols()
	// 表格视图：大小、月份、类型、文件名四列。
	a.detailTable = widget.NewTable(
		// 第 0 行是表头，数据从第 1 行开始。
		func() (int, int) { return len(a.state.Rows) + 1, len(tableHeaders) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return label
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if id.Row == 0 {
				// 表头行：加粗显示列名。
				if id.Col >= 0 && id.Col < len(tableHeaders) {
					label.SetText(tableHeaders[id.Col])
					label.TextStyle = fyne.TextStyle{Bold: true}
				}
				return
			}
			label.TextStyle = fyne.TextStyle{}
			dataRow := id.Row - 1
			if dataRow < 0 || dataRow >= len(a.state.Rows) {
				label.SetText("")
				return
			}
			label.SetText(a.tableCellText(dataRow, id.Col))
		},
	)
	for i, w := range tableColWidths {
		a.detailTable.SetColumnWidth(i, w)
	}
	a.detailTable.SetRowHeight(0, 30)
	// 点击列头排序：大小与月份两列可排，其余列保持原顺序。
	a.detailTable.OnSelected = func(id widget.TableCellID) {
		a.onTableSelected(id)
	}
	a.detailTable.Hide()

	a.rightPane = container.NewStack(a.entryList, a.detailTable, a.gridTable)
	a.gridTable.Hide()

	// 三段布局：左侧分类树 | 中间条目 | 右侧预览面板。
	midSplit := container.NewHSplit(a.rightPane, a.buildPreviewPanel())
	midSplit.SetOffset(0.68)

	split := container.NewHSplit(
		container.NewBorder(widget.NewLabel("分类"), nil, nil, nil, a.categoryList),
		midSplit,
	)
	split.SetOffset(0.20)

	// 顶部工具条放账号库下拉与刷新按钮，底部放状态栏。
	a.modeSelect = widget.NewSelect(modeLabels(), func(choice string) {
		// 同步控件状态时触发的回调直接忽略，否则会无限递归。
		if a.syncingMode {
			return
		}
		if !view.IsValidMode(choice) {
			return
		}
		a.state.SetCurrentMode(view.ViewMode(choice))
		a.rebuildRightPane()
	})
	a.modeSelect.PlaceHolder = "视图"
	a.modeSelect.SetSelected(string(a.state.CurrentMode()))

	toolbar := container.NewHBox(
		widget.NewLabel("账号库"),
		a.librarySelect,
		widget.NewLabel("视图"),
		a.modeSelect,
		refreshBtn,
	)
	footerBox := container.NewVBox(widget.NewSeparator(), footer)

	a.window.SetContent(container.NewBorder(toolbar, footerBox, nil, nil, split))

	// 初始为空视图，按当前分类（视频）选好右侧呈现方式。
	a.rebuildRightPane()
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
			a.rebuildRightPane()
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
				rowIndex: a.state.IndexOf(r.Entry.Path),
			})
		}
	}
	return items
}

// indexOfRow 在 state.Rows 中定位某条记录的下标。
//
// 走 State 内部的索引表，O(1)。这是渲染热路径：每个可见格子都要查一次，
// 线性扫描会让滚动退化成 O(n²)，几屏之后就卡死。
func (a *App) indexOfRow(target view.Row) int {
	return a.state.IndexOf(target.Entry.Path)
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
		// 勾选时也更新预览，让用户确认勾的是哪个文件。
		if idx >= 0 && idx < len(a.state.Rows) {
			a.showPreview(a.state.Rows[idx].Entry)
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

// useGrid 判断当前分类是否应该用封面网格呈现。
//
// useGrid 判断当前是否应该用封面网格呈现。
//
// 由用户选择的视图模式决定，不再按分类硬编码：图片缓存也有缩略图，
// 文档有时也想看封面。
func (a *App) useGrid() bool {
	return a.state.CurrentMode() == view.ModeGrid
}

// rebuildRightPane 按当前分类切换网格与列表。
func (a *App) rebuildRightPane() {
	mode := a.state.CurrentMode()
	if a.modeSelect != nil && !a.syncingMode {
		// 同步切换控件，避免用户切分类后按钮状态与实际视图不一致。
		// 置标志防止 SetSelected 触发 OnChanged 再次调用本函数，形成递归。
		a.syncingMode = true
		a.modeSelect.SetSelected(string(mode))
		a.syncingMode = false
	}

	switch mode {
	case view.ModeGrid:
		a.entryList.Hide()
		a.detailTable.Hide()
		a.gridTable.Show()
		a.gridTable.Refresh()
	case view.ModeTable:
		a.gridTable.Hide()
		a.entryList.Hide()
		a.detailTable.Show()
		a.detailTable.Refresh()
	default:
		a.gridTable.Hide()
		a.detailTable.Hide()
		a.entryList.Show()
		a.entryList.Refresh()
	}
}

// updateGridCols 按右侧区域宽度计算网格列数。
//
// 格子固定 160px 宽，列数 = 可用宽度 / 160，至少 1 列。
// 列数变化会让表格重建，所以只在真正变化时才更新。
func (a *App) updateGridCols() {
	if a.window == nil || a.window.Canvas() == nil {
		if a.gridCols < 1 {
			a.gridCols = 4
		}
		return
	}
	// 右侧区域约占窗口宽度的 74%（左侧分类树 26%）。
	width := float32(a.window.Canvas().Size().Width) * 0.74
	cols := int(width / 160)
	if cols < 1 {
		cols = 1
	}
	a.gridCols = cols

	// 统一设置单元格尺寸，让表格知道如何布局。
	for col := 0; col < cols; col++ {
		a.gridTable.SetColumnWidth(col, 160)
	}
	a.gridTable.SetRowHeight(0, 190)
}

// newThumbCell 创建一个空的封面格子模板。
//
// widget.Table 会复用这些模板，只对可见格子调用 updateThumbCell，
// 因此即使有上千条视频也只会实例化屏幕上那几十个格子。
func (a *App) newThumbCell() fyne.CanvasObject {
	img := canvas.NewImageFromResource(nil)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(150, 120))

	caption := widget.NewLabel("")
	caption.TextStyle = fyne.TextStyle{Italic: true}
	caption.Truncation = fyne.TextTruncateEllipsis

	return container.NewStack(
		canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 0}),
		container.NewBorder(nil, caption, nil, nil, img),
	)
}

// updateThumbCell 填充第 id 个格子。
//
// 这里才真正去读封面文件：Table 只会为可见格子调用本方法，
// 所以滚动到哪加载到哪，不会一次性解码全部封面。
// 越界时返回 false——最后一行的格子数可能不足一整行。
func (a *App) rowAtCell(id widget.TableCellID) (view.Row, bool) {
	cols := a.gridCols
	if cols < 1 {
		cols = 1
	}
	index := id.Row*cols + id.Col
	if index < 0 || index >= len(a.state.Rows) {
		return view.Row{}, false
	}
	return a.state.Rows[index], true
}
func (a *App) updateThumbCell(id widget.TableCellID, obj fyne.CanvasObject) {
	box := obj.(*fyne.Container)
	bg := box.Objects[0].(*canvas.Rectangle)
	inner := box.Objects[1].(*fyne.Container)
	img := inner.Objects[0].(*canvas.Image)
	caption := inner.Objects[1].(*widget.Label)

	row, ok := a.rowAtCell(id)
	if !ok {
		// 末行不足时的空格子：清空内容。
		if img.Image != nil {
			img.Image = nil
			img.Refresh()
		}
		if caption.Text != "" {
			caption.SetText("")
		}
		if !isTransparent(bg.FillColor) {
			bg.FillColor = color.NRGBA{A: 0}
			bg.Refresh()
		}
		return
	}

	// 只在图片确实变化时赋值并 Refresh。
	//
	// 这一步是性能关键：Fyne 每次重绘都会调用本方法，若在这里无条件
	// Refresh，就会触发「重绘 → Refresh → 重绘」的无限循环，CPU 打满。
	want := a.requestThumb(id, row.Entry)
	if img.Image != want {
		img.Image = want
		img.Refresh()
	}

	marker := ""
	if row.Entry.IsOrphanThumbnail {
		marker = " [孤儿]"
	}
	text := fmt.Sprintf("%s  %s%s",
		format.Bytes(row.Entry.Size), row.Entry.Month, marker)
	if caption.Text != text {
		caption.SetText(text)
	}

	var fill color.Color = color.NRGBA{A: 0}
	if row.Selected {
		fill = color.NRGBA{R: 43, G: 196, B: 108, A: 60}
	}
	if !sameColor(bg.FillColor, fill) {
		bg.FillColor = fill
		bg.Refresh()
	}

}

// isTransparent 判断颜色是否完全透明。
func isTransparent(c color.Color) bool {
	if c == nil {
		return true
	}
	_, _, _, a := c.RGBA()
	return a == 0
}

// sameColor 比较两个颜色是否等价。
//
// 统一按 RGBA() 的 16 位分量比较，避免 color.NRGBA 与 color.RGBA
// 因具体类型不同而永远不相等，导致每帧都触发 Refresh。
func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// requestThumb 异步请求一张封面。
//
// 解码单张约 2.3ms，滚动一屏几十张就是上百毫秒——同步做会让 UI 明显卡顿。
// 这里先返回 nil 让格子显示占位色，解码完成后经 fyne.Do 回填并只刷新该格。
//
// pending 保证同一张图不会被重复排队：滚动来回时只会有一次解码在途。
func (a *App) requestThumb(id widget.TableCellID, e model.Entry) image.Image {
	if a.thumbs == nil {
		a.thumbs = thumb.NewLoader()
	}
	if pic, ok := a.thumbs.Cached(e); ok {
		return pic
	}
	if a.thumbs.Failed(e) {
		return nil
	}

	key := e.Path
	if a.pending == nil {
		a.pending = make(map[string]struct{})
	}
	if _, inflight := a.pending[key]; inflight {
		return nil
	}
	a.pending[key] = struct{}{}

	go func() {
		pic, ok := a.thumbs.Load(e)
		fyne.Do(func() {
			delete(a.pending, key)
			if !ok {
				return
			}
			_ = pic
			// 只刷新这一格，避免整表重绘把用户滚动位置打乱。
			a.gridTable.RefreshItem(id)
		})
	}()
	return nil
}

// tableHeaders 是表格视图的列标题。
var tableHeaders = []string{"大小", "月份", "类型", "文件名"}

// tableColWidths 是表格视图各列宽度，与 tableHeaders 一一对应。
var tableColWidths = []float32{90, 80, 70, 420}

// modeLabels 返回视图切换控件的选项文本。
func modeLabels() []string {
	out := make([]string, len(view.AllModes))
	for i, m := range view.AllModes {
		out[i] = string(m)
	}
	return out
}

// tableCellText 返回表格中某格应显示的文字。
func (a *App) tableCellText(row, col int) string {
	if row < 0 || row >= len(a.state.Rows) {
		return ""
	}
	e := a.state.Rows[row].Entry
	switch col {
	case 0:
		return format.Bytes(e.Size)
	case 1:
		if e.Month == "" {
			return "—"
		}
		return e.Month
	case 2:
		return string(e.Category)
	case 3:
		name := filepath.Base(e.Path)
		if e.IsOrphanThumbnail {
			name += " [孤儿]"
		}
		return name
	default:
		return ""
	}
}

// onTableSelected 处理表格点击：勾选条目，点列头则切换排序。
func (a *App) onTableSelected(id widget.TableCellID) {
	// Fyne 的 Table 没有独立列头，这里用第 0 行之外的第一行当作「表头行」，
	// 通过点击列号 0/1 的顶行触发排序。
	if id.Row == 0 {
		// 表头行：点大小或月份列切换排序。
		if id.Col <= 1 {
			a.toggleSortByColumn(id.Col)
		}
		return
	}

	dataRow := id.Row - 1
	if dataRow < 0 || dataRow >= len(a.state.Rows) {
		return
	}
	if a.isDoubleTap(id) {
		a.openEntry(a.state.Rows[dataRow].Entry)
		return
	}

	a.state.Toggle(dataRow)
	a.updateFooter()
	a.showPreview(a.state.Rows[dataRow].Entry)
	a.detailTable.RefreshItem(id)
}

// toggleSortByColumn 按列切换排序。
//
// 列 0 是大小，列 1 是月份；再次点击同一列时反转方向。
func (a *App) toggleSortByColumn(col int) {
	var field view.SortField
	switch col {
	case 0:
		field = view.SortBySize
	case 1:
		field = view.SortByMonth
	default:
		return
	}

	descending := true
	if a.lastSortField == field {
		descending = !a.lastSortDescending
	}
	a.lastSortField = field
	a.lastSortDescending = descending

	a.state.SetSort(field, descending)
	a.detailTable.Refresh()
}

// buildPreviewPanel 构建右侧预览面板。
//
// 面板由三部分组成：顶部的文件信息、中间的预览内容、底部的操作按钮。
// 预览内容在选中条目时按需生成，不预先批量处理。
func (a *App) buildPreviewPanel() fyne.CanvasObject {
	a.previewTitle = widget.NewLabel("未选中条目")
	a.previewTitle.TextStyle = fyne.TextStyle{Bold: true}
	a.previewTitle.Wrapping = fyne.TextWrapWord

	a.previewMeta = widget.NewLabel("")
	a.previewMeta.Wrapping = fyne.TextWrapWord

	a.previewImage = canvas.NewImageFromResource(nil)
	a.previewImage.FillMode = canvas.ImageFillContain
	a.previewImage.SetMinSize(fyne.NewSize(240, 200))

	// 非图片类型显示的占位图标：一个浅色方块，配上文件名。
	// 不追求真实文件类型图标，够表达「这里有个文件」即可。
	a.previewIcon = canvas.NewImageFromResource(theme.FileIcon())
	a.previewIcon.FillMode = canvas.ImageFillContain
	a.previewIcon.SetMinSize(fyne.NewSize(96, 96))
	a.previewIcon.Hide()

	a.previewIconName = widget.NewLabel("")
	a.previewIconName.Wrapping = fyne.TextWrapWord
	a.previewIconName.Alignment = fyne.TextAlignCenter
	a.previewIconName.Hide()

	a.previewHint = widget.NewLabel("在左侧选中一个条目即可预览")
	a.previewHint.Wrapping = fyne.TextWrapWord

	a.previewScroll = container.NewVScroll(container.NewVBox(
		container.NewCenter(a.previewIcon),
		container.NewCenter(a.previewIconName),
		a.previewImage,
		a.previewHint,
	))

	a.previewOpen = widget.NewButton("打开", func() {
		if a.previewEntry.Path != "" {
			a.openEntry(a.previewEntry)
		}
	})
	a.previewReveal = widget.NewButton("在 Finder 中显示", func() {
		if a.previewEntry.Path != "" {
			a.revealEntry(a.previewEntry)
		}
	})

	actions := container.NewHBox(a.previewOpen, a.previewReveal)

	return container.NewBorder(
		container.NewVBox(a.previewTitle, a.previewMeta, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), actions),
		nil, nil,
		a.previewScroll,
	)
}

// showPreview 为指定条目更新预览面板。
// showPreview 为指定条目更新预览面板。
//
// 只做两类真实预览：图片直接显示、视频显示封面。
// 其余类型显示文件类型图标与文件名——不做内容级预览，
// 避免调外部工具带来的超时与权限问题。
func (a *App) showPreview(e model.Entry) {
	a.previewEntry = e

	a.previewTitle.SetText(filepath.Base(e.Path))
	a.previewMeta.SetText(fmt.Sprintf("%s · %s · %s",
		format.Bytes(e.Size), monthOrDash(e.Month), e.Category))

	result := preview.Detect(e)

	// 先清空旧内容，避免残留上一个条目的图。
	a.previewImage.Image = nil
	a.previewImage.Refresh()
	a.previewImage.Hide()
	a.previewIcon.Hide()
	a.previewIconName.Hide()
	a.previewHint.Hide()

	switch result.Kind {
	case preview.KindIcon:
		// 图标 + 文件名：让用户知道这是什么类型的文件。
		a.previewIconName.SetText(filepath.Base(e.Path))
		a.previewIconName.Show()
		a.previewIcon.Show()
		if result.Reason != "" {
			a.previewHint.SetText(result.Reason)
			a.previewHint.Show()
		}
		return
	}

	// 图片与视频封面都走后台解码，避免阻塞界面。
	a.previewHint.SetText("正在加载预览…")
	a.previewHint.Show()

	go func() {
		img, err := a.loadPreviewImage(e, result.Kind)
		fyne.Do(func() {
			// 用户可能已点了别的条目，过期结果直接丢弃。
			if a.previewEntry.Path != e.Path {
				return
			}
			a.previewHint.Hide()
			if err != nil {
				a.previewIconName.SetText(filepath.Base(e.Path))
				a.previewIconName.Show()
				a.previewIcon.Show()
				a.previewHint.SetText(fmt.Sprintf("无法显示预览：%v", err))
				a.previewHint.Show()
				return
			}
			a.previewImage.Image = img
			a.previewImage.Show()
			a.previewImage.Refresh()
		})
	}()
}

// loadPreviewImage 按预览类型取得图片。
func (a *App) loadPreviewImage(e model.Entry, kind preview.Kind) (image.Image, error) {
	if kind == preview.KindVideoThumb {
		if a.thumbs == nil {
			a.thumbs = thumb.NewLoader()
		}
		img, ok := a.thumbs.Load(e)
		if !ok {
			return nil, fmt.Errorf("该视频没有封面图")
		}
		return img, nil
	}
	return decodeImage(e.Path)
}

// monthOrDash 返回月份，空值时显示破折号。
func monthOrDash(month string) string {
	if month == "" {
		return "—"
	}
	return month
}

// decodeImage 读取并解码图片文件。
func decodeImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("无法解码图片: %w", err)
	}
	return img, nil
}

// openEntry 用系统默认程序打开条目。
func (a *App) openEntry(e model.Entry) {
	if err := opener.Open(e); err != nil {
		dialog.ShowError(fmt.Errorf("无法打开 %s\n\n%v", filepath.Base(e.Path), err), a.window)
	}
}

// revealEntry 在 Finder 中定位条目。
func (a *App) revealEntry(e model.Entry) {
	if err := opener.Reveal(e); err != nil {
		dialog.ShowError(fmt.Errorf("无法定位 %s\n\n%v", filepath.Base(e.Path), err), a.window)
	}
}

// buildEntryMenu 构建条目右键菜单。
func (a *App) buildEntryMenu(e model.Entry) *fyne.Menu {
	return fyne.NewMenu("",
		fyne.NewMenuItem("打开", func() { a.openEntry(e) }),
		fyne.NewMenuItem("在 Finder 中显示", func() { a.revealEntry(e) }),
	)
}

// isDoubleTap 判断两次点击是否构成双击。
//
// Fyne 的 Table 没有双击事件，这里用时间间隔判定：同一格子在
// 400ms 内被点两次即视为双击。记录的是上一次点击的格子和时间。
func (a *App) isDoubleTap(id widget.TableCellID) bool {
	now := time.Now()
	prev := a.lastTapID
	sameCell := prev.Row == id.Row && prev.Col == id.Col
	recent := now.Sub(a.lastTapAt) < 400*time.Millisecond

	a.lastTapID = id
	a.lastTapAt = now
	return sameCell && recent
}
