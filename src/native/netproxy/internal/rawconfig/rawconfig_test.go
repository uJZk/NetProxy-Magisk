package rawconfig_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/rawconfig"
)

const sampleConfig = `{"log":{"level":"info"},"inbounds":[{"type":"ebpf","tag":"ebpf-in"}],"outbounds":[{"type":"direct","tag":"direct"}]}`

func newOptions(t *testing.T, url string) rawconfig.Options {
	t.Helper()
	directory := t.TempDir()
	return rawconfig.Options{
		URL:        url,
		Timeout:    5 * time.Second,
		Interval:   24 * time.Hour,
		ConfigPath: filepath.Join(directory, "raw.json"),
		MetaPath:   filepath.Join(directory, "raw.meta.json"),
		Validate:   func(context.Context, string) error { return nil },
		Now:        time.Unix(1_700_000_000, 0),
	}
}

func TestUpdateInstallsConfigAndSchedulesNextRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("ETag", `"v1"`)
		_, _ = writer.Write([]byte(sampleConfig))
	}))
	defer server.Close()

	options := newOptions(t, server.URL)
	result, err := rawconfig.Update(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if result.NotModified {
		t.Fatal("first download must not report not-modified")
	}
	content, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != sampleConfig {
		t.Fatalf("unexpected config content: %s", content)
	}
	info, err := os.Stat(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("config must stay root readable only, got %o", mode)
	}
	meta, err := rawconfig.LoadMeta(options.MetaPath)
	if err != nil {
		t.Fatal(err)
	}
	if meta.ETag != `"v1"` || meta.LastError != "" {
		t.Fatalf("unexpected meta: %#v", meta)
	}
	if want := options.Now.Add(24 * time.Hour).Unix(); meta.NextUpdateEpoch != want {
		t.Fatalf("next update = %d, want %d", meta.NextUpdateEpoch, want)
	}
}

// 校验失败必须保留上一版配置，否则一次坏更新会让下次开机起不来。
func TestUpdateKeepsPreviousConfigWhenCheckFails(t *testing.T) {
	body := sampleConfig
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()

	options := newOptions(t, server.URL)
	if _, err := rawconfig.Update(context.Background(), options); err != nil {
		t.Fatal(err)
	}

	body = `{"outbounds":[{"type":"nonsense"}]}`
	broken := options
	broken.Force = true
	broken.Validate = func(context.Context, string) error { return errors.New("bad outbound") }
	if _, err := rawconfig.Update(context.Background(), broken); err == nil {
		t.Fatal("expected the update to fail")
	}

	content, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != sampleConfig {
		t.Fatalf("previous config was replaced by an unchecked candidate: %s", content)
	}
	entries, err := os.ReadDir(filepath.Dir(options.ConfigPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" && entry.Name()[0] == '.' {
			t.Fatalf("candidate file was left behind: %s", entry.Name())
		}
	}
	meta, err := rawconfig.LoadMeta(options.MetaPath)
	if err != nil {
		t.Fatal(err)
	}
	if meta.LastError == "" {
		t.Fatal("failure was not recorded in the meta file")
	}
	// 失败后必须比正常周期更早重试，否则一次抖动要等满一个周期。
	if want := options.Now.Add(15 * time.Minute).Unix(); meta.NextUpdateEpoch != want {
		t.Fatalf("retry scheduled at %d, want %d", meta.NextUpdateEpoch, want)
	}
}

func TestUpdateHonoursNotModified(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("If-None-Match") == `"v1"` {
			writer.WriteHeader(http.StatusNotModified)
			return
		}
		writer.Header().Set("ETag", `"v1"`)
		_, _ = writer.Write([]byte(sampleConfig))
	}))
	defer server.Close()

	options := newOptions(t, server.URL)
	if _, err := rawconfig.Update(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	result, err := rawconfig.Update(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.NotModified {
		t.Fatal("second download should have been short circuited by the ETag")
	}
	if requests != 2 {
		t.Fatalf("unexpected request count: %d", requests)
	}
}

// 本地还没有配置时不能发条件请求：304 会让我们既没有新内容也没有旧内容。
func TestUpdateSkipsConditionalRequestWithoutLocalConfig(t *testing.T) {
	conditional := false
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("If-None-Match") != "" || request.Header.Get("If-Modified-Since") != "" {
			conditional = true
		}
		writer.Header().Set("ETag", `"v1"`)
		_, _ = writer.Write([]byte(sampleConfig))
	}))
	defer server.Close()

	options := newOptions(t, server.URL)
	if err := rawconfig.SaveMeta(options.MetaPath, rawconfig.Meta{Schema: 1, ETag: `"stale"`}); err != nil {
		t.Fatal(err)
	}
	if _, err := rawconfig.Update(context.Background(), options); err != nil {
		t.Fatal(err)
	}
	if conditional {
		t.Fatal("conditional headers were sent while no local config existed")
	}
}

func TestUpdateRejectsNonConfigBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("<html>502 Bad Gateway</html>"))
	}))
	defer server.Close()

	options := newOptions(t, server.URL)
	if _, err := rawconfig.Update(context.Background(), options); err == nil {
		t.Fatal("expected an error for a non JSON body")
	}
	if _, err := os.Stat(options.ConfigPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("an HTML error page must not be installed as a configuration")
	}
}

func TestScheduling(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if rawconfig.Due(rawconfig.Meta{NextUpdateEpoch: now.Unix() + 60}, time.Hour, now) {
		t.Fatal("update is not due yet")
	}
	if !rawconfig.Due(rawconfig.Meta{NextUpdateEpoch: now.Unix() - 1}, time.Hour, now) {
		t.Fatal("update is overdue")
	}
	if !rawconfig.Due(rawconfig.Meta{}, time.Hour, now) {
		t.Fatal("a never updated config is due immediately")
	}
	if rawconfig.Due(rawconfig.Meta{}, 0, now) {
		t.Fatal("interval 0 disables automatic updates")
	}
	if rawconfig.Nearest(rawconfig.Meta{NextUpdateEpoch: 42}, 0, now) != 0 {
		t.Fatal("interval 0 must not schedule anything")
	}
}

// 没有入站的配置能通过 sing-box check 却什么都不代理，必须在这里拦下。
func TestUpdateRejectsConfigWithoutInbound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`))
	}))
	defer server.Close()

	options := newOptions(t, server.URL)
	_, err := rawconfig.Update(context.Background(), options)
	if err == nil {
		t.Fatal("a configuration without inbounds must be rejected")
	}
	if !strings.Contains(err.Error(), "inbound") {
		t.Fatalf("unhelpful error: %v", err)
	}
	if _, statErr := os.Stat(options.ConfigPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("the rejected configuration must not be installed")
	}
}

// 模块不再注入 eBPF 入站，配置里自带的 ebpf 入站必须原样保留。
func TestCheckDocumentAcceptsUserSuppliedEBPFInbound(t *testing.T) {
	config := []byte(`{
  // 用户自己的抓取入口
  "inbounds": [{"type": "ebpf", "tag": "ebpf-in"}],
  "outbounds": [{"type": "direct", "tag": "direct"}]
}`)
	if err := rawconfig.CheckDocument(config); err != nil {
		t.Fatal(err)
	}
}

// 换地址后必须重新下载：沿用上一个地址的 ETag 会让服务端回 304，
// 旧配置原地留下，用户以为已经切换。
func TestUpdateRefetchesAfterURLChange(t *testing.T) {
	const replacement = `{"inbounds":[{"type":"ebpf","tag":"ebpf-in"}],"outbounds":[{"type":"block","tag":"block"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// 无条件回 304，模拟只看 If-Modified-Since 的服务端。
		if request.Header.Get("If-None-Match") != "" || request.Header.Get("If-Modified-Since") != "" {
			writer.WriteHeader(http.StatusNotModified)
			return
		}
		writer.Header().Set("ETag", `"shared"`)
		if request.URL.Path == "/second.json" {
			_, _ = writer.Write([]byte(replacement))
			return
		}
		_, _ = writer.Write([]byte(sampleConfig))
	}))
	defer server.Close()

	options := newOptions(t, server.URL+"/first.json")
	if _, err := rawconfig.Update(context.Background(), options); err != nil {
		t.Fatal(err)
	}

	switched := options
	switched.URL = server.URL + "/second.json"
	result, err := rawconfig.Update(context.Background(), switched)
	if err != nil {
		t.Fatal(err)
	}
	if result.NotModified {
		t.Fatal("switching the URL must not be short circuited by the previous ETag")
	}
	content, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != replacement {
		t.Fatalf("configuration was not replaced after the URL changed: %s", content)
	}
}
