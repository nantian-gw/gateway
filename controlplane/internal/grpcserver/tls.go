package grpcserver

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/config"
)

func serverOptionsFromConfig(
	tlsConfig config.GRPCTLSConfig,
	runtimeConfig config.GRPCRuntimeConfig,
) ([]grpc.ServerOption, grpcRuntimeSettings, error) {
	serverOptions, runtimeSettings := runtimeServerOptionsFromConfig(runtimeConfig)
	if !grpcTLSEnabled(tlsConfig) {
		return serverOptions, runtimeSettings, nil
	}

	tlsSettings, err := loadServerTLSConfig(tlsConfig)
	if err != nil {
		return nil, grpcRuntimeSettings{}, err
	}

	serverOptions = append(serverOptions, grpc.Creds(credentials.NewTLS(tlsSettings)))
	return serverOptions, runtimeSettings, nil
}

func loadServerTLSConfig(cfg config.GRPCTLSConfig) (*tls.Config, error) {
	tlsConfig, err := loadStaticServerTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	tlsConfig.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
		return loadStaticServerTLSConfig(cfg)
	}
	return tlsConfig, nil
}

func loadStaticServerTLSConfig(cfg config.GRPCTLSConfig) (*tls.Config, error) {
	if strings.TrimSpace(cfg.CertPath) == "" || strings.TrimSpace(cfg.KeyPath) == "" {
		return nil, fmt.Errorf("grpc tls requires certPath and keyPath")
	}

	certificate, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("load grpc tls certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
		NextProtos:   []string{"h2"},
	}

	if cfg.RequireClientCert && strings.TrimSpace(cfg.ClientCAPath) == "" {
		return nil, fmt.Errorf("grpc tls requireClientCert needs clientCAPath")
	}

	if caPath := strings.TrimSpace(cfg.ClientCAPath); caPath != "" {
		raw, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read grpc tls client ca: %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(raw) {
			return nil, fmt.Errorf("parse grpc tls client ca bundle")
		}

		tlsConfig.ClientCAs = pool
		if cfg.RequireClientCert {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		}
	}

	return tlsConfig, nil
}

func grpcTLSEnabled(cfg config.GRPCTLSConfig) bool {
	return cfg.Enabled || (strings.TrimSpace(cfg.CertPath) != "" && strings.TrimSpace(cfg.KeyPath) != "")
}
