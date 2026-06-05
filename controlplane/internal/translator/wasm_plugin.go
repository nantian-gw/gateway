package translator

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapiexperimental/wasmpluginv1alpha1"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
)

func referencedConfigMapKeysForWasmPlugins(plugins []wasmpluginv1alpha1.WasmPlugin) []client.ObjectKey {
	var keys []client.ObjectKey
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
			if key == "" {
				key = "plugin.wasm"
			}
			return cm.BinaryData[key]
		}
	}
	return nil
}

func translateWasmPlugin(p wasmpluginv1alpha1.WasmPlugin, configMaps []corev1.ConfigMap) ir.WasmPluginConfig {
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
			log.Printf("wasm plugin %s/%s: failed to decode inline wasm bytes: %v", p.Namespace, p.Name, err)
		} else {
			cfg.WasmBytes = decoded
		}
	}
	if p.Spec.Wasm.URL != "" {
		wasmBytes, err := downloadWasmURL(p.Spec.Wasm.URL)
		if err != nil {
			log.Printf("wasm plugin %s/%s: failed to download from %s: %v", p.Namespace, p.Name, p.Spec.Wasm.URL, err)
		} else {
			cfg.WasmBytes = wasmBytes
		}
	}
	if p.Spec.Wasm.ConfigMap != nil {
		key := p.Spec.Wasm.ConfigMap.Key
		if key == "" {
			key = "plugin.wasm"
		}
		cfg.WasmBytes = wasmConfigMapData(configMaps, p.Namespace, p.Spec.Wasm.ConfigMap.Name, key)
	}
	return cfg
}

func translateWasmPlugins(plugins []wasmpluginv1alpha1.WasmPlugin, configMaps []corev1.ConfigMap) map[string]ir.WasmPluginConfig {
	result := make(map[string]ir.WasmPluginConfig, len(plugins))
	for _, p := range plugins {
		key := backendObjectKey(p.Namespace, p.Name)
		result[key] = translateWasmPlugin(p, configMaps)
	}
	return result
}

func downloadWasmURL(url string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}