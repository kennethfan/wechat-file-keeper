package apiserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/bridge"
)

// staticStub 是静态资源的桩，用来确认 /api/ 之外仍然走静态 handler。
func staticStub() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("static:" + r.URL.Path))
	})
}

func TestServeHTTP_RoutesAPIToService(t *testing.T) {
	h := New(staticStub(), bridge.New())

	req := httptest.NewRequest(http.MethodPost, "/api/checkRunning", strings.NewReader(`{"paths":[]}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("应返回 200，实际 %d，body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "running") {
		t.Fatalf("响应应含 running 字段，实际 %s", rec.Body.String())
	}
}

func TestServeHTTP_RoutesStaticToAssets(t *testing.T) {
	h := New(staticStub(), bridge.New())

	req := httptest.NewRequest(http.MethodGet, "/src/app.js", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != "static:/src/app.js" {
		t.Fatalf("静态请求应交给 assets，实际 %q", got)
	}
}

func TestServeHTTP_RejectsNonPost(t *testing.T) {
	h := New(staticStub(), bridge.New())

	req := httptest.NewRequest(http.MethodGet, "/api/scan", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("非 POST 应返回 405，实际 %d", rec.Code)
	}
}

func TestServeHTTP_UnknownAction(t *testing.T) {
	h := New(staticStub(), bridge.New())

	req := httptest.NewRequest(http.MethodPost, "/api/nope", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("未知接口应返回 404，实际 %d", rec.Code)
	}
}

func TestServeHTTP_ScanReturnsJSON(t *testing.T) {
	h := New(staticStub(), bridge.New())

	req := httptest.NewRequest(http.MethodPost, "/api/scan", strings.NewReader(`{"libPath":""}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("体检应成功，实际 %d: %s", rec.Code, body)
	}
	// 至少应含这些字段，说明真的扫到了数据。
	// 注意：不含 entries——全量条目由 /api/entries 分页提供。
	for _, want := range []string{`"library"`, `"total"`, `"months"`, `"orphans"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("响应缺少字段 %s", want)
		}
	}
}

func TestServeHTTP_EntriesPagination(t *testing.T) {
	h := New(staticStub(), bridge.New())

	req := httptest.NewRequest(http.MethodPost, "/api/entries",
		strings.NewReader(`{"limit":5,"offset":0,"sortBy":"size","descending":true}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("分页查询应成功，实际 %d: %s", rec.Code, body)
	}
	if !strings.Contains(body, `"total"`) || !strings.Contains(body, `"hasMore"`) {
		t.Fatalf("响应应含分页字段，实际 %s", body[:200])
	}
	// 响应体必须远小于全量（全量约 7.6MB）
	if len(body) > 200*1024 {
		t.Fatalf("单页响应过大：%d 字节，应远小于全量", len(body))
	}
}

func TestScan_DoesNotReturnAllEntries(t *testing.T) {
	// 体检结果不应携带全量条目——那是卡顿主因。
	h := New(staticStub(), bridge.New())

	req := httptest.NewRequest(http.MethodPost, "/api/scan", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("体检应成功，实际 %d", rec.Code)
	}
	// 7.6MB 是全量；这里应远小于它
	if rec.Body.Len() > 500*1024 {
		t.Fatalf("体检响应过大：%.2f MB，应不含全量条目",
			float64(rec.Body.Len())/1024/1024)
	}
}
