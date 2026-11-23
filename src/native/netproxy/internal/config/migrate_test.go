package config

import (
	"strings"
	"testing"
)

func TestMigrateRawModule(t *testing.T) {
	legacy := `{"auto_start":true,"selection":{"group_id":"default","node_tag":"null"},"wifi":{"enabled":true,"mode":"whitelist","blacklist":["备用"],"whitelist":["家"],"proxy_on_non_wifi":false}}`
	migrated, err := MigrateRawModule([]byte(legacy), []byte(`{"enabled":true,"url":"https://example.test/config","interval":3600}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseModule(migrated)
	if err != nil || !got.AutoStart || !got.WiFi.Enabled || got.WiFi.Mode != "whitelist" || len(got.WiFi.Whitelist) != 1 || got.WiFi.Whitelist[0] != "家" || got.WiFi.ProxyOnNonWiFi {
		t.Fatalf("转换后设置不一致: %+v %v", got, err)
	}
	if strings.Contains(string(migrated), `"selection"`) {
		t.Fatal("旧节点选择未移除")
	}
	for _, test := range []struct{ name, module, raw string }{
		{"直通关闭", legacy, `{"enabled":false}`},
		{"直通设置损坏", legacy, `{"enabled":true`},
		{"无旧字段", `{"auto_start":true}`, `{"enabled":true}`},
		{"旧字段为 null", `{"selection":null}`, `{"enabled":true}`},
		{"旧选择含 null", `{"selection":{"node_tag":null}}`, `{"enabled":true}`},
		{"未知节点字段", `{"selection":{"unknown":true}}`, `{"enabled":true}`},
		{"未知模块字段", `{"selection":{},"unknown":true}`, `{"enabled":true}`},
		{"无效 Wi-Fi", `{"selection":{},"wifi":{"mode":"off"}}`, `{"enabled":true}`},
		{"重复键", `{"selection":{},"selection":{}}`, `{"enabled":true}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := MigrateRawModule([]byte(test.module), []byte(test.raw)); err == nil {
				t.Fatal("不兼容配置不应迁移")
			}
		})
	}
}
