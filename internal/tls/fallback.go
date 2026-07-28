package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/nantian-gw/gateway/internal/ir"
)

const fallbackCAName = "nantian-gw-fallback-ca"

type FallbackCertManager struct {
	mu        sync.Mutex
	caCertPEM string
	caKeyPEM  string
}

func NewFallbackCertManager() *FallbackCertManager {
	return &FallbackCertManager{}
}

func (m *FallbackCertManager) EnsureCA() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.caKeyPEM != "" {
		return nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("fallback ca: generate key: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("fallback ca: marshal key: %w", err)
	}

	keyHash := sha256.Sum256(keyDER)
	serial := new(big.Int).SetBytes(keyHash[:16])

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: fallbackCAName,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("fallback ca: create cert: %w", err)
	}

	m.caCertPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))
	m.caKeyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))

	return nil
}

func (m *FallbackCertManager) IssueLeafCert(hostnames []string) (ir.SecretMaterial, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	caBlock, _ := pem.Decode([]byte(m.caCertPEM))
	if caBlock == nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback ca cert not initialized")
	}
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback: parse ca cert: %w", err)
	}

	caKeyBlock, _ := pem.Decode([]byte(m.caKeyPEM))
	if caKeyBlock == nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback ca key not initialized")
	}
	caKey, err := x509.ParseECPrivateKey(caKeyBlock.Bytes)
	if err != nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback: parse ca key: %w", err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback leaf: generate key: %w", err)
	}

	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	if err != nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback leaf: marshal key: %w", err)
	}
	leafKeyHash := sha256.Sum256(leafKeyDER)
	serial := new(big.Int).SetBytes(leafKeyHash[:16])

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: hostnames[0],
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		DNSNames: hostnames,
	}

	leafCertDER, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback leaf: create cert: %w", err)
	}

	leafCertPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafCertDER}))
	leafKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER}))

	return ir.SecretMaterial{
		Name:      fmt.Sprintf("_fallback_%s", hostnames[0]),
		Namespace: "_system",
		CertPEM:   leafCertPEM,
		KeyPEM:    leafKeyPEM,
	}, nil
}
