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
// libPath 非空时直接使用该账号库；否则自动发现实占最大的账号库。
func Run(root, libPath string) error {
	lib, err := resolve(root, libPath)
	if err != nil {
		return err
	}

	a := app.New()
	a.Settings().SetTheme(nil)
	w := a.NewWindow("微信文件管家")
	w.Resize(fyne.NewSize(1000, 680))

	ui := &App{window: w, lib: lib, manifestDir: defaultManifestDir()}
	ui.build()
	ui.refresh()

	w.ShowAndRun()
	return nil
}

// resolve 解析要打开的账号库。
func resolve(root, libPath string) (account.Library, error) {
	if libPath != "" {
		return account.Library{Path: libPath, WXID: filepath.Base(libPath)}, nil
	}
	if root == "" {
		defaultRoot, err := account.Root()
		if err != nil {
			return account.Library{}, err
		}
		root = defaultRoot
	}
	return account.Largest(root)
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

	// 右侧：条目表，带勾选框。
	a.entryList = widget.NewList(
		func() int { return len(a.state.Rows) },
		func() fyne.CanvasObject {
			return container.NewHBox(
				widget.NewCheck("", nil),
				widget.NewLabel(""),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(a.state.Rows) {
				return
			}
			row := a.state.Rows[id]
			box := obj.(*fyne.Container)
			check := box.Objects[0].(*widget.Check)
			label := box.Objects[1].(*widget.Label)

			check.OnChanged = nil
			check.SetChecked(row.Selected)
			check.OnChanged = func(checked bool) {
				// 只在状态确实不同时翻转，避免 Refresh 触发回调导致反复切换。
				if checked != a.state.Rows[id].Selected {
					a.state.Toggle(id)
					a.updateFooter()
				}
			}

			marker := ""
			if row.Entry.IsOrphanThumbnail {
				marker = "  [孤儿缩略图]"
			}
			label.SetText(fmt.Sprintf("%-10s %-9s %s%s",
				format.Bytes(row.Entry.Size),
				row.Entry.Month,
				filepath.Base(row.Entry.Path),
				marker))
		},
	)

	// 顶部：账号库信息与刷新。
	header := widget.NewLabel(fmt.Sprintf("账号库  %s", a.lib.WXID))
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
		container.NewBorder(header, nil, nil, nil, a.entryList),
	)
	split.SetOffset(0.26)

	// 顶部工具条放刷新按钮，底部放状态栏。
	toolbar := container.NewHBox(refreshBtn)
	footerBox := container.NewVBox(widget.NewSeparator(), footer)

	a.window.SetContent(container.NewBorder(toolbar, footerBox, nil, nil, split))
}

// refresh 重新扫描并刷新界面。
//
// 扫描放在后台 goroutine 里，避免大账号库把界面卡住。Fyne 要求所有
// 控件操作都在主线程执行，因此后台协程里的界面更新一律经 fyne.Do 派发。
func (a *App) refresh() {
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
