package policies

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
)

func TestTranslateWasmPlugin(t *testing.T) {
	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				SHA256: "abc123",
			},
			Hooks: []wasmplugin.WasmHook{
				wasmplugin.HookOnRequest,
				wasmplugin.HookOnResponse,
			},
			Config: `{"key": "value"}`,
			Sandbox: wasmplugin.WasmSandbox{
				MaxMemoryBytes:     10485760,
				MaxExecutionTimeMs: 100,
			},
		},
	}
	cfg := TranslateWasmPlugin(p, nil, nil)
	if cfg.Name != "test" {
		t.Errorf("expected name test, got %s", cfg.Name)
	}
	if cfg.Namespace != "default" {
		t.Errorf("expected default ns, got %s", cfg.Namespace)
	}
	if cfg.SHA256 != "abc123" {
		t.Errorf("expected sha256 abc123, got %s", cfg.SHA256)
	}
	if cfg.ConfigJSON != `{"key": "value"}` {
		t.Errorf("expected config, got %s", cfg.ConfigJSON)
	}
	if len(cfg.Hooks) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(cfg.Hooks))
	}
	if cfg.Sandbox.MaxMemoryBytes != 10485760 {
		t.Errorf("wrong max memory")
	}
	if cfg.Sandbox.MaxExecutionTimeMs != 100 {
		t.Errorf("wrong max exec time")
	}
	if cfg.WasmBytes != nil {
		t.Errorf("expected nil WasmBytes when no ConfigMap, got %v", cfg.WasmBytes)
	}
}

func TestTranslateWasmPluginFromConfigMap(t *testing.T) {
	wasmBytes := []byte("mock wasm binary data")
	configMaps := []corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "my-plugin", Namespace: "default"},
			BinaryData: map[string][]byte{
				"plugin.wasm": wasmBytes,
			},
		},
	}

	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-plugin", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				ConfigMap: &wasmplugin.WasmConfigMapRef{
					Name: "my-plugin",
				},
			},
		},
	}
	cfg := TranslateWasmPlugin(p, configMaps, nil)
	if cfg.WasmBytes == nil {
		t.Fatal("expected WasmBytes from ConfigMap, got nil")
	}
	if !bytes.Equal(cfg.WasmBytes, wasmBytes) {
		t.Errorf("expected wasm bytes %q, got %q", string(wasmBytes), string(cfg.WasmBytes))
	}
}

func TestTranslateWasmPluginFromConfigMapCustomKey(t *testing.T) {
	wasmBytes := []byte("custom key data")
	configMaps := []corev1.ConfigMap{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "my-plugin", Namespace: "default"},
			BinaryData: map[string][]byte{
				"my-custom-key.wasm": wasmBytes,
			},
		},
	}

	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-plugin", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				ConfigMap: &wasmplugin.WasmConfigMapRef{
					Name: "my-plugin",
					Key:  "my-custom-key.wasm",
				},
			},
		},
	}
	cfg := TranslateWasmPlugin(p, configMaps, nil)
	if cfg.WasmBytes == nil {
		t.Fatal("expected WasmBytes from ConfigMap with custom key, got nil")
	}
	if !bytes.Equal(cfg.WasmBytes, wasmBytes) {
		t.Errorf("expected wasm bytes %q, got %q", string(wasmBytes), string(cfg.WasmBytes))
	}
}

func TestTranslateWasmPluginConfigMapNotFound(t *testing.T) {
	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-plugin", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				ConfigMap: &wasmplugin.WasmConfigMapRef{
					Name: "nonexistent",
				},
			},
		},
	}
	cfg := TranslateWasmPlugin(p, nil, nil)
	if cfg.WasmBytes != nil {
		t.Errorf("expected nil WasmBytes for missing ConfigMap, got %v", cfg.WasmBytes)
	}
}

func TestTranslateWasmPlugins(t *testing.T) {
	plugins := []wasmplugin.WasmPlugin{
		{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "default"}},
	}
	result := TranslateWasmPlugins(plugins, nil, nil)
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
}

