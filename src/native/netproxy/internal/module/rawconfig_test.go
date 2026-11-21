package module

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/hotspot"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestRawSiteCandidatePreservesSourceAndChecksProjection(t *testing.T) {
	root := t.TempDir()
	o := newTestOptions(root)
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	o.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\ngrep -q 192.2.0.1 \"$3\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	settings := hotspot.DefaultSettings()
	settings.Enabled = true
	if err := hotspot.SaveSettings(filepath.Join(paths.New(root).Config(), "hotspot", "site.json"), settings); err != nil {
		t.Fatal(err)
	}
	_, source, _ := rawPaths(o)
	original := []byte(`{"dns":{"rules":[{"domain":"existing.test","server":"upstream"}]}}`)
	if err := os.WriteFile(source, original, 0600); err != nil {
		t.Fatal(err)
	}
	projected, cleanup, err := rawSiteCandidate(t.Context(), o, source)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(projected)
	if err != nil || !strings.Contains(string(content), "192.2.0.1") {
		t.Fatalf("projection %s: %v", content, err)
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != string(original) {
		t.Fatalf("source changed: %s, %v", got, err)
	}
	cleanup()
	if _, err := os.Stat(projected); !os.IsNotExist(err) {
		t.Fatalf("candidate not removed: %v", err)
	}
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rawSiteCandidate(t.Context(), o, source); err == nil {
		t.Fatal("accepted rejected projection")
	}
	got, err = os.ReadFile(source)
	if err != nil || string(got) != string(original) {
		t.Fatalf("source changed after failure: %s, %v", got, err)
	}
}

func TestRawSiteCandidateWithTargetCore(t *testing.T) {
	core := targetCoreBinary(t)
	root := t.TempDir()
	o := newTestOptions(root)
	o.SingBoxPath = core
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	s := hotspot.DefaultSettings()
	s.Enabled = true
	s.DNSBlock = []string{"*.ads.example"}
	s.DNSAllow = []string{"safe.ads.example"}
	if err := hotspot.SaveSettings(filepath.Join(paths.New(root).Config(), "hotspot", "site.json"), s); err != nil {
		t.Fatal(err)
	}
	_, source, _ := rawPaths(o)
	if err := os.WriteFile(source, []byte(`{"dns":{"servers":[{"type":"local","tag":"system"}],"final":"system"},"outbounds":[{"type":"direct","tag":"direct"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := rawSiteCandidate(t.Context(), o, source)
	if err != nil {
		t.Fatalf("target core rejected DNS projection: %v", err)
	}
	cleanup()
}

func TestRawUpdateAndFailurePreservesConfig(t *testing.T) {
	root := t.TempDir()
	o := newTestOptions(root)
	o.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	body := `{"outbounds":[{"type":"direct","tag":"first"}]}`
	status := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); _, _ = w.Write([]byte(body)) }))
	defer srv.Close()
	ctx := context.Background()
	if err := SetRawConfigSource(ctx, o, srv.URL, 3600, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateRawConfig(ctx, o, true); err != nil {
		t.Fatal(err)
	}
	_, p, _ := rawPaths(o)
	got, err := os.ReadFile(p)
	if err != nil || string(got) != body {
		t.Fatalf("download: %s %v", got, err)
	}
	status = http.StatusInternalServerError
	body = "fail"
	if _, err := UpdateRawConfig(ctx, o, true); err == nil {
		t.Fatal("expected download failure")
	}
	got, err = os.ReadFile(p)
	if err != nil || string(got) != `{"outbounds":[{"type":"direct","tag":"first"}]}` {
		t.Fatalf("old config lost: %s %v", got, err)
	}
	state, err := ReadRawConfig(o)
	if err != nil || !state.Installed || state.Meta.NextUpdateEpoch <= time.Now().Unix() {
		t.Fatalf("bad state %#v %v", state, err)
	}
}
func TestRawSettingsToggleWhenStopped(t *testing.T) {
	root := t.TempDir()
	o := newTestOptions(root)
	o.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\n[ -f \"$3\" ]\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	_, p, _ := rawPaths(o)
	if err := os.WriteFile(p, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := SetRawEnabled(ctx, o, true); err != nil {
		t.Fatal(err)
	}
	s, err := ReadRawConfig(o)
	if err != nil || !s.Enabled {
		t.Fatalf("not enabled: %#v %v", s, err)
	}
	if err := ClearRawConfig(ctx, o); err != nil {
		t.Fatal(err)
	}
	s, err = ReadRawConfig(o)
	if err != nil || s.Enabled || s.Installed {
		t.Fatalf("not cleared: %#v %v", s, err)
	}
}

func TestRawScheduleUpdatesOnceThenWaits(t *testing.T) {
	root := t.TempDir()
	o := newTestOptions(root)
	o.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; _, _ = w.Write([]byte(`{"outbounds":[]}`)) }))
	defer server.Close()
	ctx := context.Background()
	if err := SetRawConfigSource(ctx, o, server.URL, 3600, ""); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	next, err := RawSchedule(ctx, o, now)
	if err != nil || calls != 1 || next <= now.Unix() {
		t.Fatalf("first schedule: next=%d calls=%d err=%v", next, calls, err)
	}
	next2, err := RawSchedule(ctx, o, now.Add(time.Minute))
	if err != nil || calls != 1 || next2 != next {
		t.Fatalf("duplicate update: next=%d calls=%d err=%v", next2, calls, err)
	}
}
