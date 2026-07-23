package wasmplugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeepCopyRoundtrip(t *testing.T) {
	original := &WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-plugin",
			Namespace: "default",
		},
		Spec: WasmPluginSpec{
			Wasm: WasmSource{
				URL:    "https://example.com/plugin.wasm",
				SHA256: "abcdef",
			},
			Hooks:  []WasmHook{HookOnRequest, HookOnResponse},
			Config: `{"key": "value"}`,
			Sandbox: WasmSandbox{
				MaxMemoryBytes:     65536,
				MaxExecutionTimeMs: 100,
				AllowNetwork:       true,
			},
		},
	}
	copied := original.DeepCopy()
	assert.Equal(t, original, copied)
	assert.NotSame(t, original, copied)
}

func TestWasmPluginDeepCopy_FullConfig(t *testing.T) {
	plugin := &WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "auth-plugin", Namespace: "default"},
		Spec: WasmPluginSpec{
			Wasm: WasmSource{
				URL:    "oci://registry.example.com/auth-plugin:v1",
				SHA256: "abc123",
			},
			TargetRefs: []WasmPluginTargetRef{
				{Group: "", Kind: "Service", Name: "api-svc"},
			},
			Hooks:  []WasmHook{HookOnRequest, HookOnResponse},
			Config: `{"header": "x-auth-token"}`,
			Sandbox: WasmSandbox{
				MaxMemoryBytes:     16 * 1024 * 1024,
				MaxExecutionTimeMs: 10,
				AllowNetwork:       false,
				AllowFileSystem:    false,
			},
		},
	}
	copied := plugin.DeepCopy()
	assert.Equal(t, plugin.Spec.Wasm.URL, copied.Spec.Wasm.URL)
	assert.Equal(t, 2, len(copied.Spec.Hooks))
	assert.Equal(t, HookOnRequest, copied.Spec.Hooks[0])
	assert.Equal(t, uint64(16*1024*1024), copied.Spec.Sandbox.MaxMemoryBytes)
}

func TestWasmPluginDeepCopy_InlineWasm(t *testing.T) {
	plugin := &WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "inline-plugin", Namespace: "default"},
		Spec: WasmPluginSpec{
			Wasm: WasmSource{
				Inline: "AGFzbQEAAAAB...",
				SHA256: "def456",
			},
			Hooks: []WasmHook{HookOnStreamChunk},
		},
	}
	copied := plugin.DeepCopy()
	assert.Equal(t, "AGFzbQEAAAAB...", copied.Spec.Wasm.Inline)
	assert.Equal(t, 1, len(copied.Spec.Hooks))
}

func TestWasmPluginDeepCopy_ConfigMap(t *testing.T) {
	plugin := &WasmPlugin{
		Spec: WasmPluginSpec{
			Wasm: WasmSource{
				ConfigMap: &WasmConfigMapRef{Name: "wasm-cm", Key: "plugin.wasm"},
			},
		},
	}
	copied := plugin.DeepCopy()
	assert.NotNil(t, copied.Spec.Wasm.ConfigMap)
	assert.Equal(t, "wasm-cm", copied.Spec.Wasm.ConfigMap.Name)
	assert.NotSame(t, plugin.Spec.Wasm.ConfigMap, copied.Spec.Wasm.ConfigMap)
}

func TestWasmPluginDeepCopy_Nil(t *testing.T) {
	var p *WasmPlugin
	assert.Nil(t, p.DeepCopy())
	assert.Nil(t, p.DeepCopyObject())
}

