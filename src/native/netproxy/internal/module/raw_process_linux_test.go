//go:build linux

package module

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRawProcessUsesOnlyDownloadedConfigAndRestarts(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler unavailable")
	}
	root := t.TempDir()
	o := newTestOptions(root)
	o.SingBoxPath = filepath.Join(root, "sing-box")
	source := `#include <stdio.h>
#include <string.h>
#include <unistd.h>
int main(int n, char **v) {
 if (n==4 && !strcmp(v[1],"check") && !strcmp(v[2],"-c")) return access(v[3],R_OK)!=0;
 if (n!=5 || strcmp(v[1],"run") || strcmp(v[2],"--disable-color") || strcmp(v[3],"-c")) return 2;
 FILE *in=fopen(v[4],"r"), *out=fopen("seen.json","w"); if (!in || !out) return 3;
 char buf[4096]; size_t size=fread(buf,1,sizeof(buf)-1,in); buf[size]=0; fwrite(buf,1,size,out); fclose(in); fclose(out);
 if (strstr(buf,"bad")) return 4; for (;;) pause();
}`
	path := filepath.Join(root, "fake.c")
	if err = os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(cc, "-o", o.SingBoxPath, path).CombinedOutput(); err != nil {
		t.Fatalf("compile fake core: %s: %v", out, err)
	}
	if err = os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	body := `{"outbounds":[{"type":"direct","tag":"one"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
	defer server.Close()
	ctx := context.Background()
	if err = SetRawConfigSource(ctx, o, server.URL, 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = UpdateRawConfig(ctx, o, true); err != nil {
		t.Fatal(err)
	}
	if err = SetRawEnabled(ctx, o, true); err != nil {
		t.Fatal(err)
	}
	old := rawJoinRootCgroup
	rawJoinRootCgroup = func(int) error { return nil }
	defer func() { rawJoinRootCgroup = old }()
	if err = StartService(ctx, o); err != nil {
		t.Fatal(err)
	}
	defer StopService(ctx, o)
	first, err := ReadServiceState(o.StateFile)
	if err != nil || first.State != "running" || first.ReadyAt != 0 {
		t.Fatalf("wrong state: %+v %v", first, err)
	}
	seen, err := os.ReadFile(filepath.Join(o.SingBoxDir, "seen.json"))
	if err != nil || string(seen) != body {
		t.Fatalf("core received %q: %v", seen, err)
	}
	body = `{"outbounds":[{"type":"direct","tag":"two"}]}`
	if _, err = UpdateRawConfig(ctx, o, true); err != nil {
		t.Fatal(err)
	}
	second, err := ReadServiceState(o.StateFile)
	if err != nil || second.PID == first.PID || second.State != "running" {
		t.Fatalf("not restarted: %+v %v", second, err)
	}
	seen, err = os.ReadFile(filepath.Join(o.SingBoxDir, "seen.json"))
	if err != nil || !strings.Contains(string(seen), `"two"`) {
		t.Fatalf("new config not used: %s %v", seen, err)
	}
	body = `{"outbounds":[{"type":"direct","tag":"bad"}]}`
	if _, err = UpdateRawConfig(ctx, o, true); err == nil {
		t.Fatal("expected a startup failure")
	}
	_, rawPath, _ := rawPaths(o)
	config, err := os.ReadFile(rawPath)
	if err != nil || !strings.Contains(string(config), `"two"`) {
		t.Fatalf("old config not restored: %s %v", config, err)
	}
	state, err := ReadServiceState(o.StateFile)
	if err != nil || state.State != "running" {
		t.Fatalf("old service not restored: %+v %v", state, err)
	}
}
