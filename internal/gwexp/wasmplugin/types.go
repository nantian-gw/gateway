package wasmplugin

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var GroupVersion = schema.GroupVersion{Group: "gateway.nantian.dev", Version: "v1alpha1"}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type WasmPlugin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WasmPluginSpec   `json:"spec,omitempty"`
	Status            WasmPluginStatus `json:"status,omitempty"`
}

type WasmPluginSpec struct {
	Wasm       WasmSource            `json:"wasm"`
	TargetRefs []WasmPluginTargetRef `json:"targetRefs,omitempty"`
	Hooks      []WasmHook            `json:"hooks,omitempty"`
	Config     string                `json:"config,omitempty"`
	Sandbox    WasmSandbox           `json:"sandbox,omitempty"`
}

type WasmSource struct {
	URL       string            `json:"url,omitempty"`
	ConfigMap *WasmConfigMapRef `json:"configMap,omitempty"`
	Inline    string            `json:"inline,omitempty"`
	SHA256    string            `json:"sha256,omitempty"`
}

type WasmConfigMapRef struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

type WasmPluginTargetRef struct {
	Group string `json:"group"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
}

type WasmHook string

const (
	HookOnRequest     WasmHook = "onRequest"
	HookOnResponse    WasmHook = "onResponse"
	HookOnStreamChunk WasmHook = "onStreamChunk"
)

type WasmSandbox struct {
	MaxMemoryBytes     uint64 `json:"maxMemoryBytes,omitempty"`
	MaxExecutionTimeMs uint64 `json:"maxExecutionTimeMs,omitempty"`
	AllowNetwork       bool   `json:"allowNetwork,omitempty"`
	AllowFileSystem    bool   `json:"allowFileSystem,omitempty"`
}

type WasmPluginStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type WasmPluginList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WasmPlugin `json:"items"`
}

func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &WasmPlugin{}, &WasmPluginList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

func (in *WasmPlugin) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *WasmPlugin) DeepCopy() *WasmPlugin {
	if in == nil {
		return nil
	}
	out := new(WasmPlugin)
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.TargetRefs != nil {
		out.Spec.TargetRefs = make([]WasmPluginTargetRef, len(in.Spec.TargetRefs))
		copy(out.Spec.TargetRefs, in.Spec.TargetRefs)
	}
	if in.Spec.Hooks != nil {
		out.Spec.Hooks = make([]WasmHook, len(in.Spec.Hooks))
		copy(out.Spec.Hooks, in.Spec.Hooks)
	}
	if in.Spec.Wasm.ConfigMap != nil {
		cp := *in.Spec.Wasm.ConfigMap
		out.Spec.Wasm.ConfigMap = &cp
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
	return out
}

func (in *WasmPluginList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *WasmPluginList) DeepCopy() *WasmPluginList {
	if in == nil {
		return nil
	}
	out := new(WasmPluginList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]WasmPlugin, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

func (in *WasmPlugin) DeepCopyInto(out *WasmPlugin) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.TargetRefs != nil {
		out.Spec.TargetRefs = make([]WasmPluginTargetRef, len(in.Spec.TargetRefs))
		copy(out.Spec.TargetRefs, in.Spec.TargetRefs)
	}
	if in.Spec.Hooks != nil {
		out.Spec.Hooks = make([]WasmHook, len(in.Spec.Hooks))
		copy(out.Spec.Hooks, in.Spec.Hooks)
	}
	if in.Spec.Wasm.ConfigMap != nil {
		cp := *in.Spec.Wasm.ConfigMap
		out.Spec.Wasm.ConfigMap = &cp
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
}

func (in *WasmPluginSpec) DeepCopy() *WasmPluginSpec {
	if in == nil {
		return nil
	}
	out := new(WasmPluginSpec)
	*out = *in
	if in.TargetRefs != nil {
		out.TargetRefs = make([]WasmPluginTargetRef, len(in.TargetRefs))
		copy(out.TargetRefs, in.TargetRefs)
	}
	if in.Hooks != nil {
		out.Hooks = make([]WasmHook, len(in.Hooks))
		copy(out.Hooks, in.Hooks)
	}
	if in.Wasm.ConfigMap != nil {
		cp := *in.Wasm.ConfigMap
		out.Wasm.ConfigMap = &cp
	}
	return out
}

func (in *WasmPluginStatus) DeepCopy() *WasmPluginStatus {
	if in == nil {
		return nil
	}
	out := new(WasmPluginStatus)
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
	return out
}
