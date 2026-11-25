package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/hotspot"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/logfile"
	moduleapp "github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/module"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/paths"
	"github.com/Fanju6/NetProxy-Magisk/src/native/netproxy/internal/worker"
)

func hotspotCertificate(dir string, s hotspot.Settings) (tls.Certificate, error) {
	if s.Certificate != "" {
		return hotspot.ParseCertificate(s.Domain, []byte(s.Certificate), []byte(s.PrivateKey))
	}
	path := filepath.Join(dir, "selfsigned.json")
	stored, err := hotspot.LoadSettings(path)
	if err == nil && stored.Domain == s.Domain && stored.Certificate != "" {
		pair, err := hotspot.ParseCertificate(s.Domain, []byte(stored.Certificate), []byte(stored.PrivateKey))
		if err == nil && pair.Leaf.NotAfter.After(time.Now().Add(30*24*time.Hour)) {
			return pair, nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return tls.Certificate{}, err
	}
	cert, key, err := hotspot.NewSelfSignedCertificate(s.Domain, time.Now())
	if err != nil {
		return tls.Certificate{}, err
	}
	stored = hotspot.DefaultSettings()
	stored.Domain, stored.Certificate, stored.PrivateKey = s.Domain, string(cert), string(key)
	if err = hotspot.SaveSettings(path, stored); err != nil {
		return tls.Certificate{}, err
	}
	return hotspot.ParseCertificate(s.Domain, cert, key)
}

func hotspotHasAddress(point hotspot.AccessPoint) bool {
	link, err := net.InterfaceByIndex(point.Index)
	if err != nil || link.Name != point.Name {
		return false
	}
	addrs, err := link.Addrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		ip, _, err := net.ParseCIDR(a.String())
		if err == nil && ip.Equal(net.ParseIP(point.SiteIP())) {
			return true
		}
	}
	return false
}

func hotspotLeasePath(options moduleapp.Options) string {
	return filepath.Join(paths.New(options.ModuleDir).DevRoot(), "hotspot-lease.json")
}

