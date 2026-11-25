package hotspot

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
)

const DefaultDomain = "www.aabbcc.com"

// Settings is the single persistent configuration for the optional site.
// The private key must never be included in an unauthenticated response or log.
type Settings struct {
	TargetIP     string        `json:"target_ip,omitempty"`
	Enabled      bool          `json:"enabled"`
	Domain       string        `json:"domain,omitempty"`
	Certificate  string        `json:"certificate"`
	PrivateKey   string        `json:"private_key"`
	Routes       []Route       `json:"routes"`
	DNSOverrides []DNSOverride `json:"dns_overrides,omitempty"`
	DNSBlock     []string      `json:"dns_block,omitempty"`
	DNSAllow     []string      `json:"dns_allow,omitempty"`
}

// DNSOverride maps one exact domain to an IPv4 address. An empty address uses
// the site's fixed address.
type DNSOverride struct {
	Domain  string `json:"domain"`
	Address string `json:"address"`
}

func DefaultSettings() Settings { return Settings{Domain: DefaultDomain, Routes: []Route{}} }

func (s Settings) Validate() error {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s.Domain), "."))
	if s.TargetIP != "" {
		ip, err := netip.ParseAddr(s.TargetIP)
		if err != nil || !ip.Is4() || ip.IsMulticast() || ip.IsUnspecified() || s.TargetIP == "255.255.255.255" {
			return errors.New("MITM 目标必须是有效的单播 IPv4 地址")
		}
		if s.Enabled && s.Certificate == "" {
			return errors.New("启用 MITM 需要证书链与私钥")
		}
	} else if !validDomain(domain) {
		return errors.New("站点域名无效")
	}
	if _, err := NewHandler(s.Routes, nil); err != nil {
		return err
	}
	if s.TargetIP == "" {
		if _, err := normalizedOverrides(s.Domain, s.DNSOverrides); err != nil {
			return err
		}
		if _, err := blockRule(s.DNSBlock, s.DNSAllow); err != nil {
			return err
		}
	}
	if (s.Certificate == "") != (s.PrivateKey == "") {
		return errors.New("证书和私钥必须同时提供")
	}
	if s.Certificate != "" {
		if len(s.Certificate) > 64<<10 || len(s.PrivateKey) > 32<<10 {
			return errors.New("证书或私钥过大")
		}
		if _, err := ParseCertificate(domain, []byte(s.Certificate), []byte(s.PrivateKey)); err != nil {
			return fmt.Errorf("证书无效: %w", err)
		}
	}
	return nil
}

func LoadSettings(path string) (Settings, error) {
	return loadSettings(path, false)
}

// LoadProjectionSettings leaves disabled settings out of the raw startup path;
// invalid disabled site details must not prevent the independent core running.
func LoadProjectionSettings(path string) (Settings, error) {
	return loadSettings(path, true)
}

func loadSettings(path string, ignoreDisabledDetails bool) (Settings, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, fmt.Errorf("读取热点站点设置: %w", err)
	}
	if ignoreDisabledDetails && !settings.Enabled {
		return settings, nil
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// SaveSettings atomically replaces the file. The caller serializes changes and
// coordinates application with the service lifecycle.
func SaveSettings(path string, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hotspot-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
