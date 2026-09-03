package convert_test

import (
	"context"
	"strings"
	"testing"

	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/convert"
)

const singBoxClientConfig = `{
  "log": {"level": "info"},
  "dns": {"servers": [{"tag": "local", "type": "local"}], "final": "local"},
  "inbounds": [{"type": "tun", "tag": "tun-in", "address": ["172.19.0.1/30"]}],
  "outbounds": [
    {"type": "urltest", "tag": "auto", "outbounds": ["node-a", "node-b"], "interval": "10m"},
    {"type": "selector", "tag": "select", "outbounds": ["auto", "direct"]},
    {"type": "vless", "tag": "node-a", "server": "a.example.com", "server_port": 443,
     "uuid": "3c0f47e3-a464-470c-a931-36b8a8d62fd6", "flow": "xtls-rprx-vision",
     "connect_timeout": "1s", "tcp_fast_open": true,
     "domain_resolver": {"server": "local", "strategy": "ipv4_only"},
     "bind_interface": "wlan0", "protect_path": "/data/data/other.app/protect_path",
     "routing_mark": 4660, "netns": "sandbox",
     "tls": {"enabled": true, "server_name": "a.example.com"}},
    {"type": "shadowsocks", "tag": "node-b", "server": "b.example.com", "server_port": 8388,
     "method": "aes-128-gcm", "password": "pw", "domain_resolver": "local"},
    {"type": "direct", "tag": "direct"},
    {"type": "block", "tag": "block"}
  ],
  "route": {"final": "select", "auto_detect_interface": true},
  "experimental": {"clash_api": {"external_controller": "127.0.0.1:9090"}}
}`

func TestSingBoxClientConfigKeepsOnlyNodes(t *testing.T) {
	result, err := convert.Content(context.Background(), singBoxClientConfig, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Outbounds) != 2 {
		t.Fatalf("expected two nodes, got %d", len(result.Document.Outbounds))
	}
	for _, outbound := range result.Document.Outbounds {
		switch outbound.Type {
		case C.TypeVLESS, C.TypeShadowsocks:
		default:
			t.Fatalf("group or built-in outbound leaked into the provider: %s/%s", outbound.Type, outbound.Tag)
		}
	}
}

func TestSingBoxImportStripsSourceEnvironment(t *testing.T) {
	result, err := convert.Content(context.Background(), singBoxClientConfig, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, outbound := range result.Document.Outbounds {
		dialer := outbound.Options.(option.DialerOptionsWrapper).TakeDialerOptions()
		if dialer.DomainResolver != nil {
			t.Fatalf("%s kept a domain resolver referring to the source configuration", outbound.Tag)
		}
		if dialer.BindInterface != "" || dialer.ProtectPath != "" || dialer.NetNs != "" || dialer.RoutingMark != 0 {
			t.Fatalf("%s kept source device bindings: %#v", outbound.Tag, dialer.AbstractDialerOptions)
		}
	}
	// 可移植的拨号调优必须保留，否则导入会丢掉用户在来源配置里的设置。
	for _, outbound := range result.Document.Outbounds {
		if outbound.Tag != "node-a" {
			continue
		}
		dialer := outbound.Options.(option.DialerOptionsWrapper).TakeDialerOptions()
		if !dialer.TCPFastOpen || dialer.ConnectTimeout == 0 {
			t.Fatalf("portable dialer options were dropped: %#v", dialer.AbstractDialerOptions)
		}
	}
}

func TestSingBoxImportSalvagesSupportedNodes(t *testing.T) {
	content := `{
  "outbounds": [
    {"type": "openvpn", "tag": "vpn", "server": "v.example.com", "server_port": 1194},
    {"type": "shadowsocks", "tag": "ok", "server": "b.example.com", "server_port": 8388,
     "method": "aes-128-gcm", "password": "pw"}
  ]
}`
	result, err := convert.Content(context.Background(), content, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Outbounds) != 1 || result.Document.Outbounds[0].Tag != "ok" {
		t.Fatalf("supported node was not salvaged: %#v", result.Document.Outbounds)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Source != "vpn" {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	if !strings.Contains(result.Diagnostics[0].Message, "openvpn") {
		t.Fatalf("diagnostic does not name the protocol: %q", result.Diagnostics[0].Message)
	}
}

// sing-box 配置允许注释，逐节点回退不能把这项容错弄丢。
func TestSingBoxImportAcceptsComments(t *testing.T) {
	content := `{
  // 未支持的协议
  "outbounds": [
    {"type": "openvpn", "tag": "vpn", "server": "v.example.com", "server_port": 1194},
    {"type": "socks", "tag": "ok", "server": "b.example.com", "server_port": 1080} // 节点
  ]
}`
	result, err := convert.Content(context.Background(), content, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Outbounds) != 1 || result.Document.Outbounds[0].Tag != "ok" {
		t.Fatalf("commented configuration was not imported: %#v", result.Document.Outbounds)
	}
}

func TestSingBoxImportReportsGroupOnlyConfig(t *testing.T) {
	content := `{"outbounds": [{"type": "direct", "tag": "direct"}]}`
	result, err := convert.Content(context.Background(), content, false)
	if err == nil {
		t.Fatal("expected an error for a configuration without nodes")
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "input.no_nodes" {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

// SIP008 文档没有 outbounds/endpoints，不能被 sing-box 分支拦截。
func TestSIP008DocumentStillParses(t *testing.T) {
	content := `{"version": 1, "servers": [{"id": "1", "remarks": "sip008", "server": "a.example.com",
  "server_port": 8388, "password": "pw", "method": "aes-128-gcm"}]}`
	result, err := convert.Content(context.Background(), content, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Outbounds) != 1 || result.Document.Outbounds[0].Tag != "sip008" {
		t.Fatalf("unexpected SIP008 result: %#v", result.Document.Outbounds)
	}
}
