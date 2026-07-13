package translator

import (
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/gwexp/wasmplugin"
	"github.com/nantian-gw/gateway/internal/ir"
)

const (
	wasmDownloadTimeout           = 30 * time.Second
	wasmConfigMapDefaultKey       = "plugin.wasm"
	wasmConfigMapDefaultKeyLegacy = ""
)

func referencedConfigMapKeysForWasmPlugins(plugins []wasmplugin.WasmPlugin) []client.ObjectKey {
	keys := make([]client.ObjectKey, 0, len(plugins))
	for _, p := range plugins {
		if p.Spec.Wasm.ConfigMap != nil && p.Spec.Wasm.ConfigMap.Name != "" {
			keys = append(keys, client.ObjectKey{
				Namespace: p.Namespace,
				Name:      p.Spec.Wasm.ConfigMap.Name,
			})
		}
	}
	return keys
}

func wasmConfigMapData(configMaps []corev1.ConfigMap, namespace, name, key string) []byte {
	for _, cm := range configMaps {
		if cm.Namespace == namespace && cm.Name == name {
			if key == "" || key == wasmConfigMapDefaultKeyLegacy {
				key = wasmConfigMapDefaultKey
			}
			return cm.BinaryData[key]
		}
	}
	return nil
}

func translateWasmPlugin(p wasmplugin.WasmPlugin, configMaps []corev1.ConfigMap, logger *slog.Logger) ir.WasmPluginConfig {
	if logger == nil {
		logger = slog.Default()
	}
	cfg := ir.WasmPluginConfig{
		Name:       p.Name,
		Namespace:  p.Namespace,
		SHA256:     p.Spec.Wasm.SHA256,
		ConfigJSON: p.Spec.Config,
		Sandbox: ir.WasmSandboxConfig{
			MaxMemoryBytes:     p.Spec.Sandbox.MaxMemoryBytes,
			MaxExecutionTimeMs: p.Spec.Sandbox.MaxExecutionTimeMs,
			AllowNetwork:       p.Spec.Sandbox.AllowNetwork,
			AllowFileSystem:    p.Spec.Sandbox.AllowFileSystem,
		},
	}
	for _, h := range p.Spec.Hooks {
		cfg.Hooks = append(cfg.Hooks, string(h))
	}
	if p.Spec.Wasm.Inline != "" {
		decoded, err := base64.StdEncoding.DecodeString(p.Spec.Wasm.Inline)
		if err != nil {
			logger.Warn("wasm plugin: failed to decode inline wasm bytes",
				"namespace", p.Namespace,
				"name", p.Name,
				"error", err,
			)
		} else {
			cfg.WasmBytes = decoded
		}
	}
	if p.Spec.Wasm.URL != "" {
		wasmBytes, err := downloadWasmURL(p.Spec.Wasm.URL)
		if err != nil {
			logger.Warn("wasm plugin: failed to download wasm from URL",
				"namespace", p.Namespace,
				"name", p.Name,
				"url", p.Spec.Wasm.URL,
				"error", err,
			)
		} else {
			cfg.WasmBytes = wasmBytes
		}
	}
	if p.Spec.Wasm.ConfigMap != nil {
		key := p.Spec.Wasm.ConfigMap.Key
		if key == "" {
			key = wasmConfigMapDefaultKey
		}
		cfg.WasmBytes = wasmConfigMapData(configMaps, p.Namespace, p.Spec.Wasm.ConfigMap.Name, key)
	}
	return cfg
}

func translateWasmPlugins(plugins []wasmplugin.WasmPlugin, configMaps []corev1.ConfigMap, logger *slog.Logger) map[string]ir.WasmPluginConfig {
	if logger == nil {
		logger = slog.Default()
	}
	result := make(map[string]ir.WasmPluginConfig, len(plugins))
	for _, p := range plugins {
		key := backendObjectKey(p.Namespace, p.Name)
		result[key] = translateWasmPlugin(p, configMaps, logger)
	}
	return result
}

func downloadWasmURL(rawURL string) ([]byte, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		// Allow HTTP only for loopback addresses (test/development).
		// Production must use HTTPS.
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("only https scheme is allowed, got %q", u.Scheme)
		}
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL has no host")
	}
	ip := net.ParseIP(u.Hostname())
	if ip != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast()) && !ip.IsLoopback() {
		return nil, fmt.Errorf("URL points to restricted network address: %s", u.Host)
	}
	client := &http.Client{Timeout: wasmDownloadTimeout}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("download wasm plugin from %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 50<<20))
}
