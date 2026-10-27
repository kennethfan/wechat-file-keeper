// Command wechat-file-keeper 是 macOS 微信本地文件管理工具。
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kennethfan/wechat-file-keeper/internal/account"
	"github.com/kennethfan/wechat-file-keeper/internal/cachestat"
	"github.com/kennethfan/wechat-file-keeper/internal/clean"
	"github.com/kennethfan/wechat-file-keeper/internal/dup"
	"github.com/kennethfan/wechat-file-keeper/internal/format"
	"github.com/kennethfan/wechat-file-keeper/internal/guard"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/scan"
	"github.com/kennethfan/wechat-file-keeper/internal/trash"
	"github.com/kennethfan/wechat-file-keeper/internal/ui"
	"github.com/kennethfan/wechat-file-keeper/internal/undo"
)

// usage 打印命令帮助。
func usage() {
	fmt.Fprint(os.Stderr, `微信文件管家 — 只读扫描、显式确认才动手

用法:
  wechat-file-keeper scan                          扫描账号库并显示空间统计
  wechat-file-keeper list [选项]                    列出条目，可按分类与大小筛选
  wechat-file-keeper dup                           列出真重复分组
  wechat-file-keeper cache                         显示图片缓存年份分布
  wechat-file-keeper clean [选项]                   把选中项移入废纸篓（默认仅预览）
  wechat-file-keeper undo [--manifest <文件>]       按清单把文件搬回原位
  wechat-file-keeper gui                            打开图形界面

选项:
  --root <路径>        指定微信数据根目录（默认为本机标准位置）
  --lib <路径>         直接指定账号库目录，跳过自动发现
  --category <分类>    list/clean 用：视频 / 文档 / 压缩包 / 其他 / 图片缓存
  --min-size <大小>    list/clean 用：最小字节数，支持 100MB / 1GB 写法
  --limit <n>          list 用：最多显示条数（默认 30）
  --orphans            clean 用：只清理孤儿缩略图
  --include-cache      clean 用：允许清理图片缓存（本地唯一副本，谨慎）
  --yes                clean 用：确认执行，否则只打印计划
  -h, --help           显示本帮助
`)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		os.Exit(1)
	}
}

// run 分发子命令。
func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("缺少子命令")
	}

	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	root := fs.String("root", "", "微信数据根目录")
	libPath := fs.String("lib", "", "直接指定账号库目录")
	category := fs.String("category", "", "按分类筛选")
	minSize := fs.String("min-size", "", "最小字节数，支持 100MB / 1GB 写法")
	limit := fs.Int("limit", 30, "最多显示条数")
	orphansOnly := fs.Bool("orphans", false, "只清理孤儿缩略图")
	includeCache := fs.Bool("include-cache", false, "允许清理图片缓存")
	yes := fs.Bool("yes", false, "确认执行")
	manifestPath := fs.String("manifest", "", "指定要回滚的清单文件")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}

	switch cmd {
	case "scan":
		return runScan(*root, *libPath)
	case "list":
		return runList(*root, *libPath, *category, *minSize, *limit)
	case "dup":
		return runDup(*root, *libPath)
	case "cache":
		return runCache(*root, *libPath)
	case "clean":
		return runClean(*root, *libPath, *category, *minSize, *limit, *orphansOnly, *includeCache, *yes)
	case "undo":
		return runUndo(*manifestPath)
	case "gui":
		return ui.Run(*root, *libPath)
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("未知子命令 %q", cmd)
	}
}

// parseSize 解析 "100MB" / "1GB" / "512" 这类大小写法。
//
// 支持 B / KB / MB / GB 后缀（不区分大小写，允许省略 B），
// 一律按 1024 进制换算。
func parseSize(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	upper := strings.ToUpper(strings.TrimSpace(s))
	mult := int64(1)
	for _, suffix := range []struct {
		suffix string
		mult   int64
	}{
		{"GB", 1024 * 1024 * 1024},
		{"MB", 1024 * 1024},
		{"KB", 1024},
		{"B", 1},
	} {
		if strings.HasSuffix(upper, suffix.suffix) {
			mult = suffix.mult
			upper = strings.TrimSuffix(upper, suffix.suffix)
			break
		}
	}
	upper = strings.TrimSpace(upper)
	if upper == "" {
		return 0, fmt.Errorf("无法解析大小 %q", s)
	}

	var value float64
	if _, err := fmt.Sscanf(upper, "%g", &value); err != nil {
		return 0, fmt.Errorf("无法解析大小 %q", s)
	}
	if value < 0 {
		return 0, fmt.Errorf("大小不能为负: %q", s)
	}
	return int64(value * float64(mult)), nil
}

