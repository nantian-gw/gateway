package tokenpolicyv1alpha1

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var GroupVersion = schema.GroupVersion{Group: "gateway.nantian.dev", Version: "v1alpha1"}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type TokenPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TokenPolicySpec   `json:"spec,omitempty"`
	Status            TokenPolicyStatus `json:"status,omitempty"`
}

type TokenPolicySpec struct {
	TargetRefs        []gatewayv1.LocalPolicyTargetReference `json:"targetRefs"`
	TokensPerMinute   uint64                                 `json:"tokensPerMinute,omitempty"`
	TokensPerHour     uint64                                 `json:"tokensPerHour,omitempty"`
	RequestsPerMinute uint64                                 `json:"requestsPerMinute,omitempty"`
	Scope             string                                 `json:"scope,omitempty"`
	Burst             float64                                `json:"burst,omitempty"`
	OnLimit           string                                 `json:"onLimit,omitempty"`
}

type TokenPolicyStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type TokenPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TokenPolicy `json:"items"`
}

func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &TokenPolicy{}, &TokenPolicyList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

func (in *TokenPolicy) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *TokenPolicy) DeepCopy() *TokenPolicy {
	if in == nil {
		return nil
	}
	var out TokenPolicy
	mustRoundTrip(in, &out)
	return &out
}

func (in *TokenPolicyList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *TokenPolicyList) DeepCopy() *TokenPolicyList {
	if in == nil {
		return nil
	}
	var out TokenPolicyList
	mustRoundTrip(in, &out)
	return &out
}

func (in *TokenPolicySpec) DeepCopy() *TokenPolicySpec {
	if in == nil {
		return nil
	}
	var out TokenPolicySpec
	mustRoundTrip(in, &out)
	return &out
}

func (in *TokenPolicyStatus) DeepCopy() *TokenPolicyStatus {
	if in == nil {
		return nil
	}
	var out TokenPolicyStatus
	mustRoundTrip(in, &out)
	return &out
}

func mustRoundTrip(in any, out any) {
	data, err := json.Marshal(in)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, out); err != nil {
		return
	}
}