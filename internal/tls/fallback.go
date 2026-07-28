package tls

import (
	"context"
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

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
)

const (
	fallbackCAName      = "nantian-gw-fallback-ca"
	fallbackCASecretKey = "tls.crt"
	fallbackCAKeyKey    = "tls.key"
)

type FallbackCertManager struct {
	mu        sync.Mutex
	caCertPEM string
	caKeyPEM  string
}

func NewFallbackCertManager() *FallbackCertManager {
	return &FallbackCertManager{}
}

func (m *FallbackCertManager) LoadOrCreateCA(ctx context.Context, cl client.Client, namespace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.caKeyPEM != "" {
		return nil
	}

	secret := &corev1.Secret{}
	err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fallbackCAName}, secret)
	if err == nil {
		certPEM, hasCert := secret.Data[fallbackCASecretKey]
		keyPEM, hasKey := secret.Data[fallbackCAKeyKey]
		if hasCert && hasKey {
			m.caCertPEM = string(certPEM)
			m.caKeyPEM = string(keyPEM)
			return nil
		}
	}
	if err != nil && !kerrors.IsNotFound(err) {
		return fmt.Errorf("fallback ca: read secret: %w", err)
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

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fallbackCAName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "nantian-gw",
				"nantian.dev/component":        "fallback-ca",
			},
		},
		Type: corev1.SecretTypeTLS,
		Data: map[string][]byte{
			fallbackCASecretKey: []byte(m.caCertPEM),
			fallbackCAKeyKey:    []byte(m.caKeyPEM),
		},
	}

	if err := cl.Create(ctx, caSecret); err != nil {
		if kerrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("fallback ca: create secret: %w", err)
	}

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
