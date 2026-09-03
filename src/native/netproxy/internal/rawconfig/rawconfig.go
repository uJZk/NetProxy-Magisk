// Package rawconfig 管理直通模式运行的完整 sing-box 配置及其自动更新。
//
// 与订阅不同，这里保存的是一整份 sing-box 配置，不做节点提取，也不进入 Catalog。
// 更新遵循「下载 -> 候选文件 -> sing-box 检查 -> 原子替换」：任何一步失败都保留
// 上一版可用配置，绝不留下半应用状态。
package rawconfig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"encoding/json/jsontext"
	json "encoding/json/v2"

	SJSON "github.com/sagernet/sing/common/json"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/fetch"
)

// retryDelay 是下载或校验失败后的重试间隔。
// 直接沿用更新周期会让一次失败拖到下一个周期（默认 24 小时）才重试。
const retryDelay = 15 * time.Minute

// Meta 保存直通配置的下载状态。
// 用户设置（URL、周期、User-Agent）的事实源是 module.conf，这里只记录下载簿记，
// 避免同一项设置出现两个可写副本。
type Meta struct {
	Schema int `json:"schema"`
	// URL 记录这批缓存校验值取自哪个地址，不是设置的事实源（那仍是 module.conf）。
	// 换了地址还沿用旧的 ETag，服务端可能回 304，于是配置没换但用户以为换了。
	URL             string `json:"url"`
	ETag            string `json:"etag"`
	LastModified    string `json:"last_modified"`
	UpdatedAt       string `json:"updated_at"`
	NextUpdateAt    string `json:"next_update_at"`
	NextUpdateEpoch int64  `json:"next_update_epoch"`
	LastAttemptAt   string `json:"last_attempt_at"`
	LastStatusCode  int    `json:"last_status_code"`
	LastError       string `json:"last_error"`
	Bytes           int64  `json:"bytes"`
}

// Options 描述一次直通配置更新。
type Options struct {
	URL           string
	UserAgent     string
	Headers       map[string]string
	AllowInsecure bool
	Timeout       time.Duration
	Interval      time.Duration
	ConfigPath    string
	MetaPath      string
	// Validate 必须以 sing-box 校验候选配置。为空时 Update 直接报错，
	// 否则未经检查的配置会被原子替换进去，下次开机才暴露问题。
	Validate func(context.Context, string) error
	Now      time.Time
	// Force 忽略 ETag 与 Last-Modified，强制重新下载。
	Force bool
}

// Result 描述一次更新的结果。
type Result struct {
	NotModified  bool   `json:"not_modified"`
	StatusCode   int    `json:"status_code"`
	Bytes        int64  `json:"bytes"`
	UpdatedAt    string `json:"updated_at"`
	NextUpdateAt string `json:"next_update_at"`
}

// SingBoxValidator 返回用 sing-box 校验候选配置的校验器。
// 必须带上 Service API 文档：直通模式按同样的组合运行，只检查候选文件会漏掉
// 用户配置与 Service API 监听端口冲突这类只在合并后才出现的错误。
func SingBoxValidator(singBoxPath, singBoxDir, servicesPath string) func(context.Context, string) error {
	return func(ctx context.Context, candidate string) error {
		command := exec.CommandContext(ctx, singBoxPath, "check", "-c", candidate, "-c", servicesPath)
		command.Dir = singBoxDir
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}
}

// LoadMeta 读取下载状态，文件不存在时返回零值。
func LoadMeta(path string) (Meta, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Meta{Schema: 1}, nil
	}
	if err != nil {
		return Meta{}, err
	}
	var meta Meta
	if err := json.Unmarshal(content, &meta); err != nil {
		return Meta{}, fmt.Errorf("解析直通配置状态失败: %w", err)
	}
	if meta.Schema == 0 {
		meta.Schema = 1
	}
	return meta, nil
}

// SaveMeta 原子写入下载状态。
func SaveMeta(path string, meta Meta) error {
	if meta.Schema == 0 {
		meta.Schema = 1
	}
	content, err := json.Marshal(meta, json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		return err
	}
	return writeAtomic(path, append(content, '\n'), 0o600)
}

// Due 判断直通配置是否到达自动更新时间。
func Due(meta Meta, interval time.Duration, now time.Time) bool {
	if interval <= 0 {
		return false
	}
	if meta.NextUpdateEpoch <= 0 {
		return true
	}
	return meta.NextUpdateEpoch <= now.Unix()
}

// Nearest 返回下一次自动更新的时间戳，0 表示没有安排。
func Nearest(meta Meta, interval time.Duration, now time.Time) int64 {
	if interval <= 0 {
		return 0
	}
	if meta.NextUpdateEpoch <= 0 {
		return now.Unix()
	}
	return meta.NextUpdateEpoch
}

