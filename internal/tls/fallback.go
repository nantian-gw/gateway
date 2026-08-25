package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	cryptotls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
)

const (
	fallbackCAName      = "nantian-gw-fallback-ca"
	fallbackCASecretKey = "tls.crt"
	fallbackCAKeyKey    = "tls.key"
	fallbackLeafPrefix  = "fallback.leaf."
)

type FallbackCertManager struct {
	mu        sync.Mutex
	caCertPEM string
	caKeyPEM  string
	leafCerts map[string]ir.SecretMaterial
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
	getErr := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fallbackCAName}, secret)
	if getErr == nil {
		if m.loadCAFromSecretDataLocked(secret.Data) == nil {
			return nil
		}
	}
	if getErr != nil && !kerrors.IsNotFound(getErr) {
		return fmt.Errorf("fallback ca: read secret: %w", getErr)
	}

	caCertPEM, caKeyPEM, err := generateFallbackCA()
	if err != nil {
		return err
	}

	if getErr == nil {
		secret.Type = corev1.SecretTypeTLS
		if secret.Labels == nil {
			secret.Labels = make(map[string]string, 2)
		}
		secret.Labels["app.kubernetes.io/managed-by"] = "nantian-gw"
		secret.Labels["nantian.dev/component"] = "fallback-ca"
		if secret.Data == nil {
			secret.Data = make(map[string][]byte, 2)
		}
		secret.Data[fallbackCASecretKey] = []byte(caCertPEM)
		secret.Data[fallbackCAKeyKey] = []byte(caKeyPEM)
		if err := cl.Update(ctx, secret); err != nil {
			return fmt.Errorf("fallback ca: repair secret: %w", err)
		}
		m.caCertPEM = caCertPEM
		m.caKeyPEM = caKeyPEM
		return nil
	}

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
			fallbackCASecretKey: []byte(caCertPEM),
			fallbackCAKeyKey:    []byte(caKeyPEM),
		},
	}

	if err := cl.Create(ctx, caSecret); err != nil {
		if kerrors.IsAlreadyExists(err) {
			return m.loadCASecretLocked(ctx, cl, namespace)
		}
		return fmt.Errorf("fallback ca: create secret: %w", err)
	}

	m.caCertPEM = caCertPEM
	m.caKeyPEM = caKeyPEM
	return nil
}

func (m *FallbackCertManager) loadCASecretLocked(ctx context.Context, cl client.Client, namespace string) error {
	secret := &corev1.Secret{}
	if err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fallbackCAName}, secret); err != nil {
		return fmt.Errorf("fallback ca: read secret: %w", err)
	}
	if err := m.loadCAFromSecretDataLocked(secret.Data); err != nil {
		return fmt.Errorf("fallback ca: read secret data: %w", err)
	}
	return nil
}

func (m *FallbackCertManager) loadCAFromSecretDataLocked(data map[string][]byte) error {
	certPEM, hasCert := data[fallbackCASecretKey]
	keyPEM, hasKey := data[fallbackCAKeyKey]
	if !hasCert || !hasKey || len(certPEM) == 0 || len(keyPEM) == 0 {
		return fmt.Errorf("missing %s or %s", fallbackCASecretKey, fallbackCAKeyKey)
	}
	if _, err := cryptotls.X509KeyPair(certPEM, keyPEM); err != nil {
		return fmt.Errorf("invalid key pair: %w", err)
	}
	m.caCertPEM = string(certPEM)
	m.caKeyPEM = string(keyPEM)
	return nil
}

func generateFallbackCA() (string, string, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("fallback ca: generate key: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("fallback ca: marshal key: %w", err)
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
		return "", "", fmt.Errorf("fallback ca: create cert: %w", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})),
		nil
}