// runList 列出条目，支持按分类与最小大小筛选。
func runList(root, libPath, category, minSize string, limit int) error {
	threshold, err := parseSize(minSize)
	if err != nil {
		return err
	}

	lib, err := resolveLibrary(root, libPath)
	if err != nil {
		return err
	}
	res, err := scan.New(lib.Path).Run()
	if err != nil {
		return err
	}

	wantCat := strings.TrimSpace(category)
	var matched []model.Entry
	for _, e := range res.Entries {
		if wantCat != "" && string(e.Category) != wantCat {
			continue
		}
		if e.Size < threshold {
			continue
		}
		matched = append(matched, e)
	}
	scan.SortBySizeDesc(matched)

	if len(matched) == 0 {
		fmt.Println("没有符合条件的条目。")
		return nil
	}

	shown := len(matched)
	if limit > 0 && shown > limit {
		shown = limit
	}

	fmt.Printf("%-10s %-10s %s\n", "大小", "月份", "文件")
	fmt.Println(strings.Repeat("─", 78))
	for _, e := range matched[:shown] {
		marker := ""
		if e.IsOrphanThumbnail {
			marker = " [孤儿缩略图]"
		}
		fmt.Printf("%-10s %-10s %s%s\n",
			format.Bytes(e.Size), e.Month, shortName(e.Path), marker)
	}

	fmt.Println()
	fmt.Printf("共 %s 条，合计 %s", format.Count(len(matched)), format.Bytes(sumSizes(matched)))
	if shown < len(matched) {
		fmt.Printf("（仅显示前 %d 条，用 --limit 调整）", shown)
	}
	fmt.Println()
	return nil
}

// runDup 列出真重复分组。
func runDup(root, libPath string) error {
	lib, err := resolveLibrary(root, libPath)
	if err != nil {
		return err
	}
	res, err := scan.New(lib.Path).Run()
	if err != nil {
		return err
	}

	groups, err := dup.Finder{}.Find(res.Entries)
	if err != nil {
		return err
	}
	if len(groups) == 0 {
		fmt.Println("未发现真重复文件。")
		fmt.Println("注意：同 inode 的硬链接不算真重复——删除它们释放 0 字节。")
		return nil
	}

	for i, g := range groups {
		fmt.Printf("组 %d：%s × %d 份，可回收 %s\n",
			i+1, format.Bytes(g.Size), len(g.Entries), format.Bytes(g.Reclaimable()))
		for _, e := range g.Entries {
			fmt.Printf("    %s\n", shortName(e.Path))
		}
	}
	fmt.Println()
	fmt.Printf("合计 %d 组，可回收 %s\n",
		len(groups), format.Bytes(dup.TotalReclaimable(groups)))
	return nil
}

// runCache 显示图片缓存的年份分布。
func runCache(root, libPath string) error {
	lib, err := resolveLibrary(root, libPath)
	if err != nil {
		return err
	}
	res, err := scan.New(lib.Path).Run()
	if err != nil {
		return err
	}

	stats := cachestat.ByYear(res.Entries)
	if len(stats) == 0 {
		fmt.Println("未发现图片缓存。")
		return nil
	}

	var total int64
	var totalCount int
	fmt.Printf("%-8s %10s %12s\n", "年份", "文件数", "占用")
	fmt.Println(strings.Repeat("─", 34))
	for _, s := range stats {
		fmt.Printf("%-8s %10s %12s\n",
			s.Year, format.Count(s.Count), format.Bytes(s.Bytes))
		total += s.Bytes
		totalCount += s.Count
	}
	fmt.Println(strings.Repeat("─", 34))
	fmt.Printf("%-8s %10s %12s\n", "合计", format.Count(totalCount), format.Bytes(total))
	fmt.Println()
	fmt.Println("注意：图片缓存是本地唯一副本，微信服务器有过期策略，")
	fmt.Println("      较早年份的缓存删除后可能无法重新下载。")
	return nil
}

