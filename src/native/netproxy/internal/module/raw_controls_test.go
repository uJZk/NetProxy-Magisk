package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRawControlsUseOnlyUserConfig(t *testing.T) {
	o := StandaloneOptions(t.TempDir())
	o.StateFile = filepath.Join(o.ModuleDir, "state", "service.json")
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	o.SingBoxPath = filepath.Join(o.ModuleDir, "validator")
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\n! grep -q reject \"$3\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(o.ModuleDir, "original")
	raw := `{"services":[{/* JSONC */"type":"api","listen":"::","listen_port":1234,"dashboard":true,"tls":{"enabled":true}}],"outbounds":[]}`
	if err := os.WriteFile(source, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ImportRawFile(t.Context(), o, source); err != nil {
		t.Fatal(err)
	}
	url, err := RawPanelURL(o)
	if err != nil || url != "https://[::1]:1234/dashboard/" {
		t.Fatalf("panel %q %v", url, err)
	}
	if err := ClearRawCache(t.Context(), o); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(`{"reject":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ImportRawFile(t.Context(), o, source); err == nil {
		t.Fatal("accepted rejected config")
	}
	_, installed, _ := rawPaths(o)
	b, err := os.ReadFile(installed)
	if err != nil || string(b) != raw {
		t.Fatalf("lost original: %s %v", b, err)
	}
	s, err := ReadRawConfig(o)
	if err != nil || !s.Enabled || !s.Installed {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestPanelOmittedWithoutHostedUI(t *testing.T) {
	o := StandaloneOptions(t.TempDir())
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	_, p, _ := rawPaths(o)
	for _, doc := range []string{`{}`, `{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","external_ui_download_url":"https://example.org/ui.zip"}}}`} {
		if err := os.WriteFile(p, []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
		url, err := RawPanelURL(o)
		if err != nil || url != "" {
			t.Fatalf("panel %q %v", url, err)
		}
	}
}

func TestSwitchCoreValidatesBeforeReplacingExecutable(t *testing.T) {
	o := StandaloneOptions(t.TempDir())
	o.StateFile = filepath.Join(o.ModuleDir, "state", "service.json")
	slot, _ := coreSlot(o, "stable")
	for _, dir := range []string{filepath.Dir(o.SingBoxPath), filepath.Dir(slot), o.SingBoxDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for path, script := range map[string]string{o.SingBoxPath: "#!/bin/sh\nexit 0\n", slot: "#!/bin/sh\nexit 1\n"} {
		if err := os.WriteFile(path, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	_, p, _ := rawPaths(o)
	if err := os.WriteFile(p, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(o.SingBoxPath)
	if err != nil {
		t.Fatal(err)
	}
	if err = SwitchCore(t.Context(), o, "stable"); err == nil {
		t.Fatal("selected incompatible core")
	}
	after, err := os.Stat(o.SingBoxPath)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("active core replaced after failed validation")
	}
	if err = os.WriteFile(slot, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = SwitchCore(t.Context(), o, "stable"); err != nil {
		t.Fatal(err)
	}
	selected, err := os.Stat(slot)
	if err != nil {
		t.Fatal(err)
	}
	after, err = os.Stat(o.SingBoxPath)
	if err != nil || !os.SameFile(selected, after) {
		t.Fatal("verified core not selected")
	}
}

func TestServiceDashboardForms(t *testing.T) {
	o := StandaloneOptions(t.TempDir())
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	_, source, _ := rawPaths(o)
	for _, tc := range []struct{ form, url string }{
		{"true", "http://127.0.0.1:9090/dashboard/"},
		{`"public"`, "http://127.0.0.1:9090/dashboard/"},
		{`{"enabled":true,"path":"public"}`, "http://127.0.0.1:9090/dashboard/"},
		{"false", ""}, {`{"enabled":false}`, ""},
	} {
		doc := `{"services":[{"type":"api","listen_port":9090,"dashboard":` + tc.form + `}]}`
		if err := os.WriteFile(source, []byte(doc), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := RawPanelURL(o)
		if err != nil || got != tc.url {
			t.Errorf("dashboard %s: got %q, %v", tc.form, got, err)
		}
	}
}

func TestIntervalOnlyPreservesSource(t *testing.T) {
	o := StandaloneOptions(t.TempDir())
	o.StateFile = filepath.Join(o.ModuleDir, "state", "service.json")
	if err := SetRawConfigSource(t.Context(), o, "https://example.org/config", 60, "test-agent"); err != nil {
		t.Fatal(err)
	}
	for _, seconds := range []int64{600, 0} {
		if err := SetRawInterval(t.Context(), o, seconds); err != nil {
			t.Fatal(err)
		}
		s, err := readRawSettings(o)
		if err != nil || s.URL != "https://example.org/config" || s.UserAgent != "test-agent" || s.Interval != seconds {
			t.Fatalf("unexpected persisted schedule: %+v, %v", s, err)
		}
	}
}
