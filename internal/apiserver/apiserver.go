// Package apiserver 用一个自定义 HTTP handler 同时提供静态资源与 JSON API。
//
// 为什么不走 Wails 的绑定机制：Wails v3 的绑定依赖 /wails/runtime.js 这个
// ES module，在本机 WebKit（macOS 12）上加载失败，导致所有后端调用静默
// 不生效。而 AssetOptions.Handler 接受任意 http.Handler，于是这里直接
// 提供普通 HTTP JSON 接口——前端只用 fetch，没有额外依赖，行为可预期。
package apiserver

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/kennethfan/wechat-file-keeper/internal/bridge"
)

// Handler 把 /api/ 前缀的请求交给 JSON 接口，其余交给静态资源。
type Handler struct {
	// assets 是静态资源 handler。
	assets http.Handler
	// svc 是业务服务。
	svc *bridge.Service
	// trashMethod 是当前删除方式（rename / finder）。
	trashMethod string
}

// currentMethod 返回当前删除方式，默认 rename。
func (h *Handler) currentMethod() string {
	if h.trashMethod == "" {
		return "rename"
	}
	return h.trashMethod
}

// New 创建一个组合 handler。
func New(assets http.Handler, svc *bridge.Service) *Handler {
	return &Handler{assets: assets, svc: svc}
}

// ServeHTTP 路由请求。
func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if len(req.URL.Path) >= 5 && req.URL.Path[:5] == "/api/" {
		log.Printf("[api] %s %s", req.Method, req.URL.Path)
		h.serveAPI(rw, req)
		return
	}
	h.assets.ServeHTTP(rw, req)
}

// serveAPI 处理 JSON 接口。
//
// 统一用 POST，请求体是 JSON 对象，响应也是 JSON。出错时返回
// {"error": "..."}，前端据此弹提示。
func (h *Handler) serveAPI(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeError(rw, http.StatusMethodNotAllowed, "只支持 POST")
		return
	}

	action := req.URL.Path[len("/api/"):]

	switch action {
	case "scan":
		var body struct {
			LibPath string `json:"libPath"`
		}
		_ = decodeBody(req, &body)

		res, err := h.svc.Scan(body.LibPath)
		writeResult(rw, res, err)

	case "clean":
		var body bridge.CleanRequest
		if err := decodeBody(req, &body); err != nil {
			writeError(rw, http.StatusBadRequest, err.Error())
			return
		}
		if body.Method == "" {
			body.Method = h.currentMethod()
		}
		res, err := h.svc.Clean(body)
		writeResult(rw, res, err)

	case "undo":
		res, err := h.svc.Undo()
		writeResult(rw, res, err)

	case "checkRunning":
		var body struct {
			Paths []string `json:"paths"`
		}
		_ = decodeBody(req, &body)
		writeResult(rw, h.svc.CheckRunning(body.Paths), nil)

	case "preview":
		var body struct {
			Path string `json:"path"`
		}
		_ = decodeBody(req, &body)
		res, err := h.svc.PreviewImage(body.Path)
		writeResult(rw, res, err)

	case "open":
		var body struct {
			Path string `json:"path"`
		}
		_ = decodeBody(req, &body)
		writeResult(rw, true, h.svc.OpenFile(body.Path))

	case "reveal":
		var body struct {
			Path string `json:"path"`
		}
		_ = decodeBody(req, &body)
		writeResult(rw, true, h.svc.RevealFile(body.Path))

	case "entries":
		var q bridge.EntriesQuery
		if err := decodeBody(req, &q); err != nil {
			writeError(rw, http.StatusBadRequest, err.Error())
			return
		}
		res, err := h.svc.ScanEntries(q)
		writeResult(rw, res, err)

	case "months":
		res, err := h.svc.MonthSummary()
		writeResult(rw, res, err)

	case "trashMethod":
		var body struct {
			Method string `json:"method"`
		}
		_ = decodeBody(req, &body)
		if body.Method != "" {
			h.trashMethod = body.Method
		}
		writeResult(rw, map[string]string{"method": h.currentMethod()}, nil)

	case "libraries":
		res, err := h.svc.Libraries()
		writeResult(rw, res, err)

	default:
		writeError(rw, http.StatusNotFound, "未知接口: "+action)
	}
}

// decodeBody 解析 JSON 请求体；空体不算错误。
func decodeBody(req *http.Request, dst any) error {
	if req.Body == nil {
		return nil
	}
	dec := json.NewDecoder(req.Body)
	if err := dec.Decode(dst); err != nil {
		// 空请求体（EOF）视为没有参数。
		if err.Error() == "EOF" {
			return nil
		}
		return err
	}
	return nil
}

// writeResult 写出成功或失败结果。
func writeResult(rw http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(rw, http.StatusOK, v)
}

// writeError 写出错误响应。
func writeError(rw http.ResponseWriter, code int, msg string) {
	writeJSON(rw, code, map[string]string{"error": msg})
}

// writeJSON 写出 JSON 响应。
func writeJSON(rw http.ResponseWriter, code int, v any) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(code)
	_ = json.NewEncoder(rw).Encode(v)
}
