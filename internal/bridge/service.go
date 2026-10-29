// Package bridge 是 Wails 前端与 Go 后端之间的唯一接口。
//
// 设计原则：前端不直接碰文件系统，所有扫描、清理、撤销都经这里，
// 复用 internal/ 下已经测试过的实现。这里只做三件事：
// 把领域对象转成前端友好的结构、探测运行状态、把错误转成人话。
package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kennethfan/wechat-file-keeper/internal/account"
	"github.com/kennethfan/wechat-file-keeper/internal/cachestat"
	"github.com/kennethfan/wechat-file-keeper/internal/clean"
	"github.com/kennethfan/wechat-file-keeper/internal/dup"
	"github.com/kennethfan/wechat-file-keeper/internal/guard"
	"github.com/kennethfan/wechat-file-keeper/internal/model"
	"github.com/kennethfan/wechat-file-keeper/internal/opener"
	"github.com/kennethfan/wechat-file-keeper/internal/preview"
	"github.com/kennethfan/wechat-file-keeper/internal/scan"
	"github.com/kennethfan/wechat-file-keeper/internal/thumb"
	"github.com/kennethfan/wechat-file-keeper/internal/trash"
	"github.com/kennethfan/wechat-file-keeper/internal/undo"
)

// Service 是暴露给前端的服务。
type Service struct {
	// lib 是当前打开的账号库。
	lib account.Library
	// thumbs 复用封面加载器，避免每次请求都重新解码。
	thumbs *thumb.Loader
	// cached 是上次扫描的条目，供分页查询复用。
	cached []model.Entry
	// cachedLib 是缓存对应的账号库路径。
	cachedLib string
}

// New 创建服务实例。
func New() *Service {
	return &Service{thumbs: thumb.NewLoader()}
}

