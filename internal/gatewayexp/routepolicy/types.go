package routepolicy

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var GroupVersion = schema.GroupVersion{Group: "gateway.nantian.dev", Version: "v1alpha1"}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type RoutePolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RoutePolicySpec   `json:"spec,omitempty"`
	Status            RoutePolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type RoutePolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RoutePolicy `json:"items"`
}

type RoutePolicySpec struct {
	TargetRefs []gatewayv1.LocalPolicyTargetReference `json:"targetRefs"`
	Default    *RoutePolicyDefault                    `json:"default,omitempty"`
}

type RoutePolicyDefault struct {
	Timeout    *TimeoutConfig    `json:"timeout,omitempty"`
	BodyLimit  *BodyLimitConfig  `json:"bodyLimit,omitempty"`
	Proxy      *ProxyConfig      `json:"proxy,omitempty"`
	Connection *ConnectionConfig `json:"connection,omitempty"`
}

type RoutePolicyStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type TimeoutConfig struct {
	Request        *metav1.Duration `json:"request,omitempty"`
	BackendRequest *metav1.Duration `json:"backendRequest,omitempty"`
	Connect        *metav1.Duration `json:"connect,omitempty"`
	NextUpstream   *metav1.Duration `json:"nextUpstream,omitempty"`
}

type BodyLimitConfig struct {
	MaxRequestBodyBytes    *uint64 `json:"maxRequestBodyBytes,omitempty"`
	RequestBodyBufferBytes *uint64 `json:"requestBodyBufferBytes,omitempty"`
	MaxRequestHeaderBytes  *uint64 `json:"maxRequestHeaderBytes,omitempty"`
}

type ProxyConfig struct {
	RequestBuffering  *bool   `json:"requestBuffering,omitempty"`
	ResponseBuffering *bool   `json:"responseBuffering,omitempty"`
	BufferSize        *uint64 `json:"bufferSize,omitempty"`
	BufferCount       *uint32 `json:"bufferCount,omitempty"`
}

type ConnectionConfig struct {
	KeepaliveRequests         *uint32          `json:"keepaliveRequests,omitempty"`
	UpstreamKeepalivePoolSize *uint32          `json:"upstreamKeepalivePoolSize,omitempty"`
	KeepaliveTime             *metav1.Duration `json:"keepaliveTime,omitempty"`
	KeepaliveTimeout          *metav1.Duration `json:"keepaliveTimeout,omitempty"`
	UpstreamKeepaliveIdle     *metav1.Duration `json:"upstreamKeepaliveIdle,omitempty"`
}

func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &RoutePolicy{}, &RoutePolicyList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

func (in *RoutePolicy) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *RoutePolicy) DeepCopy() *RoutePolicy {
	if in == nil {
		return nil
	}
	out := new(RoutePolicy)
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.TargetRefs != nil {
		out.Spec.TargetRefs = make([]gatewayv1.LocalPolicyTargetReference, len(in.Spec.TargetRefs))
		copy(out.Spec.TargetRefs, in.Spec.TargetRefs)
	}
	if in.Spec.Default != nil {
		out.Spec.Default = new(RoutePolicyDefault)
		in.Spec.Default.DeepCopyInto(out.Spec.Default)
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
	return out
}

func (in *RoutePolicyList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	return in.DeepCopy()
}

func (in *RoutePolicyList) DeepCopy() *RoutePolicyList {
	if in == nil {
		return nil
	}
	out := new(RoutePolicyList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]RoutePolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

func (in *RoutePolicy) DeepCopyInto(out *RoutePolicy) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	if in.Spec.TargetRefs != nil {
		out.Spec.TargetRefs = make([]gatewayv1.LocalPolicyTargetReference, len(in.Spec.TargetRefs))
		copy(out.Spec.TargetRefs, in.Spec.TargetRefs)
	}
	if in.Spec.Default != nil {
		out.Spec.Default = new(RoutePolicyDefault)
		in.Spec.Default.DeepCopyInto(out.Spec.Default)
	}
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		copy(out.Status.Conditions, in.Status.Conditions)
	}
}

func (in *RoutePolicyDefault) DeepCopyInto(out *RoutePolicyDefault) {
	*out = *in
	if in.Timeout != nil {
		out.Timeout = new(TimeoutConfig)
		in.Timeout.DeepCopyInto(out.Timeout)
	}
	if in.BodyLimit != nil {
		out.BodyLimit = new(BodyLimitConfig)
		in.BodyLimit.DeepCopyInto(out.BodyLimit)
	}
	if in.Proxy != nil {
		out.Proxy = new(ProxyConfig)
		in.Proxy.DeepCopyInto(out.Proxy)
	}
	if in.Connection != nil {
		out.Connection = new(ConnectionConfig)
		in.Connection.DeepCopyInto(out.Connection)
	}
}

func (in *TimeoutConfig) DeepCopyInto(out *TimeoutConfig) {
	*out = *in
	if in.Request != nil {
		r := *in.Request
		out.Request = &r
	}
	if in.BackendRequest != nil {
		b := *in.BackendRequest
		out.BackendRequest = &b
	}
	if in.Connect != nil {
		c := *in.Connect
		out.Connect = &c
	}
	if in.NextUpstream != nil {
		n := *in.NextUpstream
		out.NextUpstream = &n
	}
}

func (in *BodyLimitConfig) DeepCopyInto(out *BodyLimitConfig) {
	*out = *in
	if in.MaxRequestBodyBytes != nil {
		v := *in.MaxRequestBodyBytes
		out.MaxRequestBodyBytes = &v
	}
	if in.RequestBodyBufferBytes != nil {
		v := *in.RequestBodyBufferBytes
		out.RequestBodyBufferBytes = &v
	}
	if in.MaxRequestHeaderBytes != nil {
		v := *in.MaxRequestHeaderBytes
		out.MaxRequestHeaderBytes = &v
	}
}

func (in *ProxyConfig) DeepCopyInto(out *ProxyConfig) {
	*out = *in
	if in.RequestBuffering != nil {
		v := *in.RequestBuffering
		out.RequestBuffering = &v
	}
	if in.ResponseBuffering != nil {
		v := *in.ResponseBuffering
		out.ResponseBuffering = &v
	}
	if in.BufferSize != nil {
		v := *in.BufferSize
		out.BufferSize = &v
	}
	if in.BufferCount != nil {
		v := *in.BufferCount
		out.BufferCount = &v
	}
}

func (in *ConnectionConfig) DeepCopyInto(out *ConnectionConfig) {
	*out = *in
	if in.KeepaliveRequests != nil {
		v := *in.KeepaliveRequests
		out.KeepaliveRequests = &v
	}
	if in.UpstreamKeepalivePoolSize != nil {
		v := *in.UpstreamKeepalivePoolSize
		out.UpstreamKeepalivePoolSize = &v
	}
	if in.KeepaliveTime != nil {
		v := *in.KeepaliveTime
		out.KeepaliveTime = &v
	}
	if in.KeepaliveTimeout != nil {
		v := *in.KeepaliveTimeout
		out.KeepaliveTimeout = &v
	}
	if in.UpstreamKeepaliveIdle != nil {
		v := *in.UpstreamKeepaliveIdle
		out.UpstreamKeepaliveIdle = &v
	}
}