func TestReferencedConfigMapKeysForWasmPlugins(t *testing.T) {
	plugins := []wasmplugin.WasmPlugin{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
			Spec: wasmplugin.WasmPluginSpec{
				Wasm: wasmplugin.WasmSource{
					ConfigMap: &wasmplugin.WasmConfigMapRef{
						Name: "cm1",
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "other"},
			Spec: wasmplugin.WasmPluginSpec{
				Wasm: wasmplugin.WasmSource{
					ConfigMap: &wasmplugin.WasmConfigMapRef{
						Name: "cm2",
					},
				},
			},
		},
		// Plugin without ConfigMap — should be skipped
		{
			ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "default"},
			Spec: wasmplugin.WasmPluginSpec{
				Wasm: wasmplugin.WasmSource{
					URL: "https://example.com/plugin.wasm",
				},
			},
		},
	}
	keys := ReferencedConfigMapKeysForWasmPlugins(plugins)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].Namespace != "default" || keys[0].Name != "cm1" {
		t.Errorf("unexpected key[0]: %s/%s", keys[0].Namespace, keys[0].Name)
	}
	if keys[1].Namespace != "other" || keys[1].Name != "cm2" {
		t.Errorf("unexpected key[1]: %s/%s", keys[1].Namespace, keys[1].Name)
	}
}

func TestTranslateWasmPluginFromInline(t *testing.T) {
	wasmBytes := []byte("mock wasm binary data from inline")
	encoded := base64.StdEncoding.EncodeToString(wasmBytes)

	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "inline-plugin", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				Inline: encoded,
				SHA256: "inline-sha256",
			},
		},
	}
	cfg := TranslateWasmPlugin(p, nil, nil)
	if cfg.WasmBytes == nil {
		t.Fatal("expected WasmBytes from inline, got nil")
	}
	if !bytes.Equal(cfg.WasmBytes, wasmBytes) {
		t.Errorf("expected wasm bytes %q, got %q", string(wasmBytes), string(cfg.WasmBytes))
	}
	if cfg.SHA256 != "inline-sha256" {
		t.Errorf("expected sha256 inline-sha256, got %s", cfg.SHA256)
	}
}

func TestTranslateWasmPluginFromInlineDecodeError(t *testing.T) {
	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-inline", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				Inline: "this-is-not-valid-base64!!!",
			},
		},
	}
	cfg := TranslateWasmPlugin(p, nil, nil)
	if cfg.WasmBytes != nil {
		t.Errorf("expected nil WasmBytes for invalid base64, got %v", cfg.WasmBytes)
	}
}

func TestTranslateWasmPluginFromURL(t *testing.T) {
	wasmBytes := []byte("mock wasm binary data from url")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(wasmBytes)
	}))
	defer server.Close()

	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "url-plugin", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				URL: server.URL + "/plugin.wasm",
			},
		},
	}
	cfg := TranslateWasmPlugin(p, nil, nil)
	if cfg.WasmBytes == nil {
		t.Fatal("expected WasmBytes from URL, got nil")
	}
	if !bytes.Equal(cfg.WasmBytes, wasmBytes) {
		t.Errorf("expected wasm bytes %q, got %q", string(wasmBytes), string(cfg.WasmBytes))
	}
}

func TestTranslateWasmPluginFromURLHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "url-error", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				URL: server.URL + "/not-found.wasm",
			},
		},
	}
	cfg := TranslateWasmPlugin(p, nil, nil)
	if cfg.WasmBytes != nil {
		t.Errorf("expected nil WasmBytes for HTTP error, got %v", cfg.WasmBytes)
	}
}

func TestTranslateWasmPluginURLOverridesInline(t *testing.T) {
	// When both inline and URL are set, URL overrides (processed second)
	wasmBytes := []byte("url data wins")
	encoded := base64.StdEncoding.EncodeToString([]byte("inline data"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(wasmBytes)
	}))
	defer server.Close()

	p := wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "url-override", Namespace: "default"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm: wasmplugin.WasmSource{
				Inline: encoded,
				URL:    server.URL + "/plugin.wasm",
			},
		},
	}
	cfg := TranslateWasmPlugin(p, nil, nil)
	if cfg.WasmBytes == nil {
		t.Fatal("expected WasmBytes, got nil")
	}
	if !bytes.Equal(cfg.WasmBytes, wasmBytes) {
		t.Errorf("expected url bytes %q, got %q", string(wasmBytes), string(cfg.WasmBytes))
	}
}
