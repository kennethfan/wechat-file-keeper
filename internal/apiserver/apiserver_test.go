package apiserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kennethfan/wechat-file-keeper/internal/bridge"
)

// fakeLibrary 造一个迷你微信账号库，供测试使用。
//
// 不读本机真实数据：CI 上没有微信目录，依赖真实数据会让测试必然失败。
// 结构参照微信 4.x：msg/file、msg/video、msg/attach。
func fakeLibrary(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	lib := filepath.Join(root, "wxid_test")

	write := func(rel, content string) {
		p := filepath.Join(lib, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("写文件失败: %v", err)
		}
	}

	write("msg/file/2025-02/简历.pdf", "pdf-bytes")
	write("msg/file/2025-03/合同.pdf", "pdf-bytes-2")
	write("msg/video/2025-04/abc.mp4", "video-bytes")
	write("msg/video/2025-04/gone_thumb.jpg", "orphan-thumb")

	return lib
}

// newFakeHandler 构造一个基于假账号库的 handler。
func newFakeHandler(t *testing.T) *Handler {
	t.Helper()
	return New(staticStub(), bridge.New())
}

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

	lib := fakeLibrary(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scan",
		strings.NewReader(`{"libPath":"`+lib+`"}`))
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
	lib := fakeLibrary(t)

	// 先体检一次，让服务端有缓存供分页查询
	scanReq := httptest.NewRequest(http.MethodPost, "/api/scan",
		strings.NewReader(`{"libPath":"`+lib+`"}`))
	h.ServeHTTP(httptest.NewRecorder(), scanReq)

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

	lib := fakeLibrary(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scan",
		strings.NewReader(`{"libPath":"`+lib+`"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("体检应成功，实际 %d: %s", rec.Code, rec.Body.String())
	}
	// 7.6MB 是全量；这里应远小于它
	if rec.Body.Len() > 500*1024 {
		t.Fatalf("体检响应过大：%.2f MB，应不含全量条目",
			float64(rec.Body.Len())/1024/1024)
	}
}