// sumSizes 汇总条目字节数。
func sumSizes(entries []model.Entry) int64 {
	var total int64
	for _, e := range entries {
		total += e.Size
	}
	return total
}

// shortName 返回用于展示的短名：文件名 + 月份目录，避免刷屏的绝对路径。
func shortName(path string) string {
	return pathBase(path)
}

// resolveLibrary 解析出要操作的账号库。
//
// 优先使用显式指定的账号库目录；否则在根目录下发现账号库并选实占最大的。
func resolveLibrary(root, libPath string) (account.Library, error) {
	if libPath != "" {
		info, err := os.Stat(libPath)
		if err != nil {
			return account.Library{}, fmt.Errorf("无法访问账号库 %s: %w", libPath, err)
		}
		if !info.IsDir() {
			return account.Library{}, fmt.Errorf("%s 不是目录", libPath)
		}
		return account.Library{Path: libPath, WXID: pathBase(libPath)}, nil
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

// pathBase 取路径最后一段，避免额外引入 path/filepath 别名混淆。
func pathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// runScan 执行只读扫描并打印统计。
func runScan(root, libPath string) error {
	lib, err := resolveLibrary(root, libPath)
	if err != nil {
		return err
	}

	res, err := scan.New(lib.Path).Run()
	if err != nil {
		return err
	}

	sp := res.Space
	fmt.Printf("账号库    %s\n", lib.WXID)
	fmt.Printf("路径      %s\n", lib.Path)
	fmt.Println()
	fmt.Printf("实占大小  %s\n", format.Bytes(sp.Actual))
	fmt.Printf("名义大小  %s\n", format.Bytes(sp.Nominal))
	if sp.HardlinkOvercount > 0 {
		fmt.Printf("硬链接虚增 %s  （同一 inode 挂了多个名字，删除不释放空间）\n",
			format.Bytes(sp.HardlinkOvercount))
	}
	fmt.Println()

	// 分类汇总，按字节数降序。
	byCat := cachestat.ByCategory(res.Entries)
	cats := make([]model.Category, 0, len(byCat))
	for c := range byCat {
		cats = append(cats, c)
	}
	sort.Slice(cats, func(i, j int) bool { return byCat[cats[i]] > byCat[cats[j]] })
	fmt.Println("分类占用")
	for _, c := range cats {
		fmt.Printf("  %-8s %s\n", c, format.Bytes(byCat[c]))
	}
	fmt.Println()

	// 可回收项。
	orphanCount, orphanBytes := cachestat.OrphanTotal(res.Entries)
	if orphanCount > 0 {
		fmt.Printf("孤儿缩略图 %s 个，%s（微信会按需重建）\n",
			format.Count(orphanCount), format.Bytes(orphanBytes))
	}

	groups, dupErr := dup.Finder{}.Find(res.Entries)
	if dupErr == nil && len(groups) > 0 {
		fmt.Printf("真重复     %s 组，可回收 %s\n",
			format.Count(len(groups)), format.Bytes(dup.TotalReclaimable(groups)))
	}

	fmt.Println()
	fmt.Printf("条目 %s 条 / 按 inode 归并 %s 个文件\n",
		format.Count(sp.EntryCount), format.Count(sp.FileCount))
	fmt.Printf("扫描耗时  %s\n", res.Duration.Round(1e6))
	return nil
}

// runClean 把选中项移入废纸篓。
//
// 默认只打印计划，必须加 --yes 才执行：清理是唯一会改动磁盘的操作，
// 不该因为少打一个参数就动文件。
func runClean(root, libPath, category, minSize string, limit int, orphansOnly, includeCache, yes bool) error {
	threshold, err := parseSize(minSize)
	if err != nil {
		return err
	}

	lib, err := resolveLibrary(root, libPath)
	if err != nil {
		return err
	}
	res, err := scan.New(lib.Path).Run()
	if err != nil {
		return err
	}

	wantCat := strings.TrimSpace(category)
	var picked []model.Entry
	for _, e := range res.Entries {
		if orphansOnly && !e.IsOrphanThumbnail {
			continue
		}
		if wantCat != "" && string(e.Category) != wantCat {
			continue
		}
		if e.Size < threshold {
			continue
		}
		picked = append(picked, e)
	}
	scan.SortBySizeDesc(picked)
	if limit > 0 && len(picked) > limit {
		picked = picked[:limit]
	}

	if len(picked) == 0 {
		fmt.Println("没有符合条件的条目。")
		return nil
	}

	plan := clean.Build(clean.Options{
		Entries:      picked,
		MsgRoot:      filepath.Join(lib.Path, "msg"),
		IncludeCache: includeCache,
	})

	// 预览：列出将要移动的条目。
	fmt.Printf("账号库 %s\n\n", lib.WXID)
	for _, e := range plan.Selected {
		fmt.Printf("  %-10s %-10s %s\n", format.Bytes(e.Size), e.Month, shortName(e.Path))
	}
	if len(plan.Skipped) > 0 {
		fmt.Println()
		for _, s := range plan.Skipped {
			fmt.Printf("  跳过 %s：%s\n", shortName(s.Entry.Path), s.Reason)
		}
	}
	fmt.Println()
	fmt.Printf("将移入废纸篓 %s 项，合计 %s\n",
		format.Count(len(plan.Selected)), format.Bytes(plan.TotalBytes))

	if !yes {
		fmt.Println()
		fmt.Println("这是预览。确认执行请加 --yes。")
		return nil
	}

	weChatRunning := guard.IsWeChatRunning()
	cleaner := trash.New(manifestDir(), lib.Path)
	m, applyErr := clean.Apply(plan, filepath.Join(lib.Path, "msg"), weChatRunning, cleaner)
	if applyErr != nil {
		return applyErr
	}

	fmt.Println()
	fmt.Printf("已移入废纸篓 %s 项，释放 %s\n",
		format.Count(len(m.Records)), format.Bytes(m.TotalBytes()))
	fmt.Printf("撤销：wechat-file-keeper undo\n")
	return nil
}

// manifestDir 返回清理清单的存放目录。
//
// 放在用户的应用支持目录下，而不是账号库内部——清单不该混进
// 被扫描的数据里，否则下次扫描会把它当成聊天文件。
func manifestDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".wechat-file-keeper"
	}
	return filepath.Join(home, "Library", "Application Support", "wechat-file-keeper", "manifests")
}

