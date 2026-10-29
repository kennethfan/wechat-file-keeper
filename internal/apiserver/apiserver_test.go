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
	for _, want := range []string{`"library"`, `"total"`, `"entries"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("响应缺少字段 %s", want)
		}
	}
}