func TestWasmPluginDeepCopyInto(t *testing.T) {
	src := &WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "src", Namespace: "ns"},
		Spec: WasmPluginSpec{
			Wasm: WasmSource{URL: "oci://reg.example.com/plugin:v1", SHA256: "abc"},
			TargetRefs: []WasmPluginTargetRef{
				{Group: "", Kind: "Service", Name: "api"},
			},
			Hooks: []WasmHook{HookOnRequest},
			Sandbox: WasmSandbox{
				MaxMemoryBytes:     8192,
				MaxExecutionTimeMs: 5,
			},
		},
	}
	dst := &WasmPlugin{}
	src.DeepCopyInto(dst)
	assert.Equal(t, "src", dst.Name)
	assert.Equal(t, src.Spec.Wasm.URL, dst.Spec.Wasm.URL)
	assert.NotSame(t, &src.Spec.TargetRefs, &dst.Spec.TargetRefs)
}

func TestWasmPluginListDeepCopy(t *testing.T) {
	list := &WasmPluginList{
		Items: []WasmPlugin{
			{ObjectMeta: metav1.ObjectMeta{Name: "wp1", Namespace: "ns1"}, Spec: WasmPluginSpec{Wasm: WasmSource{URL: "u1"}, Hooks: []WasmHook{HookOnRequest}}},
			{ObjectMeta: metav1.ObjectMeta{Name: "wp2", Namespace: "ns2"}, Spec: WasmPluginSpec{Wasm: WasmSource{Inline: "..."}, Hooks: []WasmHook{HookOnResponse}}},
		},
	}
	copied := list.DeepCopy()
	assert.Equal(t, 2, len(copied.Items))
	assert.Equal(t, "wp1", copied.Items[0].Name)
	assert.Equal(t, "u1", copied.Items[0].Spec.Wasm.URL)
}

func TestWasmPluginListDeepCopy_Nil(t *testing.T) {
	var list *WasmPluginList
	assert.Nil(t, list.DeepCopy())
	assert.Nil(t, list.DeepCopyObject())
}

func TestWasmPluginSpecDeepCopy(t *testing.T) {
	cmRef := &WasmConfigMapRef{Name: "cm", Key: "plugin.wasm"}
	spec := &WasmPluginSpec{
		Wasm: WasmSource{
			ConfigMap: cmRef,
			SHA256:    "sha256:abc",
		},
		TargetRefs: []WasmPluginTargetRef{
			{Group: "gateway.nantian.dev", Kind: "HTTPRoute", Name: "route1"},
		},
		Hooks:  []WasmHook{HookOnRequest, HookOnResponse, HookOnStreamChunk},
		Config: `{"x": 1}`,
		Sandbox: WasmSandbox{
			MaxMemoryBytes:     65536,
			MaxExecutionTimeMs: 50,
			AllowNetwork:       true,
			AllowFileSystem:    false,
		},
	}
	copied := spec.DeepCopy()
	assert.Equal(t, 3, len(copied.Hooks))
	assert.Equal(t, spec.Wasm.SHA256, copied.Wasm.SHA256)
	assert.NotNil(t, copied.Wasm.ConfigMap)
	assert.Equal(t, "cm", copied.Wasm.ConfigMap.Name)
	assert.NotSame(t, spec.Wasm.ConfigMap, copied.Wasm.ConfigMap)
	assert.NotSame(t, &spec.TargetRefs, &copied.TargetRefs)
	assert.NotSame(t, &spec.Hooks, &copied.Hooks)
}

func TestWasmPluginSpecDeepCopy_Nil(t *testing.T) {
	var spec *WasmPluginSpec
	assert.Nil(t, spec.DeepCopy())
}

func TestWasmPluginStatusDeepCopy(t *testing.T) {
	status := &WasmPluginStatus{
		Conditions: []metav1.Condition{
			{Type: "Programmed", Status: "True", Reason: "Applied", Message: "ok"},
		},
	}
	copied := status.DeepCopy()
	assert.Equal(t, 1, len(copied.Conditions))
	assert.Equal(t, "Programmed", copied.Conditions[0].Type)
	assert.NotSame(t, &status.Conditions, &copied.Conditions)
}

func TestWasmPluginStatusDeepCopy_Nil(t *testing.T) {
	var status *WasmPluginStatus
	assert.Nil(t, status.DeepCopy())
}
