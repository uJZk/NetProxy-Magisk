package module

import (
	"slices"
	"testing"
)

// 直通模式绝不能再带上 -C confdir：sing-box 合并多份配置是数组追加，
// 托管配置会和用户配置叠加出重复的 inbounds、dns.servers 与出站标签。
func TestConfigArgsSelectRawConfiguration(t *testing.T) {
	raw := PrepareResult{
		Raw:         true,
		RawConfig:   "/m/config/singbox/raw.json",
		RawServices: "/m/config/singbox/confdir/08_services.json",
	}
	args := raw.ConfigArgs("/m/config/singbox")
	want := []string{
		"-c", "/m/config/singbox/raw.json",
		"-c", "/m/config/singbox/confdir/08_services.json",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("raw args = %v, want %v", args, want)
	}
	if slices.Contains(args, "-C") {
		t.Fatal("raw mode must not load the managed confdir")
	}
}

func TestConfigArgsKeepManagedLayout(t *testing.T) {
	managed := PrepareResult{Providers: "/r/providers.json", Outbounds: "/r/outbounds.json", EBPF: "/r/ebpf.json"}
	args := managed.ConfigArgs("/m/config/singbox")
	want := []string{
		"-C", "/m/config/singbox/confdir",
		"-c", "/r/providers.json", "-c", "/r/outbounds.json", "-c", "/r/ebpf.json",
	}
	if !slices.Equal(args, want) {
		t.Fatalf("managed args = %v, want %v", args, want)
	}
}
