package tokenpolicyv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDeepCopyRoundtrip(t *testing.T) {
	original := &TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-policy",
			Namespace: "default",
		},
		Spec: TokenPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{
				{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Name: "my-route"},
			},
			TokensPerMinute:   1000,
			RequestsPerMinute: 100,
			Scope:             "per-user",
			Burst:             1.5,
		},
	}
	copied := original.DeepCopy()
	assert.Equal(t, original, copied)
	assert.NotSame(t, original, copied)
}