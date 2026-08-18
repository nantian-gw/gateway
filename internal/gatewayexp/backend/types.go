package backend

import (
	"time"

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

type (
	LocalPolicyTargetReference = gatewayv1.LocalPolicyTargetReference
	PolicyAncestorStatus       = gatewayv1.PolicyAncestorStatus
	PolicyStatus               = gatewayv1.PolicyStatus
	SessionPersistence         = gatewaycorev1alpha2.SessionPersistence
)

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
	SlowStart      *SlowStartConfig           `json:"slowStart,omitempty"`
}

type ConsistentHashPolicy struct {
	KeyType    *HashKeyType `json:"keyType,omitempty"`
	HeaderName *string      `json:"headerName,omitempty"`
}

type SlowStartConfig struct {
	Window *time.Duration `json:"window,omitempty"`
}

type HealthCheckConfig struct {
	Type               *string         `json:"type,omitempty"`
	Path               *string         `json:"path,omitempty"`
	ExpectedStatus     *int32          `json:"expectedStatus,omitempty"`
	Interval           *time.Duration  `json:"interval,omitempty"`
	Timeout            *time.Duration  `json:"timeout,omitempty"`
	HealthyThreshold   *uint32         `json:"healthyThreshold,omitempty"`
	UnhealthyThreshold *uint32         `json:"unhealthyThreshold,omitempty"`
}

type OutlierDetectionConfig struct {
	Consecutive5xx     *uint32         `json:"consecutive5xx,omitempty"`
	Interval           *time.Duration  `json:"interval,omitempty"`
	BaseEjectionTime   *time.Duration  `json:"baseEjectionTime,omitempty"`
	MaxEjectionPercent *uint32         `json:"maxEjectionPercent,omitempty"`
}

type BackendLBPolicySpec struct {
	TargetRefs         []LocalPolicyTargetReference `json:"targetRefs"`
	SessionPersistence *SessionPersistence          `json:"sessionPersistence,omitempty"`
	LoadBalancing      *LoadBalancingPolicy         `json:"loadBalancing,omitempty"`
	CircuitBreaker     *CircuitBreakerConfig        `json:"circuitBreaker,omitempty"`
	HealthCheck        *HealthCheckConfig           `json:"healthCheck,omitempty"`
	OutlierDetection   *OutlierDetectionConfig      `json:"outlierDetection,omitempty"`
}

