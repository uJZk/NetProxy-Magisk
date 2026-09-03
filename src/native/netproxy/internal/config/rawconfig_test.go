package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
)

func writeModuleConf(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "module.conf")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigModeDefaultsToManaged(t *testing.T) {
	module, err := config.LoadModule(writeModuleConf(t, "AUTO_START=0\n"))
	if err != nil {
		t.Fatal(err)
	}
	if module.ConfigMode != "managed" {
		t.Fatalf("config mode = %q, want managed", module.ConfigMode)
	}
	if module.RawConfigInterval != 86400 {
		t.Fatalf("interval = %d, want 86400", module.RawConfigInterval)
	}
}

// URL 带查询串和片段时必须能原样存取，否则订阅地址会被截断。
func TestRawConfigURLSurvivesQuoting(t *testing.T) {
	url := "https://example.com/api/file/theWorld.android%20(SB)?token=a&b=c#frag"
	path := writeModuleConf(t, "CONFIG_MODE=raw\nRAW_CONFIG_URL="+config.Quote(url)+"\n")
	module, err := config.LoadModule(path)
	if err != nil {
		t.Fatal(err)
	}
	if module.RawConfigURL != url {
		t.Fatalf("url = %q, want %q", module.RawConfigURL, url)
	}
}

func TestRawModeRequiresURL(t *testing.T) {
	_, err := config.LoadModule(writeModuleConf(t, "CONFIG_MODE=raw\n"))
	if err == nil {
		t.Fatal("raw mode without a URL must be rejected")
	}
	if !strings.Contains(err.Error(), "RAW_CONFIG_URL") {
		t.Fatalf("unhelpful error: %v", err)
	}
}

func TestInvalidRawConfigValuesAreRejected(t *testing.T) {
	for name, body := range map[string]string{
		"未知模式":   "CONFIG_MODE=passthrough\n",
		"非 http": "RAW_CONFIG_URL=\"ftp://example.com/config.json\"\n",
		"周期不是整数": "RAW_CONFIG_INTERVAL=daily\n",
		"周期为负":   "RAW_CONFIG_INTERVAL=-1\n",
	} {
		if _, err := config.LoadModule(writeModuleConf(t, body)); err == nil {
			t.Fatalf("%s: expected the value to be rejected", name)
		}
	}
}
