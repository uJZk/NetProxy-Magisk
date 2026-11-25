package module

import (
	"context"
	"errors"
	"github.com/sagernet/sing-box/option"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/rawconfig"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/service"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
	sjson "github.com/sagernet/sing/common/json"
)

func ClearRawCache(ctx context.Context, o Options) error {
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return err
	}
	defer lock.release()
	_, _, meta := rawPaths(o)
	err = os.Remove(meta)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func RawPanelURL(o Options) (string, error) {
	_, source, _ := rawPaths(o)
	b, err := os.ReadFile(source)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var config struct {
		Services []struct {
			Type, Listen string
			Port         uint16 `json:"listen_port"`
			Dashboard    *option.APIDashboardOptions
			TLS          struct{ Enabled bool }
		}
		Experimental struct {
			Clash struct {
				Controller string `json:"external_controller"`
				UI         string `json:"external_ui"`
			} `json:"clash_api"`
		}
	}
	if err = sjson.Unmarshal(b, &config); err != nil {
		return "", err
	}
	endpoint := func(host string, port uint16, scheme, path string) string {
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		} else if host == "::" {
			host = "::1"
		}
		if port == 0 {
			return ""
		}
		if _, err := netip.ParseAddr(host); err != nil && host != "localhost" {
			return ""
		}
		u := url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.Itoa(int(port))), Path: path}
		return u.String()
	}
	for _, svc := range config.Services {
		if svc.Type != "api" {
			continue
		}
		if svc.Dashboard == nil || !svc.Dashboard.Enabled {
			continue
		}
		scheme := "http"
		if svc.TLS.Enabled {
			scheme = "https"
		}
		if address := endpoint(svc.Listen, svc.Port, scheme, "/dashboard/"); address != "" {
			return address, nil
		}
	}
	clash := config.Experimental.Clash
	if clash.UI != "" {
		host, p, err := net.SplitHostPort(clash.Controller)
		if err == nil {
			port, err := strconv.ParseUint(p, 10, 16)
			if err == nil {
				return endpoint(host, uint16(port), "http", "/ui/"), nil
			}
		}
	}
	return "", nil
}

// changeRaw snapshots each modified file under the lifecycle lock and restores
// the snapshot if validation, activation, or restarting the running core fails.
func changeRaw(ctx context.Context, o Options, change func(RawSettings, string) (RawSettings, error)) error {
	lock, err := waitLifecycleLock(ctx, o.StateFile)
	if err != nil {
		return err
	}
	defer lock.release()
	settings, err := readRawSettings(o)
	if err != nil {
		return err
	}
	sp, source, mp := rawPaths(o)
	type snapshot struct {
		path   string
		data   []byte
		exists bool
	}
	var old []snapshot
	for _, p := range []string{sp, source, mp} {
		b, e := os.ReadFile(p)
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		old = append(old, snapshot{p, b, e == nil})
	}
	state, err := ReadServiceState(o.StateFile)
	if err != nil {
		return err
	}
	running := service.FindProcess(o.SingBoxPath, int(state.PID)) > 0
	next, err := change(settings, source)
	if err == nil {
		err = writeRawSettings(o, next)
	}
	if err == nil {
		err = validateRawSite(ctx, o, source)
	}
	stopped := false
	if err == nil && running {
		err = StopService(ctx, o)
		if err == nil {
			stopped = true
			err = StartService(ctx, o)
		}
	}
	if err == nil {
		_ = worker.Wake(workerOptions(o))
		return nil
	}
	for _, s := range old {
		var restore error
		if s.exists {
			restore = writeRawFile(s.path, s.data)
		} else {
			restore = os.Remove(s.path)
			if errors.Is(restore, os.ErrNotExist) {
				restore = nil
			}
		}
		err = errors.Join(err, restore)
	}
	if stopped {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		err = errors.Join(err, StartService(recovery, o))
	}
	return err
}

func ImportRawFile(ctx context.Context, o Options, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = rawconfig.CheckDocument(data); err != nil {
		return err
	}
	return changeRaw(ctx, o, func(s RawSettings, source string) (RawSettings, error) {
		s.Enabled = true
		s.URL = ""
		s.Interval = 0
		s.UserAgent = ""
		_, _, meta := rawPaths(o)
		if err := os.Remove(meta); err != nil && !errors.Is(err, os.ErrNotExist) {
			return s, err
		}
		return s, writeRawFile(source, data)
	})
}
