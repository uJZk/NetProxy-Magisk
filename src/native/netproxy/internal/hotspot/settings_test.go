package hotspot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettingsPersistenceAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "site.json")
	initial, err := LoadSettings(path)
	if err != nil || initial.Enabled || initial.Domain != DefaultDomain {
		t.Fatalf("%+v: %v", initial, err)
	}
	initial.Enabled = true
	if err := SaveSettings(path, initial); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode: %v, %v", info, err)
	}
	bad := initial
	bad.Domain = "bad..domain"
	if err := SaveSettings(path, bad); err == nil {
		t.Fatal("accepted invalid settings")
	}
	got, err := LoadSettings(path)
	if err != nil || got.Domain != initial.Domain || !got.Enabled {
		t.Fatalf("%+v: %v", got, err)
	}
}

func TestDisabledInvalidSettingsDoNotBlockRawProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.json")
	if err := os.WriteFile(path, []byte(`{"enabled":false,"domain":"bad..name"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSettings(path); err == nil {
		t.Fatal("editable invalid settings were accepted")
	}
	settings, err := LoadProjectionSettings(path)
	if err != nil || settings.Enabled {
		t.Fatalf("disabled settings blocked raw core: %+v %v", settings, err)
	}
}
