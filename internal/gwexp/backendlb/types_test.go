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