package backendlbv1alpha2

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewaycorev1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	GroupVersion = schema.GroupVersion{Group: gatewayv1.GroupName, Version: "v1alpha2"}
	Install      = AddToScheme
)

const (
	PolicyConditionAccepted    = gatewayv1.PolicyConditionAccepted
	PolicyReasonAccepted       = gatewayv1.PolicyReasonAccepted
	PolicyReasonConflicted     = gatewayv1.PolicyReasonConflicted
	PolicyReasonInvalid        = gatewayv1.PolicyReasonInvalid
	PolicyReasonTargetNotFound = gatewayv1.PolicyReasonTargetNotFound
)

type LocalPolicyTargetReference = gatewayv1.LocalPolicyTargetReference
type PolicyAncestorStatus = gatewayv1.PolicyAncestorStatus
type PolicyStatus = gatewayv1.PolicyStatus
type SessionPersistence = gatewaycorev1alpha2.SessionPersistence

type LoadBalancingStrategyType string

const (
	LoadBalancingStrategyTypeRoundRobin     LoadBalancingStrategyType = "RoundRobin"
	LoadBalancingStrategyTypeConsistentHash LoadBalancingStrategyType = "ConsistentHash"
	LoadBalancingStrategyTypeLeastRequest   LoadBalancingStrategyType = "LeastRequest"
	LoadBalancingStrategyTypeRandom         LoadBalancingStrategyType = "Random"
)

type HashKeyType string

const (
	HashKeyTypeSourceIP HashKeyType = "SourceIP"
	HashKeyTypeHeader   HashKeyType = "Header"
	HashKeyTypeHostname HashKeyType = "Hostname"
)

type LoadBalancingPolicy struct {
	Type           *LoadBalancingStrategyType `json:"type,omitempty"`
	ConsistentHash *ConsistentHashPolicy      `json:"consistentHash,omitempty"`
}

type ConsistentHashPolicy struct {
	KeyType    *HashKeyType `json:"keyType,omitempty"`
	HeaderName *string      `json:"headerName,omitempty"`
}

type BackendLBPolicySpec struct {
	TargetRefs         []LocalPolicyTargetReference `json:"targetRefs"`
	SessionPersistence *SessionPersistence          `json:"sessionPersistence,omitempty"`
	LoadBalancing      *LoadBalancingPolicy         `json:"loadBalancing,omitempty"`
}

type BackendLBPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BackendLBPolicySpec `json:"spec,omitempty"`
	Status            PolicyStatus        `json:"status,omitempty"`
}

type BackendLBPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackendLBPolicy `json:"items"`
}

func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &BackendLBPolicy{}, &BackendLBPolicyList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

func (in *BackendLBPolicy) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}

	return in.DeepCopy()
}

func (in *BackendLBPolicy) DeepCopy() *BackendLBPolicy {
	if in == nil {
		return nil
	}

	var out BackendLBPolicy
	mustRoundTrip(in, &out)
	return &out
}

func (in *BackendLBPolicyList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}

	return in.DeepCopy()
}

func (in *BackendLBPolicyList) DeepCopy() *BackendLBPolicyList {
	if in == nil {
		return nil
	}

	var out BackendLBPolicyList
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
