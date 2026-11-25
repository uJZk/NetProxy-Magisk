package main

import (
	"errors"
	"os"
	"path/filepath"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/hotspot"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
)

// Upgrade only operates on the installer's staged snapshot.
func upgradeSnapshot(o moduleapp.Options) error {
	if _, err := moduleconfig.LoadModule(o.ModuleConfig); err != nil {
		if err = migrateRawModule(o.ModuleConfig, filepath.Join(o.SingBoxDir, "raw-settings.json")); err != nil {
			return err
		}
	}
	dir := filepath.Join(filepath.Dir(o.ModuleConfig), "hotspot")
	path := filepath.Join(dir, "site.json")
	settings, err := hotspot.LoadSettings(path)
	if err != nil {
		return err
	}
	if settings.TargetIP != "" {
		return nil
	}
	if settings.Certificate == "" {
		stored, err := hotspot.LoadSettings(filepath.Join(dir, "selfsigned.json"))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if stored.Domain == settings.Domain {
			settings.Certificate, settings.PrivateKey = stored.Certificate, stored.PrivateKey
		}
	}
	settings.TargetIP = hotspot.DefaultSiteAddress
	settings.Domain = ""
	settings.DNSOverrides = nil
	settings.DNSAllow = nil
	settings.DNSBlock = nil
	if settings.Certificate == "" {
		settings.Enabled = false
	}
	return hotspot.SaveSettings(path, settings)
}
