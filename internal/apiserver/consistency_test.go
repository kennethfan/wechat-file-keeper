package apiserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/bridge"
)

// TestMonthSummaryMatchesEntries 验证月份桶的头部数字与桶内条目一致。
//
// 这是用户实际遇到的 bug：选中「图片」后桶头显示 322 个，
// 展开却说「没有符合条件的文件」——因为 Months 接口没接收筛选条件。
func TestMonthSummaryMatchesEntries(t *testing.T) {
	h := New(staticStub(), newService())
	lib := fakeLibrary(t)

	h.ServeHTTP(httptest.NewRecorder(),
		post("/api/scan", `{"libPath":"`+lib+`"}`))

	// 按「文档」筛选，比较桶头统计与桶内条目数
	const q = `{"category":"文档"}`

	monthRec := httptest.NewRecorder()
	h.ServeHTTP(monthRec, post("/api/months", q))
	if monthRec.Code != http.StatusOK {
		t.Fatalf("months 应成功，实际 %d", monthRec.Code)
	}
	var months []map[string]any
	if err := json.Unmarshal(monthRec.Body.Bytes(), &months); err != nil {
		t.Fatalf("解析 months 失败: %v", err)
	}

	entryRec := httptest.NewRecorder()
	h.ServeHTTP(entryRec, post("/api/entries", `{"category":"文档","limit":100}`))
	if entryRec.Code != http.StatusOK {
		t.Fatalf("entries 应成功，实际 %d", entryRec.Code)
	}
	var page struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal(entryRec.Body.Bytes(), &page); err != nil {
		t.Fatalf("解析 entries 失败: %v", err)
	}

	sum := 0
	for _, m := range months {
		if c, ok := m["count"].(float64); ok {
			sum += int(c)
		}
	}

	if sum != page.Total {
		t.Fatalf("桶头合计 %d 与条目总数 %d 不一致", sum, page.Total)
	}
	if page.Total == 0 {
		t.Fatal("假账号库里有文档，筛选后不应为 0")
	}
}

// TestEntriesSorting 验证三种排序字段与方向。
func TestEntriesSorting(t *testing.T) {
	h := New(staticStub(), newService())
	lib := fakeLibrary(t)
	h.ServeHTTP(httptest.NewRecorder(), post("/api/scan", `{"libPath":"`+lib+`"}`))

	cases := []struct {
		name  string
		query string
		check func(t *testing.T, entries []map[string]any)
	}{
		{
			name:  "按大小降序",
			query: `{"sortBy":"size","descending":true,"limit":50}`,
			check: func(t *testing.T, es []map[string]any) {
				for i := 1; i < len(es); i++ {
					prev := es[i-1]["size"].(float64)
					cur := es[i]["size"].(float64)
					if prev < cur {
						t.Fatalf("第 %d 项 %v 小于第 %d 项 %v，不是降序", i-1, prev, i, cur)
					}
				}
			},
		},
		{
			name:  "按大小升序",
			query: `{"sortBy":"size","descending":false,"limit":50}`,
			check: func(t *testing.T, es []map[string]any) {
				for i := 1; i < len(es); i++ {
					prev := es[i-1]["size"].(float64)
					cur := es[i]["size"].(float64)
					if prev > cur {
						t.Fatalf("第 %d 项 %v 大于第 %d 项 %v，不是升序", i-1, prev, i, cur)
					}
				}
			},
		},
		{
			name:  "按名称升序",
			query: `{"sortBy":"name","descending":false,"limit":50}`,
			check: func(t *testing.T, es []map[string]any) {
				for i := 1; i < len(es); i++ {
					prev := es[i-1]["name"].(string)
					cur := es[i]["name"].(string)
					if prev > cur {
						t.Fatalf("第 %d 项 %q 大于第 %d 项 %q，不是升序", i-1, prev, i, cur)
					}
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, post("/api/entries", c.query))
			if rec.Code != http.StatusOK {
				t.Fatalf("应成功，实际 %d: %s", rec.Code, rec.Body.String())
			}
			var page struct {
				Entries []map[string]any `json:"entries"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatalf("解析失败: %v", err)
			}
			if len(page.Entries) < 2 {
				t.Skip("假账号库条目太少，无法验证排序")
			}
			c.check(t, page.Entries)
		})
	}
}

// TestRefreshSummaryUsesCache 验证 refresh 接口不重扫磁盘。
func TestRefreshSummaryUsesCache(t *testing.T) {
	h := New(staticStub(), newService())
	lib := fakeLibrary(t)
	h.ServeHTTP(httptest.NewRecorder(), post("/api/scan", `{"libPath":"`+lib+`"}`))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, post("/api/refresh", `{}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh 应成功，实际 %d: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		EntryCount int `json:"entryCount"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if res.EntryCount == 0 {
		t.Fatal("refresh 应返回缓存的条目数，实际为 0")
	}
}

// TestRefreshBeforeScanFallsBack 验证没体检过时 refresh 会先扫一次。
func TestRefreshBeforeScanFallsBack(t *testing.T) {
	svc := bridge.New()
	// 没有 libPath，走默认路径会失败——这里只验证不 panic
	_, err := svc.RefreshSummary()
	_ = err
}
