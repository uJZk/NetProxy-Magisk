package module

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/rawconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

type RawSettings struct {
	Enabled   bool   `json:"enabled"`
	URL       string `json:"url"`
	UserAgent string `json:"user_agent"`
	Interval  int64  `json:"interval"`
}
type RawState struct {
	RawSettings
	Installed bool           `json:"installed"`
	Meta      rawconfig.Meta `json:"meta"`
}

var rawJoinRootCgroup = ensureSingBoxRootCgroup

func rawPaths(o Options) (string, string, string) {
	return filepath.Join(o.SingBoxDir, "raw-settings.json"), filepath.Join(o.SingBoxDir, "raw.json"), filepath.Join(o.SingBoxDir, "raw.meta.json")
}
func readRawSettings(o Options) (RawSettings, error) {
	path, _, _ := rawPaths(o)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return RawSettings{}, nil
	}
	if err != nil {
		return RawSettings{}, err
	}
	var s RawSettings
	if err = json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("解析直通设置: %w", err)
	}
	if s.Interval < 0 || s.Interval > int64((365*24*time.Hour)/time.Second) {
		return s, errors.New("直通更新周期无效")
	}
	return s, nil
}
func RawEnabled(o Options) (bool, error) {
	s, err := readRawSettings(o)
	return s.Enabled, err
}
func writeRawSettings(o Options, s RawSettings) error {
	path, _, _ := rawPaths(o)
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writeRawFile(path, append(b, '\n'))
}
func writeRawFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".raw-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
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
func ReadRawConfig(o Options) (RawState, error) {
	s, err := readRawSettings(o)
	if err != nil {
		return RawState{}, err
	}
	_, p, m := rawPaths(o)
	meta, err := rawconfig.LoadMeta(m)
	if err != nil {
		return RawState{}, err
	}
	info, err := os.Stat(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RawState{}, err
	}
	return RawState{s, err == nil && info.Mode().IsRegular(), meta}, nil
}
func SetRawConfigSource(ctx context.Context, o Options, address string, interval int64, ua string) error {
	u, err := url.Parse(strings.TrimSpace(address))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("直通配置地址必须是有效的 HTTP(S) URL")
	}
	if interval < 0 || interval > int64((365*24*time.Hour)/time.Second) {
		return errors.New("更新周期无效")
	}
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return err
	}
	defer lock.release()
	s, err := readRawSettings(o)
	if err != nil {
		return err
	}
	changed := s.URL != u.String() || s.Interval != interval
	s.URL = u.String()
	s.Interval = interval
	s.UserAgent = ua
	if err = writeRawSettings(o, s); err != nil {
		return err
	}
	if changed {
		_, _, metaPath := rawPaths(o)
		meta, err := rawconfig.LoadMeta(metaPath)
		if err != nil {
			return err
		}
		meta.NextUpdateEpoch, meta.NextUpdateAt = 0, ""
		if err := rawconfig.SaveMeta(metaPath, meta); err != nil {
			return err
		}
	}
	_ = worker.Wake(workerOptions(o))
	return nil
}
func SetRawEnabled(ctx context.Context, o Options, enabled bool) error {
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return err
	}
	defer lock.release()
	return setRawEnabledLocked(ctx, o, enabled)
}

func setRawEnabledLocked(ctx context.Context, o Options, enabled bool) error {
	s, err := readRawSettings(o)
	if err != nil {
		return err
	}
	if s.Enabled == enabled {
		return nil
	}
	if enabled {
		_, p, _ := rawPaths(o)
		if err = rawconfig.SingBoxValidator(o.SingBoxPath, o.SingBoxDir)(ctx, p); err != nil {
			return fmt.Errorf("直通配置检查失败: %w", err)
		}
	}
	state, err := ReadServiceState(o.StateFile)
	if err != nil {
		return err
	}
	running := service.FindProcess(o.SingBoxPath, int(state.PID)) > 0
	if running {
		if err = StopService(ctx, o); err != nil {
			return err
		}
	}
	old := s
	s.Enabled = enabled
	if err = writeRawSettings(o, s); err != nil {
		if running {
			_ = StartService(ctx, o)
		}
		return err
	}
	if running {
		if err = StartService(ctx, o); err != nil {
			restoreErr := writeRawSettings(o, old)
			if restoreErr == nil {
				restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serviceReadyTimeout+15*time.Second)
				restoreErr = StartService(restoreCtx, o)
				cancel()
			}
			return errors.Join(err, restoreErr)
		}
	}
	_ = worker.Wake(workerOptions(o))
	return nil
}
func UpdateRawConfig(ctx context.Context, o Options, force bool) (rawconfig.Result, error) {
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return rawconfig.Result{}, err
	}
	defer lock.release()
	return updateRawLocked(ctx, o, force)
}
func updateRawLocked(ctx context.Context, o Options, force bool) (rawconfig.Result, error) {
	s, err := readRawSettings(o)
	if err != nil {
		return rawconfig.Result{}, err
	}
	_, p, m := rawPaths(o)
	old, oldErr := os.ReadFile(p)
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return rawconfig.Result{}, oldErr
	}
	state, err := ReadServiceState(o.StateFile)
	if err != nil {
		return rawconfig.Result{}, err
	}
	result, err := rawconfig.Update(ctx, rawconfig.Options{URL: s.URL, UserAgent: s.UserAgent, Timeout: 60 * time.Second, Interval: time.Duration(s.Interval) * time.Second, ConfigPath: p, MetaPath: m, Validate: rawconfig.SingBoxValidator(o.SingBoxPath, o.SingBoxDir), Force: force})
	if err != nil || result.NotModified {
		return result, err
	}
	if s.Enabled && service.FindProcess(o.SingBoxPath, int(state.PID)) > 0 {
		if err = StopService(ctx, o); err == nil {
			err = StartService(ctx, o)
		}
		if err != nil {
			var restoreErr error
			if oldErr == nil {
				restoreErr = writeRawFile(p, old)
			} else {
				restoreErr = os.Remove(p)
			}
			metaErr := os.Remove(m)
			if errors.Is(metaErr, os.ErrNotExist) {
				metaErr = nil
			}
			if oldErr == nil && restoreErr == nil {
				restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), serviceReadyTimeout+15*time.Second)
				restoreErr = StartService(restoreCtx, o)
				cancel()
			}
			return result, fmt.Errorf("应用直通配置失败，已尝试恢复旧配置: %w", errors.Join(err, restoreErr, metaErr))
		}
	}
	return result, nil
}
func ClearRawConfig(ctx context.Context, o Options) error {
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return err
	}
	defer lock.release()
	if err := setRawEnabledLocked(ctx, o, false); err != nil {
		return err
	}
	a, b, c := rawPaths(o)
	for _, p := range []string{a, b, c} {
		if err = os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	_ = worker.Wake(workerOptions(o))
	return nil
}

