package apiserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
