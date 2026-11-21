package hotspot

import (
	"testing"
	"time"
)

func TestCertificateHostnameAndKey(t *testing.T) {
	cert, key, err := NewSelfSignedCertificate("www.example.com", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCertificate("www.example.com", cert, key); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCertificate("wrong.example.com", cert, key); err == nil {
		t.Fatal("accepted wrong hostname")
	}
	_, otherKey, err := NewSelfSignedCertificate("www.example.com", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseCertificate("www.example.com", cert, otherKey); err == nil {
		t.Fatal("accepted mismatched key")
	}
}
