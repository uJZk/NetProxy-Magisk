package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/hotspot"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
)

func TestHotspotHostStartsAndCleansOwnedAddress(t *testing.T) {
	root := t.TempDir()
	t.Setenv("NETPROXY_DEV_ROOT", filepath.Join(root, "dev"))
	options := moduleapp.NewOptions(root)
	if err := os.MkdirAll(options.SingBoxDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(options.SingBoxDir, "raw-settings.json"), []byte(`{"enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(paths.New(root).Config(), "hotspot", "site.json")
	settings := hotspot.DefaultSettings()
	settings.Enabled = true
	if err := hotspot.SaveSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}
	point := hotspot.AccessPoint{Name: "testap", Index: 42}
	started, removed := make(chan struct{}, 1), make(chan bool, 2)
	if err := writeHotspotLease(options, point); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 5)
	deps := hotspotHostDeps{
		watch:      func(ctx context.Context, _ func()) error { <-ctx.Done(); return nil },
		detect:     func() (hotspot.AccessPoint, error) { return point, nil },
		hasAddress: func(hotspot.AccessPoint) bool { return true },
		addAddress: func(p hotspot.AccessPoint) (bool, error) {
			if p != point {
				t.Errorf("unexpected AP: %+v", p)
			}
			return true, nil
		},
		removeAddress: func(p hotspot.AccessPoint, owned bool) error { removed <- owned; return nil },
		serve: func(ctx context.Context, address string, _ tls.Certificate, _ http.Handler, ready func()) error {
			if address != "192.2.0.1:443" {
				t.Errorf("address %s", address)
			}
			ready()
			started <- struct{}{}
			<-ctx.Done()
			return nil
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	wake := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runHotspotHostWith(ctx, options, wake, func(err error) { errs <- err }, deps)
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("site did not start")
	}
	select {
	case owned := <-removed:
		if !owned {
			t.Fatal("did not recover recorded address")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("previous address not recovered")
	}
	settings.Enabled = false
	if err := hotspot.SaveSettings(settingsPath, settings); err != nil {
		t.Fatal(err)
	}
	wake <- struct{}{}
	select {
	case owned := <-removed:
		if !owned {
			t.Fatal("did not clean owned address")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("site did not stop")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("host did not stop")
	}
	select {
	case err := <-errs:
		t.Fatal(err)
	default:
	}
}