// RawSchedule is called by the existing worker and returns the next update time.
func RawSchedule(ctx context.Context, o Options, now time.Time) (int64, error) {
	s, err := readRawSettings(o)
	if err != nil {
		return 0, err
	}
	if s.URL == "" || s.Interval <= 0 {
		return 0, nil
	}
	_, _, p := rawPaths(o)
	m, err := rawconfig.LoadMeta(p)
	if err != nil {
		return 0, err
	}
	if rawconfig.Due(m, time.Duration(s.Interval)*time.Second, now) {
		if _, err = UpdateRawConfig(ctx, o, false); err != nil {
			m, e := rawconfig.LoadMeta(p)
			if e == nil && m.NextUpdateEpoch > now.Unix() {
				return m.NextUpdateEpoch, err
			}
			return now.Add(15 * time.Minute).Unix(), err
		}
		m, err = rawconfig.LoadMeta(p)
		if err != nil {
			return 0, err
		}
	}
	return rawconfig.Nearest(m, time.Duration(s.Interval)*time.Second, now), nil
}

// startRawService starts only the user's complete configuration. Process survival
// is reported as running; it does not claim that an arbitrary inbound is ready.
func startRawService(ctx context.Context, o Options) error {
	if err := ensureLifecycleStateDir(o); err != nil {
		return err
	}
	state, err := ReadServiceState(o.StateFile)
	if err != nil {
		return err
	}
	if pid := service.FindProcess(o.SingBoxPath, int(state.PID)); pid > 0 {
		if state.State == "running" && state.PID == int64(pid) {
			return nil
		}
		return errors.New("已有 sing-box 进程运行，请先停止服务")
	}
	_, p, _ := rawPaths(o)
	if err = rawconfig.SingBoxValidator(o.SingBoxPath, o.SingBoxDir)(ctx, p); err != nil {
		return fmt.Errorf("直通配置检查失败: %w", err)
	}
	logPath, err := LogFile(o, "core")
	if err != nil {
		return err
	}
	f, err := logfile.OpenForProcess(logPath)
	if err != nil {
		return err
	}
	cmd := exec.Command(o.SingBoxPath, "run", "--disable-color", "-c", p)
	cmd.Dir = o.SingBoxDir
	cmd.Stdout = f
	cmd.Stderr = f
	detachServiceCommand(cmd)
	if err = cmd.Start(); err != nil {
		_ = f.Close()
		return err
	}
	pid := cmd.Process.Pid
	_ = f.Close()
	started := time.Now().Unix()
	if err = rawJoinRootCgroup(pid); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return failServiceStart(o, 0, 0, "sing-box 无法加入 root cgroup", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if err = writeServiceState(o.StateFile, "starting", int64(pid), started, 0, ""); err != nil {
		_ = cmd.Process.Kill()
		<-done
		return err
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case err = <-done:
		if err == nil {
			err = errors.New("核心进程已退出")
		}
		return failServiceStart(o, 0, started, "sing-box 启动后退出", err)
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return failServiceStart(o, 0, started, "启动已取消", ctx.Err())
	case <-timer.C:
	}
	if err = writeServiceState(o.StateFile, "running", int64(pid), started, 0, ""); err != nil {
		_ = cmd.Process.Kill()
		<-done
		return err
	}
	return nil
}
