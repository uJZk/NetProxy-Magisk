package hotspot

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"time"
)

// NewSelfSignedCertificate creates a certificate for the configured hostname.
// Clients must explicitly trust it; a publicly trusted certificate can be supplied instead.
func NewSelfSignedCertificate(domain string, now time.Time) (certPEM, keyPEM []byte, err error) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
	if !validDomain(domain) {
		return nil, nil, errors.New("站点域名无效")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	cert := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain}, NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	private, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), nil
}

// ParseCertificate validates the key pair and its hostname before use.
func ParseCertificate(domain string, certPEM, keyPEM []byte) (tls.Certificate, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := leaf.VerifyHostname(domain); err != nil {
		return tls.Certificate{}, err
	}
	pair.Leaf = leaf
	return pair, nil
}
