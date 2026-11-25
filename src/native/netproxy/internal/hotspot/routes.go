// Package hotspot provides the optional hotspot-facing HTTPS site.
package hotspot

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Route maps a path segment sequence to a fixed JSON response.
type Route struct {
	Path string          `json:"path"`
	JSON json.RawMessage `json:"json"`
}

type compiledRoute struct {
	path     string
	body     []byte
	checksum string
}

// Handler is immutable after construction and is safe for concurrent requests.
type Handler struct {
	routes  []compiledRoute
	status  func() any
	OnMatch func(string)
}

func normalizePath(path string) string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	return "/" + strings.Join(parts, "/")
}

// NewHandler validates all routes before making a handler available.
func NewHandler(routes []Route, status func() any) (*Handler, error) {
	if len(routes) > 8 {
		return nil, errors.New("最多允许 8 条 HTTPS 自定义接口")
	}
	h := &Handler{status: status}
	seen := make(map[string]bool)
	for _, route := range routes {
		path := normalizePath(route.Path)
		key := strings.ToLower(path)
		if !strings.HasPrefix(route.Path, "/") || len(path) < 2 || len(path) > 64 || strings.ContainsAny(path, " \t\r\n?#*%") {
			return nil, fmt.Errorf("无效的 HTTPS 接口路径 %q", route.Path)
		}
		switch key {
		case "/ping", "/status", "/headers", "/echo":
			return nil, fmt.Errorf("HTTPS 内置路径 %q 不可覆盖", path)
		}
		if seen[key] {
			return nil, fmt.Errorf("重复的 HTTPS 接口路径 %q", path)
		}
		seen[key] = true
		var compact bytes.Buffer
		if err := json.Compact(&compact, route.JSON); err != nil || compact.Len() > 1024 {
			return nil, fmt.Errorf("HTTPS 接口 %q 的 JSON 无效或超过 1024 字节", path)
		}
		body := compact.Bytes()
		sum := md5.Sum(body) // Compatibility with the original site's response header.
		h.routes = append(h.routes, compiledRoute{key, body, hex.EncodeToString(sum[:])})
	}
	return h, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.ToLower(normalizePath(r.URL.Path))
	switch path {
	case "/":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "HTTPS site: /ping /status /headers /echo\n")
	case "/ping":
		writeJSON(w, map[string]any{"protocol": r.Proto, "client": r.RemoteAddr})
	case "/status":
		if h.status == nil {
			writeJSON(w, map[string]any{})
			return
		}
		writeJSON(w, h.status())
	case "/headers":
		writeJSON(w, r.Header)
	case "/echo":
		body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil {
			http.Error(w, "cannot read request", http.StatusBadRequest)
			return
		}
		if len(body) > 1<<20 {
			http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
			return
		}
		contentType := r.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Echo-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body)
	default:
		var best *compiledRoute
		for i := range h.routes {
			route := &h.routes[i]
			if (best == nil || len(route.path) > len(best.path)) && containsSegments(path, route.path) {
				best = route
			}
		}
		if best == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if h.OnMatch != nil {
			h.OnMatch(best.path)
		}
		w.Header().Set("X-Checksum", best.checksum)
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(best.body)
	}
}

func containsSegments(path, pattern string) bool {
	for start := 0; start < len(path); start++ {
		if path[start] != '/' || !strings.HasPrefix(path[start:], pattern) {
			continue
		}
		end := start + len(pattern)
		if end == len(path) || path[end] == '/' {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
