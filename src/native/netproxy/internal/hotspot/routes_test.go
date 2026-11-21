package hotspot

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCustomRoutes(t *testing.T) {
	h, err := NewHandler([]Route{
		{Path: "/abc", JSON: json.RawMessage(`{ "ok": true }`)},
		{Path: "/abc/long", JSON: json.RawMessage(`{"choice":2}`)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/X/ABC/?q=1", `{"ok":true}`, 200},
		{"/abc/long/next", `{"choice":2}`, 200},
		{"/abcx", "404 page not found\n", 404},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", tc.path, nil))
		if w.Code != tc.status || w.Body.String() != tc.body {
			t.Fatalf("%s: %d %q", tc.path, w.Code, w.Body.String())
		}
		if tc.status == 200 && (len(w.Header().Get("X-Checksum")) != 32 || w.Header().Get("Cache-Control") != "no-store") {
			t.Fatal(w.Header())
		}
	}
}

func TestRouteValidation(t *testing.T) {
	for _, route := range []Route{
		{Path: "/ping", JSON: json.RawMessage(`{}`)},
		{Path: "/test", JSON: json.RawMessage(`{`)},
		{Path: "/test", JSON: json.RawMessage(`"` + strings.Repeat("x", 1024) + `"`)},
	} {
		if _, err := NewHandler([]Route{route}, nil); err == nil {
			t.Fatalf("accepted invalid route: %q", route.Path)
		}
	}
}
