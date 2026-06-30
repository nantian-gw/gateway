package routepolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDeepCopyObject_ReturnsNonNilNonIdenticalCopy(t *testing.T) {
	timeout := metav1.Duration{Duration: 30_000_000_000}
	maxBody := uint64(1048576)
	policy := &RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: RoutePolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{
				{Group: "", Kind: "HTTPRoute", Name: "my-route"},
			},
			Default: &RoutePolicyDefault{
				Timeout: &TimeoutConfig{
					Request: &timeout,
				},
				BodyLimit: &BodyLimitConfig{
					MaxRequestBodyBytes: &maxBody,
				},
			},
		},
	}

	result := policy.DeepCopyObject()
	assert.NotNil(t, result)
	copied, ok := result.(*RoutePolicy)
	assert.True(t, ok)
	assert.Equal(t, policy, copied)
	assert.NotSame(t, policy, copied)
	assert.NotSame(t, policy.Spec.Default, copied.Spec.Default)
	assert.NotSame(t, policy.Spec.Default.Timeout, copied.Spec.Default.Timeout)
	assert.NotSame(t, policy.Spec.Default.BodyLimit, copied.Spec.Default.BodyLimit)
}

func TestDeepCopy_NilReturnsNil(t *testing.T) {
	var policy *RoutePolicy
	assert.Nil(t, policy.DeepCopy())
	assert.Nil(t, policy.DeepCopyObject())

	var list *RoutePolicyList
	assert.Nil(t, list.DeepCopy())
	assert.Nil(t, list.DeepCopyObject())
}

func TestRoutePolicyListDeepCopy(t *testing.T) {
	list := &RoutePolicyList{
		Items: []RoutePolicy{
			{ObjectMeta: metav1.ObjectMeta{Name: "rp1", Namespace: "ns1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "rp2", Namespace: "ns2"}},
		},
	}
	copied := list.DeepCopy()
	assert.Equal(t, 2, len(copied.Items))
	assert.Equal(t, "rp1", copied.Items[0].Name)
	assert.NotSame(t, list, copied)
	assert.NotSame(t, &list.Items[0], &copied.Items[0])
}

func TestAddToScheme_RegistersTypes(t *testing.T) {
	scheme := runtime.NewScheme()
	err := AddToScheme(scheme)
	assert.NoError(t, err)
	assert.True(t, scheme.Recognizes(GroupVersion.WithKind("RoutePolicy")))
	assert.True(t, scheme.Recognizes(GroupVersion.WithKind("RoutePolicyList")))
}

func TestDeepCopyWithAllDefaults(t *testing.T) {
	requestDur := metav1.Duration{Duration: 10_000_000_000}
	backendDur := metav1.Duration{Duration: 20_000_000_000}
	connectDur := metav1.Duration{Duration: 30_000_000_000}
	nextDur := metav1.Duration{Duration: 40_000_000_000}
	maxBody := uint64(1048576)
	bufBytes := uint64(65536)
	hdrBytes := uint64(16384)
	reqBuf := true
	respBuf := false
	bufSize := uint64(8192)
	bufCount := uint32(16)
	keepReqs := uint32(1000)
	poolSize := uint32(32)
	keepTime := metav1.Duration{Duration: 60_000_000_000}
	keepTimeout := metav1.Duration{Duration: 30_000_000_000}
	keepIdle := metav1.Duration{Duration: 120_000_000_000}

	policy := &RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "full-policy",
			Namespace: "default",
		},
		Spec: RoutePolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{
				{Group: "", Kind: "HTTPRoute", Name: "api-route"},
			},
			Default: &RoutePolicyDefault{
				Timeout: &TimeoutConfig{
					Request:        &requestDur,
					BackendRequest: &backendDur,
					Connect:        &connectDur,
					NextUpstream:   &nextDur,
				},
				BodyLimit: &BodyLimitConfig{
					MaxRequestBodyBytes:    &maxBody,
					RequestBodyBufferBytes: &bufBytes,
					MaxRequestHeaderBytes:  &hdrBytes,
				},
				Proxy: &ProxyConfig{
					RequestBuffering:  &reqBuf,
					ResponseBuffering: &respBuf,
					BufferSize:        &bufSize,
					BufferCount:       &bufCount,
				},
				Connection: &ConnectionConfig{
					KeepaliveRequests:         &keepReqs,
					UpstreamKeepalivePoolSize: &poolSize,
					KeepaliveTime:             &keepTime,
					KeepaliveTimeout:          &keepTimeout,
					UpstreamKeepaliveIdle:     &keepIdle,
				},
			},
		},
		Status: RoutePolicyStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "Accepted",
					Status: metav1.ConditionTrue,
					Reason: "Accepted",
				},
			},
		},
	}

	copied := policy.DeepCopy()
	assert.Equal(t, policy, copied)
	assert.NotSame(t, policy, copied)
	assert.NotSame(t, policy.Spec.Default, copied.Spec.Default)
	assert.NotSame(t, policy.Spec.Default.Timeout, copied.Spec.Default.Timeout)
	assert.NotSame(t, policy.Spec.Default.BodyLimit, copied.Spec.Default.BodyLimit)
	assert.NotSame(t, policy.Spec.Default.Proxy, copied.Spec.Default.Proxy)
	assert.NotSame(t, policy.Spec.Default.Connection, copied.Spec.Default.Connection)
	assert.NotSame(t, &policy.Status.Conditions[0], &copied.Status.Conditions[0])
}

func TestRoutePolicyListDeepCopyObject(t *testing.T) {
	list := &RoutePolicyList{
		Items: []RoutePolicy{
			{ObjectMeta: metav1.ObjectMeta{Name: "rp1", Namespace: "ns1"}},
		},
	}
	result := list.DeepCopyObject()
	assert.NotNil(t, result)
	copied, ok := result.(*RoutePolicyList)
	assert.True(t, ok)
	assert.Equal(t, len(list.Items), len(copied.Items))
	assert.NotSame(t, list, copied)
}