// runUndo 按清单把文件从废纸篓搬回原位。
//
// 默认回滚最近一次清理；也可用 --manifest 指定某份清单。
func runUndo(manifestPath string) error {
	dir := manifestDir()
	target := manifestPath
	if target == "" {
		latest, err := trash.Latest(dir)
		if err != nil {
			return fmt.Errorf("%w\n提示：清单目录为 %s", err, dir)
		}
		target = latest
	}

	m, err := trash.Load(target)
	if err != nil {
		return err
	}

	fmt.Printf("清单 %s\n", filepath.Base(target))
	fmt.Printf("创建于 %s，共 %s 条记录\n\n",
		m.CreatedAt, format.Count(len(m.Records)))

	res, err := undo.Apply(m)
	if err != nil {
		return err
	}

	for _, s := range res.Skipped {
		fmt.Printf("  跳过 %s：%s\n", shortName(s.Record.OriginalPath), s.Reason)
	}
	if len(res.Skipped) > 0 {
		fmt.Println()
	}

	fmt.Printf("已恢复 %s 项，合计 %s\n",
		format.Count(len(res.Restored)), format.Bytes(res.RestoredBytes))
	if len(res.Skipped) > 0 {
		fmt.Printf("未恢复 %s 项\n", format.Count(len(res.Skipped)))
	}
	return nil
}