// EntryDTO 是传给前端的条目结构。
//
// 字段名用 camelCase，前端直接消费。Path 保留绝对路径供打开/定位用，
// RelPath 是相对账号库的短路径，用于界面展示。
type EntryDTO struct {
	Path        string `json:"path"`
	RelPath     string `json:"relPath"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	SizeText    string `json:"sizeText"`
	Month       string `json:"month"`
	Category    string `json:"category"`
	ModTime     int64  `json:"modTime"`
	DaysAgo     int    `json:"daysAgo"`
	IsThumbnail bool   `json:"isThumbnail"`
	IsOrphan    bool   `json:"isOrphan"`
	// Previewable 表示能否显示真实预览（图片/视频封面）。
	Previewable bool `json:"previewable"`
	// PreviewReason 在不可预览时说明原因。
	PreviewReason string `json:"previewReason"`
}

// ScanResult 是一次体检的完整结果。
type ScanResult struct {
	// Library 是账号库信息。
	Library LibraryDTO `json:"library"`
	// Total 是实占字节数。
	Total int64 `json:"total"`
	// Nominal 是按路径累加的字节数。
	Nominal int64 `json:"nominal"`
	// HardlinkOvercount 是硬链接虚增。
	HardlinkOvercount int64 `json:"hardlinkOvercount"`
	// TotalText 是格式化的实占大小。
	TotalText string `json:"totalText"`
	// EntryCount 是条目数。
	EntryCount int `json:"entryCount"`
	// DurationMs 是扫描耗时（毫秒）。
	DurationMs int64 `json:"durationMs"`
	// Categories 是按分类的汇总。
	Categories []CategoryDTO `json:"categories"`
	// Months 是按月份的汇总。
	Months []MonthDTO `json:"months"`
	// Orphans 是孤儿缩略图统计。
	Orphans OrphanDTO `json:"orphans"`
	// Duplicates 是真重复统计。
	Duplicates DuplicateDTO `json:"duplicates"`
	// CacheYears 是图片缓存年份分布。
	CacheYears []YearDTO `json:"cacheYears"`
	// WeChatRunning 表示微信是否在运行。
	WeChatRunning bool `json:"weChatRunning"`
	// ScannedAt 是扫描完成时间。
	ScannedAt string `json:"scannedAt"`
	// Entries 是全部条目。默认不返回（体积可达数 MB），
	// 由 ScanEntries 按分类/月份分页拉取。
	Entries []EntryDTO `json:"entries,omitempty"`
}

// LibraryDTO 是账号库信息。
type LibraryDTO struct {
	WXID  string `json:"wxid"`
	Path  string `json:"path"`
	Total int64  `json:"total"`
}

// CategoryDTO 是分类汇总。
type CategoryDTO struct {
	Name     string `json:"name"`
	Bytes    int64  `json:"bytes"`
	BytesTxt string `json:"bytesText"`
	Count    int    `json:"count"`
}

// MonthDTO 是月份汇总。
type MonthDTO struct {
	Month    string `json:"month"`
	Bytes    int64  `json:"bytes"`
	BytesTxt string `json:"bytesText"`
	Count    int    `json:"count"`
}

// OrphanDTO 是孤儿缩略图统计。
type OrphanDTO struct {
	Count    int    `json:"count"`
	Bytes    int64  `json:"bytes"`
	BytesTxt string `json:"bytesText"`
}

// DuplicateDTO 是真重复统计。
type DuplicateDTO struct {
	Groups    int    `json:"groups"`
	Bytes     int64  `json:"bytes"`
	BytesText string `json:"bytesText"`
}

// YearDTO 是图片缓存的年份分布。
type YearDTO struct {
	Year     string `json:"year"`
	Count    int    `json:"count"`
	Bytes    int64  `json:"bytes"`
	BytesTxt string `json:"bytesText"`
}

// Scan 执行数据体检，返回完整结果。
//
// 这是只读操作，不改动任何文件。
func (s *Service) Scan(libPath string) (*ScanResult, error) {
	lib, err := s.resolveLibrary(libPath)
	if err != nil {
		return nil, err
	}
	s.lib = lib

	res, err := scan.New(lib.Path).Run()
	if err != nil {
		return nil, fmt.Errorf("扫描失败：%w", err)
	}

	out := &ScanResult{
		Library:           LibraryDTO{WXID: lib.WXID, Path: lib.Path},
		Total:             res.Space.Actual,
		Nominal:           res.Space.Nominal,
		HardlinkOvercount: res.Space.HardlinkOvercount,
		TotalText:         humanBytes(res.Space.Actual),
		EntryCount:        res.Space.EntryCount,
		DurationMs:        res.Duration.Milliseconds(),
		WeChatRunning:     guard.IsWeChatRunning(),
		ScannedAt:         time.Now().Format(time.RFC3339),
	}

	out.Categories = s.summarizeCategories(res.Entries)
	out.Months = s.summarizeMonths(res.Entries)

	orphanCount, orphanBytes := cachestat.OrphanTotal(res.Entries)
	out.Orphans = OrphanDTO{
		Count: orphanCount, Bytes: orphanBytes, BytesTxt: humanBytes(orphanBytes),
	}

	if groups, dupErr := (dup.Finder{}).Find(res.Entries); dupErr == nil {
		out.Duplicates = DuplicateDTO{
			Groups:    len(groups),
			Bytes:     dup.TotalReclaimable(groups),
			BytesText: humanBytes(dup.TotalReclaimable(groups)),
		}
	}

	for _, y := range cachestat.ByYear(res.Entries) {
		out.CacheYears = append(out.CacheYears, YearDTO{
			Year: y.Year, Count: y.Count, Bytes: y.Bytes, BytesTxt: humanBytes(y.Bytes),
		})
	}

	// 只返回汇总，不返回全量条目——14000 条会产生 7.6MB JSON，
	// 浏览器解析要几秒，是界面卡顿的主因。
	// 缓存扫描结果供后续分页查询复用，避免每次翻页都重扫。
	s.cached = res.Entries
	s.cachedLib = lib.Path
	return out, nil
}

// summarizeCategories 按分类汇总。
func (s *Service) summarizeCategories(entries []model.Entry) []CategoryDTO {
	bytesByCat := cachestat.ByCategory(entries)
	countByCat := make(map[model.Category]int, len(bytesByCat))
	for _, e := range entries {
		countByCat[e.Category]++
	}

	out := make([]CategoryDTO, 0, len(bytesByCat))
	for cat, b := range bytesByCat {
		out = append(out, CategoryDTO{
			Name: string(cat), Bytes: b, BytesTxt: humanBytes(b), Count: countByCat[cat],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bytes > out[j].Bytes })
	return out
}

// summarizeMonths 按月份汇总，月份降序。
func (s *Service) summarizeMonths(entries []model.Entry) []MonthDTO {
	type acc struct {
		bytes int64
		count int
	}
	m := make(map[string]*acc, 64)
	for _, e := range entries {
		key := e.Month
		if key == "" {
			key = "未知月份"
		}
		a, ok := m[key]
		if !ok {
			a = &acc{}
			m[key] = a
		}
		a.bytes += e.Size
		a.count++
	}

	out := make([]MonthDTO, 0, len(m))
	for k, a := range m {
		out = append(out, MonthDTO{
			Month: k, Bytes: a.bytes, BytesTxt: humanBytes(a.bytes), Count: a.count,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Month == "未知月份" {
			return false
		}
		if out[j].Month == "未知月份" {
			return true
		}
		return out[i].Month > out[j].Month
	})
	return out
}

// toDTOs 把领域条目转成前端结构。
func (s *Service) toDTOs(entries []model.Entry, libPath string) []EntryDTO {
	now := time.Now()
	out := make([]EntryDTO, len(entries))
	for i, e := range entries {
		rel, relErr := filepath.Rel(libPath, e.Path)
		if relErr != nil {
			rel = e.Path
		}
		det := preview.Detect(e)

		daysAgo := 0
		if e.ModTime > 0 {
			daysAgo = int(now.Sub(time.Unix(e.ModTime, 0)).Hours() / 24)
		}

		out[i] = EntryDTO{
			Path:          e.Path,
			RelPath:       rel,
			Name:          filepath.Base(e.Path),
			Size:          e.Size,
			SizeText:      humanBytes(e.Size),
			Month:         e.Month,
			Category:      string(e.Category),
			ModTime:       e.ModTime,
			DaysAgo:       daysAgo,
			IsThumbnail:   e.IsThumbnail,
			IsOrphan:      e.IsOrphanThumbnail,
			Previewable:   det.Kind == preview.KindImage || det.Kind == preview.KindVideoThumb,
			PreviewReason: det.Reason,
		}
	}
	return out
}

// CleanRequest 是清理请求。
type CleanRequest struct {
	// Paths 是要清理的条目路径。
	Paths []string `json:"paths"`
	// IncludeCache 表示允许清理图片缓存。
	IncludeCache bool `json:"includeCache"`
}

// CleanResult 是清理结果。
type CleanResult struct {
	// Moved 是实际移入废纸篓的条目数。
	Moved int `json:"moved"`
	// Bytes 是释放的字节数。
	Bytes int64 `json:"bytes"`
	// BytesText 是格式化的字节数。
	BytesText string `json:"bytesText"`
	// Skipped 是被跳过的条目数。
	Skipped int `json:"skipped"`
	// SkippedReasons 是跳过原因。
	SkippedReasons []string `json:"skippedReasons"`
}

// Clean 把指定路径移入废纸篓。
//
// 前端负责弹窗确认（含风险提示），这里只执行。
func (s *Service) Clean(req CleanRequest) (*CleanResult, error) {
	if s.lib.Path == "" {
		return nil, fmt.Errorf("请先完成数据体检")
	}

	res, err := scan.New(s.lib.Path).Run()
	if err != nil {
		return nil, fmt.Errorf("重新扫描失败：%w", err)
	}

	want := make(map[string]struct{}, len(req.Paths))
	for _, p := range req.Paths {
		want[p] = struct{}{}
	}

	var picked []model.Entry
	for _, e := range res.Entries {
		if _, ok := want[e.Path]; ok {
			picked = append(picked, e)
		}
	}
	if len(picked) == 0 {
		return nil, fmt.Errorf("所选文件已不存在，请重新体检")
	}

	msgRoot := filepath.Join(s.lib.Path, "msg")
	plan := clean.Build(clean.Options{
		Entries:      picked,
		MsgRoot:      msgRoot,
		IncludeCache: req.IncludeCache,
	})

	cleaner := trash.New(manifestDir(), s.lib.Path)
	m, applyErr := clean.Apply(plan, msgRoot, guard.IsWeChatRunning(), cleaner)

	out := &CleanResult{
		Moved:     len(m.Records),
		Bytes:     m.TotalBytes(),
		BytesText: humanBytes(m.TotalBytes()),
		Skipped:   len(plan.Skipped),
	}
	for _, sk := range plan.Skipped {
		out.SkippedReasons = append(out.SkippedReasons,
			fmt.Sprintf("%s：%s", filepath.Base(sk.Entry.Path), sk.Reason))
	}

	// 部分失败仍返回已成功的部分，同时把错误带回前端提示。
	if applyErr != nil && len(m.Records) == 0 {
		return nil, applyErr
	}
	return out, nil
}

// UndoResult 是撤销结果。
type UndoResult struct {
	Restored     int      `json:"restored"`
	Bytes        int64    `json:"bytes"`
	BytesText    string   `json:"bytesText"`
	Skipped      int      `json:"skipped"`
	SkippedNotes []string `json:"skippedNotes"`
}

// Undo 回滚最近一次清理。
func (s *Service) Undo() (*UndoResult, error) {
	dir := manifestDir()
	latest, err := trash.Latest(dir)
	if err != nil {
		return nil, fmt.Errorf("没有可撤销的清理记录")
	}

	m, err := trash.Load(latest)
	if err != nil {
		return nil, err
	}

	res, err := undo.Apply(m)
	if err != nil {
		return nil, err
	}

	out := &UndoResult{
		Restored:  len(res.Restored),
		Bytes:     res.RestoredBytes,
		BytesText: humanBytes(res.RestoredBytes),
		Skipped:   len(res.Skipped),
	}
	for _, sk := range res.Skipped {
		out.SkippedNotes = append(out.SkippedNotes,
			fmt.Sprintf("%s：%s", filepath.Base(sk.Record.OriginalPath), sk.Reason))
	}
	return out, nil
}

// RunningWarning 是微信运行时的风险提示。
type RunningWarning struct {
	// Running 表示微信是否在运行。
	Running bool `json:"running"`
	// HasChatFiles 表示所选含聊天文件或视频。
	HasChatFiles bool `json:"hasChatFiles"`
	// HasCache 表示所选含图片缓存。
	HasCache bool `json:"hasCache"`
	// Message 是给用户看的提示。
	Message string `json:"message"`
}

// CheckRunning 检查所选条目的风险，供前端弹窗展示。
func (s *Service) CheckRunning(paths []string) RunningWarning {
	if !guard.IsWeChatRunning() {
		return RunningWarning{Running: false}
	}

	res, err := scan.New(s.lib.Path).Run()
	if err != nil {
		return RunningWarning{Running: true}
	}

	want := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		want[p] = struct{}{}
	}

	var hasChat, hasCache bool
	for _, e := range res.Entries {
		if _, ok := want[e.Path]; !ok {
			continue
		}
		switch guard.RiskOf(e) {
		case guard.RiskLow:
			hasCache = true
		case guard.RiskMedium:
			hasChat = true
		}
	}

	msg := ""
	if hasChat {
		msg += "聊天文件与视频可能被微信的硬链接记录追踪，删除后微信的「存储空间管理」统计可能对不上（文件本身不受影响）。"
	}
	if hasCache {
		if msg != "" {
			msg += "\n"
		}
		msg += "图片缓存删除后，如果你滚到对应聊天记录，微信会重新下载写回。"
	}

	return RunningWarning{
		Running: true, HasChatFiles: hasChat, HasCache: hasCache, Message: msg,
	}
}

// OpenFile 用系统默认程序打开文件。
func (s *Service) OpenFile(path string) error {
	return opener.Open(model.Entry{Path: path})
}

// RevealFile 在 Finder 中定位文件。
func (s *Service) RevealFile(path string) error {
	return opener.Reveal(model.Entry{Path: path})
}

// PreviewImage 返回条目的预览图（base64 data URL）。
//
// 图片直接解码，视频取封面。不可预览时返回空串与原因。
func (s *Service) PreviewImage(path string) (string, error) {
	e := model.Entry{Path: path, IsThumbnail: previewIsThumb(path)}
	det := preview.Detect(e)
	if det.Kind == preview.KindIcon {
		return "", nil
	}

	if det.Kind == preview.KindVideoThumb {
		img, ok := s.thumbs.Load(e)
		if !ok {
			return "", nil
		}
		return encodeDataURL(img), nil
	}

	img, err := decodeFile(path)
	if err != nil {
		return "", err
	}
	return encodeDataURL(img), nil
}

// Libraries 返回全部账号库。
func (s *Service) Libraries() ([]LibraryDTO, error) {
	root, err := account.Root()
	if err != nil {
		return nil, err
	}
	libs, err := account.Discover(root)
	if err != nil {
		return nil, err
	}

	out := make([]LibraryDTO, 0, len(libs))
	for _, lib := range libs {
		size, sizeErr := account.SizeOf(lib)
		if sizeErr != nil {
			size = 0
		}
		out = append(out, LibraryDTO{WXID: lib.WXID, Path: lib.Path, Total: size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Total > out[j].Total })
	return out, nil
}

// resolveLibrary 解析要操作的账号库。
func (s *Service) resolveLibrary(libPath string) (account.Library, error) {
	if libPath != "" {
		info, err := os.Stat(libPath)
		if err != nil {
			return account.Library{}, fmt.Errorf("无法访问账号库：%w", err)
		}
		if !info.IsDir() {
			return account.Library{}, fmt.Errorf("%s 不是目录", libPath)
		}
		return account.Library{Path: libPath, WXID: filepath.Base(libPath)}, nil
	}

	root, err := account.Root()
	if err != nil {
		return account.Library{}, err
	}
	return account.Largest(root)
}

// manifestDir 返回清理清单目录。
func manifestDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".wechat-file-keeper"
	}
	return filepath.Join(home, "Library", "Application Support",
		"wechat-file-keeper", "manifests")
}

// EntriesQuery 是分页查询条目的参数。
type EntriesQuery struct {
	// Category 是分类筛选，空表示全部。
	Category string `json:"category"`
	// Month 是月份筛选，空表示全部。
	Month string `json:"month"`
	// MinSize 是最小字节数。
	MinSize int64 `json:"minSize"`
	// OrphansOnly 表示只看孤儿缩略图。
	OrphansOnly bool `json:"orphansOnly"`
	// Offset 是起始下标。
	Offset int `json:"offset"`
	// Limit 是最多返回条数。
	Limit int `json:"limit"`
	// SortBy 是排序字段：size / month / name。
	SortBy string `json:"sortBy"`
	// Descending 表示降序。
	Descending bool `json:"descending"`
}

// EntriesPage 是一页条目。
type EntriesPage struct {
	// Entries 是当前页的条目。
	Entries []EntryDTO `json:"entries"`
	// Total 是符合条件的总条数。
	Total int `json:"total"`
	// Offset 是当前起始下标。
	Offset int `json:"offset"`
	// HasMore 表示还有更多。
	HasMore bool `json:"hasMore"`
}

// ScanEntries 分页查询条目。
//
// 前端只在需要时拉一页（默认 60 条），不再一次拿 14000 条。
func (s *Service) ScanEntries(q EntriesQuery) (*EntriesPage, error) {
	if s.cached == nil {
		if _, err := s.Scan(""); err != nil {
			return nil, err
		}
	}

	// 筛选
	matched := make([]model.Entry, 0, len(s.cached))
	for _, e := range s.cached {
		if q.OrphansOnly && !e.IsOrphanThumbnail {
			continue
		}
		if q.Category != "" && string(e.Category) != q.Category {
			continue
		}
		if q.Month != "" && e.Month != q.Month {
			continue
		}
		if e.Size < q.MinSize {
			continue
		}
		matched = append(matched, e)
	}

	// 排序
	switch q.SortBy {
	case "month":
		sort.SliceStable(matched, func(i, j int) bool {
			if q.Descending {
				return matched[i].Month > matched[j].Month
			}
			return matched[i].Month < matched[j].Month
		})
	case "name":
		sort.SliceStable(matched, func(i, j int) bool {
			if q.Descending {
				return filepath.Base(matched[i].Path) > filepath.Base(matched[j].Path)
			}
			return filepath.Base(matched[i].Path) < filepath.Base(matched[j].Path)
		})
	default:
		sort.SliceStable(matched, func(i, j int) bool {
			if q.Descending {
				return matched[i].Size > matched[j].Size
			}
			return matched[i].Size < matched[j].Size
		})
	}

	total := len(matched)
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 60
	}
	end := offset + limit
	if end > total {
		end = total
	}

	return &EntriesPage{
		Entries: s.toDTOs(matched[offset:end], s.cachedLib),
		Total:   total,
		Offset:  offset,
		HasMore: end < total,
	}, nil
}

// MonthSummary 返回按月份聚合的统计，供月份桶列表使用。
//
// 与 ScanResult.Months 的区别：这里带上该月的分类明细，
// 前端不用为了画桶头而拉全量条目。
func (s *Service) MonthSummary() ([]MonthDTO, error) {
	if s.cached == nil {
		if _, err := s.Scan(""); err != nil {
			return nil, err
		}
	}
	return s.summarizeMonths(s.cached), nil
}
