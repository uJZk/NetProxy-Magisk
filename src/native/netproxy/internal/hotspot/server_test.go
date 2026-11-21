package hotspot

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestHTTP2SiteAndShutdown(t *testing.T) {
	cert, key, err := NewSelfSignedCertificate("www.example.test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	pair, err := ParseCertificate("www.example.test", cert, key)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler([]Route{{Path: "/reply", JSON: json.RawMessage(`{"ok":true}`)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serveListener(ctx, listener, pair, handler) }()
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(cert) {
		t.Fatal("bad certificate")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "www.example.test"}, ForceAttemptHTTP2: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get("https://" + listener.Addr().String() + "/a/reply?x=1")
	if err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(data) != `{"ok":true}` || response.ProtoMajor != 2 || response.Header.Get("X-Checksum") == "" {
		t.Fatalf("response: %s %s %v", response.Proto, data, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("server did not stop")
	}
}
