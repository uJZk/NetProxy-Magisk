package module

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
