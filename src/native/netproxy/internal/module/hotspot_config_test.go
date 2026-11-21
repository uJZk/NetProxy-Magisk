package module

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/hotspot"
)

func TestHotspotConfigurationValidationAndRevision(t *testing.T) {
	root := t.TempDir()
	o := newTestOptions(root)
	if err := os.MkdirAll(o.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	o.SingBoxPath = filepath.Join(root, "sing-box")
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	_, source, _ := rawPaths(o)
	if err := os.WriteFile(source, []byte(`{"outbounds":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	initial, err := ReadHotspotConfig(o)
	if err != nil {
		t.Fatal(err)
	}
	next := hotspot.DefaultSettings()
	next.Enabled = true
	cert, key, err := hotspot.NewSelfSignedCertificate(next.Domain, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	next.Certificate, next.PrivateKey = string(cert), string(key)
	candidate, err := json.Marshal(next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyHotspotConfig(t.Context(), o, candidate, false, initial["revision"]); err == nil {
		t.Fatal("enabled without raw mode")
	}
	if err := writeRawSettings(o, RawSettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	revision, err := ApplyHotspotConfig(t.Context(), o, candidate, false, initial["revision"])
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadHotspotConfig(o)
	if err != nil {
		t.Fatal(err)
	}
	if read["revision"] != revision || strings.Contains(read["content"], "PRIVATE KEY") {
		t.Fatalf("key exposed or revision incorrect: %+v", read)
	}
	if _, err := ApplyHotspotConfig(t.Context(), o, candidate, false, initial["revision"]); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	var redacted hotspot.Settings
	if err := json.Unmarshal([]byte(read["content"]), &redacted); err != nil {
		t.Fatal(err)
	}
	redacted.Domain = next.Domain
	redacted.DNSBlock = []string{"*.ads.example"}
	update, err := json.Marshal(redacted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyHotspotConfig(t.Context(), o, update, false, revision); err != nil {
		t.Fatal(err)
	}
	actual, err := hotspot.LoadSettings(hotspotSettingsPath(o))
	if err != nil || actual.PrivateKey != string(key) {
		t.Fatalf("key not retained: %v", err)
	}
	if err := os.WriteFile(o.SingBoxPath, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	redacted.DNSBlock = []string{"*.other.example"}
	invalid, _ := json.Marshal(redacted)
	if _, err := ApplyHotspotConfig(t.Context(), o, invalid, false, ""); err == nil {
		t.Fatal("accepted core validation failure")
	}
	actual, err = hotspot.LoadSettings(hotspotSettingsPath(o))
	if err != nil || actual.DNSBlock[0] != "*.ads.example" {
		t.Fatalf("failed validation modified settings: %+v, %v", actual, err)
	}
}
