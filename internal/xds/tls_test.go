package xds

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nantian-gw/gateway/internal/config"
)

func TestLoadServerTLSConfigRequiresCertificatePair(t *testing.T) {
	t.Parallel()

	_, err := loadServerTLSConfig(config.GRPCTLSConfig{Enabled: true})
	if err == nil {
		t.Fatal("expected tls config error when cert and key are missing")
	}
}

func TestLoadServerTLSConfigBuildsMutualTLSSettings(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caCertPEM, caKeyPEM := generateCertificateAuthority(t)
	serverCertPEM, serverKeyPEM := generateLeafCertificate(t, caCertPEM, caKeyPEM, false)

	caPath := filepath.Join(dir, "ca.crt")
	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")

	writeFile(t, caPath, caCertPEM)
	writeFile(t, certPath, serverCertPEM)
	writeFile(t, keyPath, serverKeyPEM)

	tlsConfig, err := loadServerTLSConfig(config.GRPCTLSConfig{
		Enabled:           true,
		CertPath:          certPath,
		KeyPath:           keyPath,
		ClientCAPath:      caPath,
		RequireClientCert: true,
	})
	if err != nil {
		t.Fatalf("load tls config: %v", err)
	}

	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected min version: %v", tlsConfig.MinVersion)
	}
	if tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("unexpected client auth mode: %v", tlsConfig.ClientAuth)
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Fatalf("expected 1 server certificate, got %d", len(tlsConfig.Certificates))
	}
	if tlsConfig.ClientCAs == nil || len(tlsConfig.ClientCAs.Subjects()) == 0 {
		t.Fatal("expected client CA pool to be configured")
	}
	if tlsConfig.GetConfigForClient == nil {
		t.Fatal("expected tls config reload callback to be configured")
	}
}

func TestLoadServerTLSConfigReloadsCertificatePairForNewHandshakes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caCertPEM, caKeyPEM := generateCertificateAuthority(t)
	serverCertPEM, serverKeyPEM := generateLeafCertificate(t, caCertPEM, caKeyPEM, false)
	rotatedCertPEM, rotatedKeyPEM := generateLeafCertificateWithCommonName(
		t,
		caCertPEM,
		caKeyPEM,
		false,
		"nantian-controlplane-rotated",
	)

	certPath := filepath.Join(dir, "tls.crt")
	keyPath := filepath.Join(dir, "tls.key")
	writeFile(t, certPath, serverCertPEM)
	writeFile(t, keyPath, serverKeyPEM)

	tlsConfig, err := loadServerTLSConfig(config.GRPCTLSConfig{
		Enabled:  true,
		CertPath: certPath,
		KeyPath:  keyPath,
	})
	if err != nil {
		t.Fatalf("load tls config: %v", err)
	}

	initialCN := certificateCommonName(t, tlsConfig.Certificates[0])
	if initialCN != "nantian-controlplane" {
		t.Fatalf("unexpected initial common name: %q", initialCN)
	}

	writeFile(t, certPath, rotatedCertPEM)
	writeFile(t, keyPath, rotatedKeyPEM)

	reloaded, err := tlsConfig.GetConfigForClient(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("reload tls config: %v", err)
	}

	reloadedCN := certificateCommonName(t, reloaded.Certificates[0])
	if reloadedCN != "nantian-controlplane-rotated" {
		t.Fatalf("unexpected reloaded common name: %q", reloadedCN)
	}
}

func generateCertificateAuthority(t *testing.T) ([]byte, []byte) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "nantian-gw-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	raw, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("generate ca cert: %v", err)
	}

	return encodeCertificate(raw), encodePrivateKey(privateKey)
}

func generateLeafCertificate(t *testing.T, caCertPEM, caKeyPEM []byte, isClient bool) ([]byte, []byte) {
	t.Helper()

	commonName := "nantian-controlplane"
	if isClient {
		commonName = "nantian-dataplane"
	}

	return generateLeafCertificateWithCommonName(t, caCertPEM, caKeyPEM, isClient, commonName)
}

func generateLeafCertificateWithCommonName(
	t *testing.T,
	caCertPEM, caKeyPEM []byte,
	isClient bool,
	commonName string,
) ([]byte, []byte) {
	t.Helper()

	caCert, caKey := decodeAuthority(t, caCertPEM, caKeyPEM)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}

	extKeyUsage := x509.ExtKeyUsageServerAuth
	if isClient {
		extKeyUsage = x509.ExtKeyUsageClientAuth
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: commonName,
		},
		DNSNames:       []string{"nantian-controlplane.nantian-gw.svc.cluster.local", "localhost"},
		NotBefore:      time.Now().Add(-time.Hour),
		NotAfter:       time.Now().Add(24 * time.Hour),
		KeyUsage:       x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:    []x509.ExtKeyUsage{extKeyUsage},
		IsCA:           false,
		IPAddresses:    nil,
		EmailAddresses: nil,
	}

	raw, err := x509.CreateCertificate(rand.Reader, template, caCert, &privateKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("generate leaf cert: %v", err)
	}

	return encodeCertificate(raw), encodePrivateKey(privateKey)
}

func certificateCommonName(t *testing.T, certificate tls.Certificate) string {
	t.Helper()

	if len(certificate.Certificate) == 0 {
		t.Fatal("expected certificate chain")
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatalf("parse leaf certificate: %v", err)
	}
	return leaf.Subject.CommonName
}

func decodeAuthority(t *testing.T, certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey) {
	t.Helper()

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil {
		t.Fatal("decode ca cert")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		t.Fatalf("parse ca cert: %v", err)
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		t.Fatal("decode ca key")
	}
	key, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatalf("parse ca key: %v", err)
	}

	return cert, key
}

func encodeCertificate(raw []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})
}

func encodePrivateKey(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func writeFile(t *testing.T, path string, raw []byte) {
	t.Helper()

	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
