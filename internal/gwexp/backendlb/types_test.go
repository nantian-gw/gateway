package backendlb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeepCopyRoundtrip(t *testing.T) {
	lbType := LoadBalancingStrategyTypeRoundRobin
	original := &BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-lb",
			Namespace: "default",
		},
		Spec: BackendLBPolicySpec{
			LoadBalancing: &LoadBalancingPolicy{
				Type: &lbType,
			},
		},
	}
	copied := original.DeepCopy()
	assert.Equal(t, original, copied)
	assert.NotSame(t, original, copied)
}

func TestBackendLBPolicyWithCircuitBreaker(t *testing.T) {
	maxInflight := int32(100)
	policy := &BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cb-policy",
			Namespace: "default",
		},
		Spec: BackendLBPolicySpec{
			TargetRefs: []LocalPolicyTargetReference{
				{Group: "", Kind: "Service", Name: "backend-svc"},
			},
			CircuitBreaker: &CircuitBreakerConfig{
				MaxInflightRequests: &maxInflight,
			},
		},
	}

	assert.Equal(t, int32(100), *policy.Spec.CircuitBreaker.MaxInflightRequests)
	assert.Len(t, policy.Spec.TargetRefs, 1)
	assert.Equal(t, "backend-svc", string(policy.Spec.TargetRefs[0].Name))

	copied := policy.DeepCopy()
	assert.Equal(t, policy.Spec.CircuitBreaker.MaxInflightRequests, copied.Spec.CircuitBreaker.MaxInflightRequests)
}

func TestBackendLBPolicyWithAllFeatures(t *testing.T) {
	maxInflight := int32(50)
	lbType := LoadBalancingStrategyTypeConsistentHash
	hashKeyType := HashKeyTypeHeader
	headerName := "x-tenant-id"

	policy := &BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "full-policy",
			Namespace: "default",
		},
		Spec: BackendLBPolicySpec{
			TargetRefs: []LocalPolicyTargetReference{
				{Group: "", Kind: "Service", Name: "api-svc"},
			},
			LoadBalancing: &LoadBalancingPolicy{
				Type: &lbType,
				ConsistentHash: &ConsistentHashPolicy{
					KeyType:    &hashKeyType,
					HeaderName: &headerName,
				},
			},
			CircuitBreaker: &CircuitBreakerConfig{
				MaxInflightRequests: &maxInflight,
			},
		},
	}

	assert.NotNil(t, policy.Spec.LoadBalancing)
	assert.Equal(t, LoadBalancingStrategyTypeConsistentHash, *policy.Spec.LoadBalancing.Type)
	assert.NotNil(t, policy.Spec.CircuitBreaker)
	assert.Equal(t, int32(50), *policy.Spec.CircuitBreaker.MaxInflightRequests)

	copied := policy.DeepCopy()
	assert.Equal(t, policy, copied)
}

func TestCircuitBreakerConfig_NilMaxInflight(t *testing.T) {
	cfg := &CircuitBreakerConfig{}
	assert.Nil(t, cfg.MaxInflightRequests)
}