func (m *FallbackCertManager) IssueLeafCert(
	ctx context.Context,
	cl client.Client,
	namespace string,
	hostnames []string,
) (ir.SecretMaterial, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hostnames = canonicalFallbackHostnames(hostnames)
	if len(hostnames) == 0 {
		return ir.SecretMaterial{}, fmt.Errorf("fallback leaf: no hostnames")
	}
	cacheKey := strings.Join(hostnames, "\x00")
	if leaf, ok := m.leafCerts[cacheKey]; ok {
		return leaf, nil
	}

	if cl != nil && namespace != "" {
		secret := &corev1.Secret{}
		err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fallbackCAName}, secret)
		if err != nil && !kerrors.IsNotFound(err) {
			return ir.SecretMaterial{}, fmt.Errorf("fallback leaf: read persisted material: %w", err)
		}
		if err == nil {
			if leaf, ok := fallbackLeafFromSecretData(secret.Data, cacheKey, hostnames); ok {
				return m.cacheLeafCert(cacheKey, leaf), nil
			}
		}
	}

	leaf, err := m.issueLeafCertLocked(hostnames)
	if err != nil {
		return ir.SecretMaterial{}, err
	}

	if cl != nil && namespace != "" {
		leaf, err = m.persistLeafCertLocked(ctx, cl, namespace, cacheKey, hostnames, leaf)
		if err != nil {
			return ir.SecretMaterial{}, err
		}
	}

	return m.cacheLeafCert(cacheKey, leaf), nil
}

func (m *FallbackCertManager) issueLeafCertLocked(hostnames []string) (ir.SecretMaterial, error) {
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
		Name:      fallbackLeafMaterialName(hostnames),
		Namespace: "_system",
		CertPEM:   leafCertPEM,
		KeyPEM:    leafKeyPEM,
	}, nil
}

func (m *FallbackCertManager) persistLeafCertLocked(
	ctx context.Context,
	cl client.Client,
	namespace string,
	cacheKey string,
	hostnames []string,
	leaf ir.SecretMaterial,
) (ir.SecretMaterial, error) {
	var persisted ir.SecretMaterial
	certKey, keyKey := fallbackLeafSecretDataKeys(cacheKey)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret := &corev1.Secret{}
		if err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: fallbackCAName}, secret); err != nil {
			return err
		}
		if existing, ok := fallbackLeafFromSecretData(secret.Data, cacheKey, hostnames); ok {
			persisted = existing
			return nil
		}
		if secret.Data == nil {
			secret.Data = make(map[string][]byte, 2)
		}
		secret.Data[certKey] = []byte(leaf.CertPEM)
		secret.Data[keyKey] = []byte(leaf.KeyPEM)
		if err := cl.Update(ctx, secret); err != nil {
			return err
		}
		persisted = leaf
		return nil
	})
	if err != nil {
		return ir.SecretMaterial{}, fmt.Errorf("fallback leaf: persist material: %w", err)
	}
	return persisted, nil
}

func (m *FallbackCertManager) cacheLeafCert(key string, leaf ir.SecretMaterial) ir.SecretMaterial {
	if m.leafCerts == nil {
		m.leafCerts = make(map[string]ir.SecretMaterial)
	}
	m.leafCerts[key] = leaf
	return leaf
}

func fallbackLeafFromSecretData(data map[string][]byte, cacheKey string, hostnames []string) (ir.SecretMaterial, bool) {
	certKey, keyKey := fallbackLeafSecretDataKeys(cacheKey)
	certPEM, hasCert := data[certKey]
	keyPEM, hasKey := data[keyKey]
	if !hasCert || !hasKey || len(certPEM) == 0 || len(keyPEM) == 0 {
		return ir.SecretMaterial{}, false
	}
	if _, err := cryptotls.X509KeyPair(certPEM, keyPEM); err != nil {
		return ir.SecretMaterial{}, false
	}
	return ir.SecretMaterial{
		Name:      fallbackLeafMaterialName(hostnames),
		Namespace: "_system",
		CertPEM:   string(certPEM),
		KeyPEM:    string(keyPEM),
	}, true
}

func fallbackLeafSecretDataKeys(cacheKey string) (string, string) {
	sum := sha256.Sum256([]byte(cacheKey))
	suffix := hex.EncodeToString(sum[:])
	return fallbackLeafPrefix + suffix + ".crt", fallbackLeafPrefix + suffix + ".key"
}

func fallbackLeafMaterialName(hostnames []string) string {
	return fmt.Sprintf("_fallback_%s", hostnames[0])
}

func canonicalFallbackHostnames(hostnames []string) []string {
	if len(hostnames) == 0 {
		return nil
	}

	out := make([]string, 0, len(hostnames))
	seen := make(map[string]struct{}, len(hostnames))
	for _, hostname := range hostnames {
		hostname = strings.TrimSpace(hostname)
		if hostname == "" {
			continue
		}
		if _, ok := seen[hostname]; ok {
			continue
		}
		seen[hostname] = struct{}{}
		out = append(out, hostname)
	}
	sort.Strings(out)
	return out
}