func writeHotspotLease(options moduleapp.Options, point hotspot.AccessPoint) error {
	path := hotspotLeasePath(options)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(point)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hotspot-lease-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func writeHotspotStatus(options moduleapp.Options, state, message string) error {
	path := filepath.Join(paths.New(options.ModuleDir).DevRoot(), "hotspot-status.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(struct {
		State     string `json:"state"`
		Message   string `json:"message"`
		WorkerPID int    `json:"worker_pid"`
	}{state, message, os.Getpid()})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hotspot-status-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// runHotspotHost keeps the site inside the existing Worker process. It retries
// transient interface and listener failures without stopping the management API.
func runHotspotHost(ctx context.Context, options moduleapp.Options, wake <-chan struct{}, report func(error)) {
	runHotspotHostWith(ctx, options, wake, report, hotspotHostDeps{
		watch: worker.WatchNetworkEvents, detect: hotspot.DetectAccessPoint, hasAddress: hotspotHasAddress,
		addAddress: hotspot.AddSiteAddress, removeAddress: hotspot.RemoveSiteAddress, serve: hotspot.ServeReady,
	})
}

type hotspotHostDeps struct {
	watch         func(context.Context, func()) error
	detect        func() (hotspot.AccessPoint, error)
	hasAddress    func(hotspot.AccessPoint) bool
	addAddress    func(hotspot.AccessPoint) (bool, error)
	removeAddress func(hotspot.AccessPoint, bool) error
	serve         func(context.Context, string, tls.Certificate, http.Handler, func()) error
}

func runHotspotHostWith(ctx context.Context, options moduleapp.Options, wake <-chan struct{}, report func(error), deps hotspotHostDeps) {
	events := make(chan struct{}, 1)
	notify := func() {
		select {
		case events <- struct{}{}:
		default:
		}
	}
	dir := filepath.Join(paths.New(options.ModuleDir).Config(), "hotspot")
	go func() {
		for ctx.Err() == nil {
			if err := deps.watch(ctx, notify); err != nil && ctx.Err() == nil {
				s, readErr := hotspot.LoadSettings(filepath.Join(dir, "site.json"))
				if readErr == nil && s.Enabled {
					report(fmt.Errorf("热点网络事件: %w", err))
				}
			}
			timer := time.NewTimer(30 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	var stop context.CancelFunc
	var done chan error
	var ready chan struct{}
	var point hotspot.AccessPoint
	var owns bool
	var active [32]byte
	recoverLease := func() error {
		path := hotspotLeasePath(options)
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var lease hotspot.AccessPoint
		if err := json.Unmarshal(data, &lease); err != nil || lease.Index <= 0 || lease.Name == "" {
			return errors.New("热点地址记录损坏，无法安全清理")
		}
		if deps.hasAddress(lease) {
			if err := deps.removeAddress(lease, true); err != nil {
				return fmt.Errorf("清理先前的热点地址: %w", err)
			}
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	shutdown := func() {
		if stop == nil {
			return
		}
		stop()
		if err := <-done; err != nil && ctx.Err() == nil {
			report(err)
		}
		if err := deps.removeAddress(point, owns); err != nil {
			report(fmt.Errorf("清理热点地址: %w", err))
		} else if owns {
			if err := os.Remove(hotspotLeasePath(options)); err != nil && !errors.Is(err, os.ErrNotExist) {
				report(err)
			}
		}
		stop, done, ready, owns = nil, nil, nil, false
	}
	defer shutdown()
	notify()
	retry := time.NewTimer(time.Hour)
	if !retry.Stop() {
		<-retry.C
	}
	defer retry.Stop()
	var retryCh <-chan time.Time
	status := func(state, message string) {
		level := "INFO"
		if state == "error" {
			level = "ERROR"
		}
		if options.LogDir != "" {
			_ = logfile.AppendEntry(filepath.Join(options.LogDir, "mitm.log"), logfile.Entry{Level: level, Component: "mitm", Event: "https.listener", Result: state, Message: message})
		}
		if err := writeHotspotStatus(options, state, message); err != nil {
			report(err)
		}
	}
	defer status("stopped", "后台站点已停止")
	failed := func(err error) {
		report(err)
		status("error", err.Error())
		retry.Reset(30 * time.Second)
		retryCh = retry.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-wake:
			if !ok {
				wake = nil
				continue
			}
		case <-events:
			if retryCh != nil {
				continue
			}
		case <-retryCh:
			retryCh = nil
		case <-ready:
			ready = nil
			status("running", "MITM HTTPS 服务已监听")
		case err := <-done:
			done = nil
			ready = nil
			if stop != nil {
				stop()
				stop = nil
			}
			if err := deps.removeAddress(point, owns); err != nil {
				report(fmt.Errorf("清理热点地址: %w", err))
			} else if owns {
				if err := os.Remove(hotspotLeasePath(options)); err != nil && !errors.Is(err, os.ErrNotExist) {
					report(err)
				}
			}
			owns = false
			if err == nil {
				err = errors.New("热点 HTTPS 站点意外退出")
			}
			failed(err)
			continue
		}
		if !retry.Stop() && retryCh != nil {
			select {
			case <-retry.C:
			default:
			}
		}
		retryCh = nil
		if stop == nil {
			if err := recoverLease(); err != nil {
				failed(err)
				continue
			}
		}
		s, err := hotspot.LoadSettings(filepath.Join(dir, "site.json"))
		if err != nil {
			shutdown()
			failed(err)
			continue
		}
		raw, err := moduleapp.RawEnabled(options)
		if err != nil {
			shutdown()
			failed(err)
			continue
		}
		if !s.Enabled || !raw {
			shutdown()
			if err := recoverLease(); err != nil {
				failed(err)
				continue
			}
			status("stopped", "MITM 已关闭")
			continue
		}
		var next hotspot.AccessPoint
		if s.TargetIP == "" {
			next, err = deps.detect()
		} else {
			var link *net.Interface
			link, err = net.InterfaceByName("lo")
			if err == nil {
				next = hotspot.AccessPoint{Name: link.Name, Index: link.Index, Address: s.TargetIP}
			}
		}
		if err != nil {
			shutdown()
			failed(err)
			continue
		}
		encoded, err := json.Marshal(s)
		if err != nil {
			shutdown()
			failed(err)
			continue
		}
		fingerprint := sha256.Sum256(encoded)
		if stop != nil && next == point && fingerprint == active && deps.hasAddress(point) {
			continue
		}
		shutdown()
		if err := recoverLease(); err != nil {
			failed(err)
			continue
		}
		handler, err := hotspot.NewHandler(s.Routes, func() any {
			state, stateErr := moduleapp.ReadServiceState(options.StateFile)
			core := "unknown"
			if stateErr == nil {
				core = state.State
			}
			return map[string]any{"interface": next.Name, "site_address": next.SiteIP(), "core_state": core}
		})
		if err != nil {
			failed(err)
			continue
		}
		handler.OnMatch = func(path string) {
			_ = logfile.AppendEntry(filepath.Join(options.LogDir, "mitm.log"), logfile.Entry{Level: "INFO", Component: "https", Event: "route.match", Result: "success", Message: path})
		}
		cert, err := hotspotCertificate(dir, s)
		if err != nil {
			failed(err)
			continue
		}
		owns, err = deps.addAddress(next)
		if err != nil {
			failed(err)
			continue
		}
		if owns {
			if err := writeHotspotLease(options, next); err != nil {
				cleanupErr := deps.removeAddress(next, true)
				owns = false
				failed(errors.Join(fmt.Errorf("记录热点地址: %w", err), cleanupErr))
				continue
			}
		}
		point, active = next, fingerprint
		siteCtx, cancel := context.WithCancel(ctx)
		stop, done = cancel, make(chan error, 1)
		ready = make(chan struct{}, 1)
		siteDone := done
		siteReady := ready
		status("starting", "正在启动 MITM HTTPS 服务")
		go func() {
			siteDone <- deps.serve(siteCtx, net.JoinHostPort(next.SiteIP(), "443"), cert, handler, func() { siteReady <- struct{}{} })
		}()
	}
}
