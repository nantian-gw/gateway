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
			Hooks:   []WasmHook{HookOnRequest, HookOnResponse},
			Config:  `{"key": "value"}`,
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