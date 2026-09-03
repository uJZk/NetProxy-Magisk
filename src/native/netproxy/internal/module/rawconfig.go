package module

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	moduleconfig "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/config"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/rawconfig"
)

// RawConfigState 是 config raw show 返回的完整状态。
type RawConfigState struct {
	Mode      string         `json:"mode"`
	URL       string         `json:"url"`
	UserAgent string         `json:"user_agent,omitempty"`
	Interval  int64          `json:"interval"`
	Path      string         `json:"path"`
	Installed bool           `json:"installed"`
	Meta      rawconfig.Meta `json:"meta"`
}

// RawConfigEnabled 报告模块是否运行用户提供的完整配置。
func RawConfigEnabled(options Options) (bool, error) {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return false, err
	}
	return module.ConfigMode == "raw", nil
}

// ReadRawConfig 汇总直通配置的设置与下载状态。
func ReadRawConfig(options Options) (RawConfigState, error) {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return RawConfigState{}, err
	}
	configPath := paths.SingBoxRawConfig(options.SingBoxDir)
	meta, err := rawconfig.LoadMeta(paths.SingBoxRawConfigMeta(options.SingBoxDir))
	if err != nil {
		return RawConfigState{}, err
	}
	info, statErr := os.Stat(configPath)
	return RawConfigState{
		Mode:      module.ConfigMode,
		URL:       module.RawConfigURL,
		UserAgent: module.RawConfigUA,
		Interval:  module.RawConfigInterval,
		Path:      configPath,
		Installed: statErr == nil && !info.IsDir(),
		Meta:      meta,
	}, nil
}

// SetRawConfigSource 保存直通配置的地址与更新周期，不切换运行模式。
func SetRawConfigSource(options Options, url string, interval int64, userAgent string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errors.New("直通配置地址不能为空")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("直通配置地址必须是 http(s): %s", url)
	}
	if interval < 0 {
		return errors.New("更新周期不能为负")
	}
	updates := map[string]string{
		"RAW_CONFIG_URL":        moduleconfig.Quote(url),
		"RAW_CONFIG_INTERVAL":   strconv.FormatInt(interval, 10),
		"RAW_CONFIG_USER_AGENT": moduleconfig.Quote(userAgent),
	}
	return moduleconfig.UpdateModule(options.ModuleConfig, updates)
}

// EnableRawConfig 切换到直通模式。
// 必须先有一份通过检查的配置，否则切换后下一次启动才会失败。
func EnableRawConfig(options Options) error {
	state, err := ReadRawConfig(options)
	if err != nil {
		return err
	}
	if state.URL == "" {
		return errors.New("请先用 config raw set 设置直通配置地址")
	}
	if !state.Installed {
		return errors.New("尚未成功下载配置，请先执行 config raw update")
	}
	return moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{"CONFIG_MODE": "raw"})
}

// DisableRawConfig 切回模块自己生成配置的托管模式。
func DisableRawConfig(options Options) error {
	return moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{"CONFIG_MODE": "managed"})
}

// ClearRawConfig 切回托管模式并删除直通配置及其下载状态。
func ClearRawConfig(options Options) error {
	// 先切模式再删文件：反过来会在两次写入之间留下 raw 模式却没有配置的窗口，
	// 此时开机会直接启动失败。
	if err := moduleconfig.UpdateModule(options.ModuleConfig, map[string]string{
		"CONFIG_MODE":           "managed",
		"RAW_CONFIG_URL":        moduleconfig.Quote(""),
		"RAW_CONFIG_USER_AGENT": moduleconfig.Quote(""),
	}); err != nil {
		return err
	}
	for _, path := range []string{
		paths.SingBoxRawConfig(options.SingBoxDir),
		paths.SingBoxRawConfigMeta(options.SingBoxDir),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// UpdateRawConfig 下载并应用一份新的完整配置。
func UpdateRawConfig(ctx context.Context, options Options, force bool) (rawconfig.Result, error) {
	module, err := moduleconfig.LoadModule(options.ModuleConfig)
	if err != nil {
		return rawconfig.Result{}, err
	}
	if strings.TrimSpace(module.RawConfigURL) == "" {
		return rawconfig.Result{}, errors.New("请先用 config raw set 设置直通配置地址")
	}
	result, err := rawconfig.Update(ctx, rawconfig.Options{
		URL:        module.RawConfigURL,
		UserAgent:  module.RawConfigUA,
		Timeout:    60 * time.Second,
		Interval:   time.Duration(module.RawConfigInterval) * time.Second,
		ConfigPath: paths.SingBoxRawConfig(options.SingBoxDir),
		MetaPath:   paths.SingBoxRawConfigMeta(options.SingBoxDir),
		Validate: rawconfig.SingBoxValidator(options.SingBoxPath, options.SingBoxDir,
			paths.SingBoxServicesDoc(options.SingBoxDir)),
		Force: force,
	})
	if err != nil {
		logService(options, "ERROR", "rawconfig.update", "failed", "直通配置更新失败: %v", err)
		return rawconfig.Result{}, err
	}
	if result.NotModified {
		logService(options, "INFO", "rawconfig.update", "not-modified", "直通配置无变化")
		return result, nil
	}
	logService(options, "INFO", "rawconfig.update", "success", "直通配置已更新 (%d 字节)", result.Bytes)
	return result, nil
}