// Update 下载并原子应用一份新的完整配置。
func Update(ctx context.Context, options Options) (Result, error) {
	if strings.TrimSpace(options.URL) == "" {
		return Result{}, errors.New("未配置直通配置地址")
	}
	if strings.TrimSpace(options.ConfigPath) == "" || strings.TrimSpace(options.MetaPath) == "" {
		return Result{}, errors.New("直通配置路径不完整")
	}
	if options.Validate == nil {
		return Result{}, errors.New("缺少直通配置校验器")
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}

	meta, err := LoadMeta(options.MetaPath)
	if err != nil {
		return Result{}, err
	}
	meta.LastAttemptAt = formatEpoch(now.Unix())

	request := fetch.Request{
		URL:           options.URL,
		UserAgent:     options.UserAgent,
		Headers:       options.Headers,
		AllowInsecure: options.AllowInsecure,
		Timeout:       options.Timeout,
	}
	// 只有本地已有可用配置、且这批校验值确实取自同一个地址时才发条件请求：
	// 没有本地配置时 304 会让我们既没有新内容也没有旧内容；换了地址时 304 会
	// 让旧配置原地留下，看起来更新成功了。
	if !options.Force && fileExists(options.ConfigPath) && meta.URL == options.URL {
		request.ETag = meta.ETag
		request.LastModified = meta.LastModified
	}

	response, fetchErr := fetch.Subscription(ctx, request)
	if fetchErr != nil {
		return Result{}, options.fail(meta, now, fetchErr)
	}
	meta.LastStatusCode = response.Metadata.StatusCode

	if response.Metadata.NotModified {
		meta.LastError = ""
		options.schedule(&meta, now, options.Interval)
		if err := SaveMeta(options.MetaPath, meta); err != nil {
			return Result{}, err
		}
		return Result{
			NotModified: true, StatusCode: meta.LastStatusCode, Bytes: meta.Bytes,
			UpdatedAt: meta.UpdatedAt, NextUpdateAt: meta.NextUpdateAt,
		}, nil
	}

	if err := CheckDocument(response.Body); err != nil {
		return Result{}, options.fail(meta, now, err)
	}

	candidate, err := writeCandidate(options.ConfigPath, response.Body)
	if err != nil {
		return Result{}, options.fail(meta, now, err)
	}
	defer os.Remove(candidate)

	if err := options.Validate(ctx, candidate); err != nil {
		return Result{}, options.fail(meta, now, fmt.Errorf("sing-box 检查未通过: %w", err))
	}
	if err := os.Rename(candidate, options.ConfigPath); err != nil {
		return Result{}, options.fail(meta, now, err)
	}

	meta.URL = options.URL
	meta.ETag = response.Metadata.ETag
	meta.LastModified = response.Metadata.LastModified
	meta.Bytes = int64(len(response.Body))
	meta.UpdatedAt = formatEpoch(now.Unix())
	meta.LastError = ""
	options.schedule(&meta, now, options.Interval)
	if err := SaveMeta(options.MetaPath, meta); err != nil {
		return Result{}, err
	}
	return Result{
		StatusCode: meta.LastStatusCode, Bytes: meta.Bytes,
		UpdatedAt: meta.UpdatedAt, NextUpdateAt: meta.NextUpdateAt,
	}, nil
}

// fail 记录失败原因并安排较短的重试，磁盘上的配置保持不变。
func (options Options) fail(meta Meta, now time.Time, cause error) error {
	meta.LastError = cause.Error()
	delay := retryDelay
	if options.Interval > 0 && options.Interval < delay {
		delay = options.Interval
	}
	options.schedule(&meta, now, delay)
	if err := SaveMeta(options.MetaPath, meta); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (options Options) schedule(meta *Meta, now time.Time, interval time.Duration) {
	if interval <= 0 {
		meta.NextUpdateEpoch = 0
		meta.NextUpdateAt = ""
		return
	}
	next := now.Add(interval).Unix()
	meta.NextUpdateEpoch = next
	meta.NextUpdateAt = formatEpoch(next)
}

// CheckDocument 只做廉价的结构预检，协议层校验交给 sing-box。
// 机场故障时常返回 HTML 错误页并带 200，先在这里拦掉能给出可读得多的原因。
//
// 入站非空是硬性要求：直通模式模块不再生成 eBPF 入站，一份没有入站的配置能
// 通过 sing-box check、能正常 ready，但一个连接都不会被代理，日志里也没有错误。
func CheckDocument(body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return errors.New("下载内容为空")
	}
	if !strings.HasPrefix(trimmed, "{") {
		return errors.New("下载内容不是 sing-box 配置对象")
	}
	var shape struct {
		Inbounds []SJSON.RawMessage `json:"inbounds"`
	}
	// 用 sing 的解码器，保留 sing-box 配置允许的注释。
	if err := SJSON.Unmarshal(body, &shape); err != nil {
		return fmt.Errorf("配置不是合法 JSON: %w", err)
	}
	if len(shape.Inbounds) == 0 {
		return errors.New("配置里没有任何 inbound，代理不会生效；请在配置中声明入站（模块的透明代理入站类型为 ebpf）")
	}
	return nil
}

func writeCandidate(configPath string, body []byte) (string, error) {
	directory := filepath.Dir(configPath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, ".raw-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := finishCandidate(file, body); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func finishCandidate(file *os.File, body []byte) error {
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		return err
	}
	return file.Sync()
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".meta-*.tmp")
	if err != nil {
		return err
	}
	temporary := file.Name()
	if err := finishCandidate(file, content); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Chmod(temporary, mode); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func formatEpoch(epoch int64) string {
	if epoch <= 0 {
		return ""
	}
	return time.Unix(epoch, 0).UTC().Format(time.RFC3339)
}
