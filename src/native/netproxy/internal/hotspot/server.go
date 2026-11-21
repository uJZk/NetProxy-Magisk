package hotspot

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// Serve hosts the site on a caller-selected hotspot address. It never falls
// back to a wildcard listener when the address or port is unavailable.
func Serve(ctx context.Context, address string, certificate tls.Certificate, handler http.Handler) error {
	return ServeReady(ctx, address, certificate, handler, nil)
}

// ServeReady calls ready only after the requested address has been bound.
func ServeReady(ctx context.Context, address string, certificate tls.Certificate, handler http.Handler, ready func()) error {
	if handler == nil || len(certificate.Certificate) == 0 || certificate.PrivateKey == nil {
		return errors.New("HTTPS 站点缺少处理器或证书")
	}
	endpoint, err := netip.ParseAddrPort(address)
	if err != nil || !endpoint.Addr().Is4() || endpoint.Addr().IsUnspecified() {
		return errors.New("HTTPS 站点必须绑定明确的热点 IPv4 地址")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("HTTPS 站点监听失败: %w", err)
	}
	if ready != nil {
		ready()
	}
	return serveListener(ctx, listener, certificate, handler)
}

func serveListener(ctx context.Context, listener net.Listener, certificate tls.Certificate, handler http.Handler) error {
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    16 << 10,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}},
	}
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
		}
		err := <-done
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
