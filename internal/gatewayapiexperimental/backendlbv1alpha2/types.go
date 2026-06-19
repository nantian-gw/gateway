package backendlbv1alpha2

import (
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

	out := new(BackendLBPolicy)
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)

	if in.Spec.TargetRefs != nil {
		out.Spec.TargetRefs = make([]LocalPolicyTargetReference, len(in.Spec.TargetRefs))
		copy(out.Spec.TargetRefs, in.Spec.TargetRefs)
	}
	if in.Spec.SessionPersistence != nil {
		sp := *in.Spec.SessionPersistence
		out.Spec.SessionPersistence = &sp
	}
	if in.Spec.LoadBalancing != nil {
		lb := *in.Spec.LoadBalancing
		out.Spec.LoadBalancing = &lb
		if in.Spec.LoadBalancing.Type != nil {
			t := *in.Spec.LoadBalancing.Type
			out.Spec.LoadBalancing.Type = &t
		}
		if in.Spec.LoadBalancing.ConsistentHash != nil {
			ch := *in.Spec.LoadBalancing.ConsistentHash
			out.Spec.LoadBalancing.ConsistentHash = &ch
			if in.Spec.LoadBalancing.ConsistentHash.KeyType != nil {
				kt := *in.Spec.LoadBalancing.ConsistentHash.KeyType
				out.Spec.LoadBalancing.ConsistentHash.KeyType = &kt
			}
			if in.Spec.LoadBalancing.ConsistentHash.HeaderName != nil {
				hn := *in.Spec.LoadBalancing.ConsistentHash.HeaderName
				out.Spec.LoadBalancing.ConsistentHash.HeaderName = &hn
			}
		}
	}

	if in.Status.Ancestors != nil {
		out.Status.Ancestors = make([]gatewayv1.PolicyAncestorStatus, len(in.Status.Ancestors))
		for i := range in.Status.Ancestors {
			out.Status.Ancestors[i] = in.Status.Ancestors[i]
			if in.Status.Ancestors[i].Conditions != nil {
				out.Status.Ancestors[i].Conditions = make([]metav1.Condition, len(in.Status.Ancestors[i].Conditions))
				copy(out.Status.Ancestors[i].Conditions, in.Status.Ancestors[i].Conditions)
			}
		}
	}

	return out
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

	out := new(BackendLBPolicyList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]BackendLBPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

func (in *BackendLBPolicy) DeepCopyInto(out *BackendLBPolicy) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)

	if in.Spec.TargetRefs != nil {
		out.Spec.TargetRefs = make([]LocalPolicyTargetReference, len(in.Spec.TargetRefs))
		copy(out.Spec.TargetRefs, in.Spec.TargetRefs)
	}
	if in.Spec.SessionPersistence != nil {
		sp := *in.Spec.SessionPersistence
		out.Spec.SessionPersistence = &sp
	}
	if in.Spec.LoadBalancing != nil {
		lb := *in.Spec.LoadBalancing
		out.Spec.LoadBalancing = &lb
		if in.Spec.LoadBalancing.Type != nil {
			t := *in.Spec.LoadBalancing.Type
			out.Spec.LoadBalancing.Type = &t
		}
		if in.Spec.LoadBalancing.ConsistentHash != nil {
			ch := *in.Spec.LoadBalancing.ConsistentHash
			out.Spec.LoadBalancing.ConsistentHash = &ch
			if in.Spec.LoadBalancing.ConsistentHash.KeyType != nil {
				kt := *in.Spec.LoadBalancing.ConsistentHash.KeyType
				out.Spec.LoadBalancing.ConsistentHash.KeyType = &kt
			}
			if in.Spec.LoadBalancing.ConsistentHash.HeaderName != nil {
				hn := *in.Spec.LoadBalancing.ConsistentHash.HeaderName
				out.Spec.LoadBalancing.ConsistentHash.HeaderName = &hn
			}
		}
	}

	if in.Status.Ancestors != nil {
		out.Status.Ancestors = make([]gatewayv1.PolicyAncestorStatus, len(in.Status.Ancestors))
		for i := range in.Status.Ancestors {
			out.Status.Ancestors[i] = in.Status.Ancestors[i]
			if in.Status.Ancestors[i].Conditions != nil {
				out.Status.Ancestors[i].Conditions = make([]metav1.Condition, len(in.Status.Ancestors[i].Conditions))
				copy(out.Status.Ancestors[i].Conditions, in.Status.Ancestors[i].Conditions)
			}
		}
	}
}

func (in *BackendLBPolicySpec) DeepCopy() *BackendLBPolicySpec {
	if in == nil {
		return nil
	}

	out := new(BackendLBPolicySpec)
	*out = *in
	if in.TargetRefs != nil {
		out.TargetRefs = make([]LocalPolicyTargetReference, len(in.TargetRefs))
		copy(out.TargetRefs, in.TargetRefs)
	}
	if in.SessionPersistence != nil {
		sp := *in.SessionPersistence
		out.SessionPersistence = &sp
	}
	if in.LoadBalancing != nil {
		lb := *in.LoadBalancing
		out.LoadBalancing = &lb
		if in.LoadBalancing.Type != nil {
			t := *in.LoadBalancing.Type
			out.LoadBalancing.Type = &t
		}
		if in.LoadBalancing.ConsistentHash != nil {
			ch := *in.LoadBalancing.ConsistentHash
			out.LoadBalancing.ConsistentHash = &ch
			if in.LoadBalancing.ConsistentHash.KeyType != nil {
				kt := *in.LoadBalancing.ConsistentHash.KeyType
				out.LoadBalancing.ConsistentHash.KeyType = &kt
			}
			if in.LoadBalancing.ConsistentHash.HeaderName != nil {
				hn := *in.LoadBalancing.ConsistentHash.HeaderName
				out.LoadBalancing.ConsistentHash.HeaderName = &hn
			}
		}
	}

	return out
}