type CircuitBreakerConfig struct {
	MaxInflightRequests *int32 `json:"maxInflightRequests,omitempty"`
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
		if in.Spec.LoadBalancing.SlowStart != nil {
			ss := *in.Spec.LoadBalancing.SlowStart
			out.Spec.LoadBalancing.SlowStart = &ss
			if in.Spec.LoadBalancing.SlowStart.Window != nil {
				w := *in.Spec.LoadBalancing.SlowStart.Window
				out.Spec.LoadBalancing.SlowStart.Window = &w
			}
		}
	}
	if in.Spec.HealthCheck != nil {
		hc := *in.Spec.HealthCheck
		out.Spec.HealthCheck = &hc
		if in.Spec.HealthCheck.Type != nil {
			t := *in.Spec.HealthCheck.Type
			out.Spec.HealthCheck.Type = &t
		}
		if in.Spec.HealthCheck.Path != nil {
			p := *in.Spec.HealthCheck.Path
			out.Spec.HealthCheck.Path = &p
		}
		if in.Spec.HealthCheck.ExpectedStatus != nil {
			s := *in.Spec.HealthCheck.ExpectedStatus
			out.Spec.HealthCheck.ExpectedStatus = &s
		}
		if in.Spec.HealthCheck.Interval != nil {
			iv := *in.Spec.HealthCheck.Interval
			out.Spec.HealthCheck.Interval = &iv
		}
		if in.Spec.HealthCheck.Timeout != nil {
			to := *in.Spec.HealthCheck.Timeout
			out.Spec.HealthCheck.Timeout = &to
		}
		if in.Spec.HealthCheck.HealthyThreshold != nil {
			ht := *in.Spec.HealthCheck.HealthyThreshold
			out.Spec.HealthCheck.HealthyThreshold = &ht
		}
		if in.Spec.HealthCheck.UnhealthyThreshold != nil {
			uht := *in.Spec.HealthCheck.UnhealthyThreshold
			out.Spec.HealthCheck.UnhealthyThreshold = &uht
		}
	}
	if in.Spec.OutlierDetection != nil {
		od := *in.Spec.OutlierDetection
		out.Spec.OutlierDetection = &od
		if in.Spec.OutlierDetection.Consecutive5xx != nil {
			c := *in.Spec.OutlierDetection.Consecutive5xx
			out.Spec.OutlierDetection.Consecutive5xx = &c
		}
		if in.Spec.OutlierDetection.Interval != nil {
			iv := *in.Spec.OutlierDetection.Interval
			out.Spec.OutlierDetection.Interval = &iv
		}
		if in.Spec.OutlierDetection.BaseEjectionTime != nil {
			be := *in.Spec.OutlierDetection.BaseEjectionTime
			out.Spec.OutlierDetection.BaseEjectionTime = &be
		}
		if in.Spec.OutlierDetection.MaxEjectionPercent != nil {
			mep := *in.Spec.OutlierDetection.MaxEjectionPercent
			out.Spec.OutlierDetection.MaxEjectionPercent = &mep
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
		if in.Spec.LoadBalancing.SlowStart != nil {
			ss := *in.Spec.LoadBalancing.SlowStart
			out.Spec.LoadBalancing.SlowStart = &ss
			if in.Spec.LoadBalancing.SlowStart.Window != nil {
				w := *in.Spec.LoadBalancing.SlowStart.Window
				out.Spec.LoadBalancing.SlowStart.Window = &w
			}
		}
	}
	if in.Spec.HealthCheck != nil {
		hc := *in.Spec.HealthCheck
		out.Spec.HealthCheck = &hc
		if in.Spec.HealthCheck.Type != nil {
			t := *in.Spec.HealthCheck.Type
			out.Spec.HealthCheck.Type = &t
		}
		if in.Spec.HealthCheck.Path != nil {
			p := *in.Spec.HealthCheck.Path
			out.Spec.HealthCheck.Path = &p
		}
		if in.Spec.HealthCheck.ExpectedStatus != nil {
			s := *in.Spec.HealthCheck.ExpectedStatus
			out.Spec.HealthCheck.ExpectedStatus = &s
		}
		if in.Spec.HealthCheck.Interval != nil {
			iv := *in.Spec.HealthCheck.Interval
			out.Spec.HealthCheck.Interval = &iv
		}
		if in.Spec.HealthCheck.Timeout != nil {
			to := *in.Spec.HealthCheck.Timeout
			out.Spec.HealthCheck.Timeout = &to
		}
		if in.Spec.HealthCheck.HealthyThreshold != nil {
			ht := *in.Spec.HealthCheck.HealthyThreshold
			out.Spec.HealthCheck.HealthyThreshold = &ht
		}
		if in.Spec.HealthCheck.UnhealthyThreshold != nil {
			uht := *in.Spec.HealthCheck.UnhealthyThreshold
			out.Spec.HealthCheck.UnhealthyThreshold = &uht
		}
	}
	if in.Spec.OutlierDetection != nil {
		od := *in.Spec.OutlierDetection
		out.Spec.OutlierDetection = &od
		if in.Spec.OutlierDetection.Consecutive5xx != nil {
			c := *in.Spec.OutlierDetection.Consecutive5xx
			out.Spec.OutlierDetection.Consecutive5xx = &c
		}
		if in.Spec.OutlierDetection.Interval != nil {
			iv := *in.Spec.OutlierDetection.Interval
			out.Spec.OutlierDetection.Interval = &iv
		}
		if in.Spec.OutlierDetection.BaseEjectionTime != nil {
			be := *in.Spec.OutlierDetection.BaseEjectionTime
			out.Spec.OutlierDetection.BaseEjectionTime = &be
		}
		if in.Spec.OutlierDetection.MaxEjectionPercent != nil {
			mep := *in.Spec.OutlierDetection.MaxEjectionPercent
			out.Spec.OutlierDetection.MaxEjectionPercent = &mep
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
		if in.LoadBalancing.SlowStart != nil {
			ss := *in.LoadBalancing.SlowStart
			out.LoadBalancing.SlowStart = &ss
			if in.LoadBalancing.SlowStart.Window != nil {
				w := *in.LoadBalancing.SlowStart.Window
				out.LoadBalancing.SlowStart.Window = &w
			}
		}
	}
	if in.HealthCheck != nil {
		hc := *in.HealthCheck
		out.HealthCheck = &hc
		if in.HealthCheck.Type != nil {
			t := *in.HealthCheck.Type
			out.HealthCheck.Type = &t
		}
		if in.HealthCheck.Path != nil {
			p := *in.HealthCheck.Path
			out.HealthCheck.Path = &p
		}
		if in.HealthCheck.ExpectedStatus != nil {
			s := *in.HealthCheck.ExpectedStatus
			out.HealthCheck.ExpectedStatus = &s
		}
		if in.HealthCheck.Interval != nil {
			iv := *in.HealthCheck.Interval
			out.HealthCheck.Interval = &iv
		}
		if in.HealthCheck.Timeout != nil {
			to := *in.HealthCheck.Timeout
			out.HealthCheck.Timeout = &to
		}
		if in.HealthCheck.HealthyThreshold != nil {
			ht := *in.HealthCheck.HealthyThreshold
			out.HealthCheck.HealthyThreshold = &ht
		}
		if in.HealthCheck.UnhealthyThreshold != nil {
			uht := *in.HealthCheck.UnhealthyThreshold
			out.HealthCheck.UnhealthyThreshold = &uht
		}
	}
	if in.OutlierDetection != nil {
		od := *in.OutlierDetection
		out.OutlierDetection = &od
		if in.OutlierDetection.Consecutive5xx != nil {
			c := *in.OutlierDetection.Consecutive5xx
			out.OutlierDetection.Consecutive5xx = &c
		}
		if in.OutlierDetection.Interval != nil {
			iv := *in.OutlierDetection.Interval
			out.OutlierDetection.Interval = &iv
		}
		if in.OutlierDetection.BaseEjectionTime != nil {
			be := *in.OutlierDetection.BaseEjectionTime
			out.OutlierDetection.BaseEjectionTime = &be
		}
		if in.OutlierDetection.MaxEjectionPercent != nil {
			mep := *in.OutlierDetection.MaxEjectionPercent
			out.OutlierDetection.MaxEjectionPercent = &mep
		}
	}

	return out
}
