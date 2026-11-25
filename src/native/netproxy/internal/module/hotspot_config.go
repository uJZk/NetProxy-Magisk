package module

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/hotspot"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

func hotspotSettingsPath(o Options) string {
	return filepath.Join(paths.New(o.ModuleDir).Config(), "hotspot", "site.json")
}

// ReadHotspotConfig returns the editable configuration without disclosing the private key.
func ReadHotspotConfig(o Options) (map[string]string, error) {
	return readHotspotConfig(o, false)
}

// ReadHotspotBackup is restricted to root because it includes the private key.
func ReadHotspotBackup(o Options) (map[string]string, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("仅 Root 可导出包含私钥的站点配置")
	}
	return readHotspotConfig(o, true)
}

func readHotspotConfig(o Options, includePrivate bool) (map[string]string, error) {
	s, err := hotspot.LoadSettings(hotspotSettingsPath(o))
	if err != nil {
		return nil, err
	}
	original, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if !includePrivate {
		s.PrivateKey = ""
	}
	data, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	status := `{"state":"unknown","message":"后台状态暂不可用"}`
	statusBytes, err := os.ReadFile(filepath.Join(paths.New(o.ModuleDir).DevRoot(), "hotspot-status.json"))
	if err == nil {
		var saved struct {
			State     string `json:"state"`
			WorkerPID int    `json:"worker_pid"`
		}
		if json.Unmarshal(statusBytes, &saved) == nil && saved.WorkerPID > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			workerState, stateErr := worker.ReadStatus(ctx, workerOptions(o))
			cancel()
			if stateErr == nil && workerState.State == "running" && workerState.PID == saved.WorkerPID {
				status = string(statusBytes)
			}
		}
	}
	return map[string]string{"content": string(data), "revision": configRevision(original), "status": status}, nil
}

// ApplyHotspotConfig validates before replacing the settings, and restores the
// previous settings and core if applying the projected DNS configuration fails.
func ApplyHotspotConfig(ctx context.Context, o Options, content []byte, validateOnly bool, expected string) (string, error) {
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return "", err
	}
	defer lock.release()
	path := hotspotSettingsPath(o)
	old, err := hotspot.LoadSettings(path)
	if err != nil {
		return "", err
	}
	oldData, err := json.Marshal(old)
	if err != nil {
		return "", err
	}
	if expected != "" && configRevision(oldData) != expected {
		return "", ErrConfigConflict
	}
	var next hotspot.Settings
	if err := json.Unmarshal(content, &next); err != nil {
		return "", fmt.Errorf("站点设置格式错误: %w", err)
	}
	if o.RawOnly && next.TargetIP == "" {
		return "", errors.New("缺少 MITM 目标 IPv4 地址")
	}
	if next.PrivateKey == "" && next.Certificate != "" && next.Certificate == old.Certificate {
		next.PrivateKey = old.PrivateKey
	}
	if err := next.Validate(); err != nil {
		return "", err
	}
	raw, err := readRawSettings(o)
	if err != nil {
		return "", err
	}
	if next.Enabled && !raw.Enabled {
		return "", errors.New("请先启用完整配置直通")
	}
	encoded, err := json.Marshal(next)
	if err != nil {
		return "", err
	}
	revision := configRevision(encoded)
	if !o.RawOnly && raw.Enabled && (next.TargetIP == "" || old.Enabled && old.TargetIP == "") {
		_, source, _ := rawPaths(o)
		_, cleanup, err := rawSiteCandidateForSettings(ctx, o, source, next)
		if cleanup != nil {
			cleanup()
		}
		if err != nil {
			return "", err
		}
	}
	if validateOnly {
		return revision, nil
	}
	if bytes.Equal(oldData, encoded) {
		return revision, nil
	}
	previous, previousErr := os.ReadFile(path)
	if previousErr != nil && !errors.Is(previousErr, os.ErrNotExist) {
		return "", previousErr
	}
	if err := hotspot.SaveSettings(path, next); err != nil {
		return "", err
	}
	rollback := func() error {
		defer func() { _ = worker.Wake(workerOptions(o)) }()
		if errors.Is(previousErr, os.ErrNotExist) {
			err := os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		return writeRawFile(path, previous)
	}
	if !o.RawOnly && raw.Enabled && (next.TargetIP == "" || old.Enabled && old.TargetIP == "") {
		state, err := ReadServiceState(o.StateFile)
		if err != nil {
			return "", errors.Join(err, rollback())
		}
		if service.FindProcess(o.SingBoxPath, int(state.PID)) > 0 {
			if err = StopService(ctx, o); err == nil {
				err = StartService(ctx, o)
			}
			if err != nil {
				restoreErr := rollback()
				if restoreErr == nil {
					restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serviceReadyTimeout+15*time.Second)
					defer cancel()
					restoreErr = StartService(restoreCtx, o)
				}
				return "", fmt.Errorf("应用站点配置失败，已尝试恢复: %w", errors.Join(err, restoreErr))
			}
		}
	}
	_ = worker.Wake(workerOptions(o))
	return revision, nil
}
