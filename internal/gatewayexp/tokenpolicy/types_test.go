package tokenpolicy

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

func TestTokenPolicyDeepCopy_FullConfig(t *testing.T) {
	policy := &TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "gpt4-limit", Namespace: "default"},
		Spec: TokenPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{
				{Group: "gateway.nantian.dev", Kind: "AIService", Name: "gpt4-svc"},
			},
			TokensPerMinute:   1000,
			TokensPerHour:     50000,
			RequestsPerMinute: 100,
			Scope:             "model",
			Burst:             2.0,
			OnLimit:           "block",
		},
	}
	copied := policy.DeepCopy()
	assert.Equal(t, uint64(1000), copied.Spec.TokensPerMinute)
	assert.Equal(t, "model", copied.Spec.Scope)
	assert.Equal(t, 1, len(copied.Spec.TargetRefs))
	assert.Equal(t, "gpt4-svc", string(copied.Spec.TargetRefs[0].Name))
	assert.NotSame(t, &policy.Spec.TargetRefs, &copied.Spec.TargetRefs)
}

func TestTokenPolicyDeepCopy_NoTargetRefs(t *testing.T) {
	policy := &TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "global-limit", Namespace: "default"},
		Spec: TokenPolicySpec{
			TokensPerMinute: 500,
		},
	}
	copied := policy.DeepCopy()
	assert.Nil(t, copied.Spec.TargetRefs)
	assert.Equal(t, uint64(500), copied.Spec.TokensPerMinute)
}

func TestTokenPolicyDeepCopy_Nil(t *testing.T) {
	var p *TokenPolicy
	assert.Nil(t, p.DeepCopy())
	assert.Nil(t, p.DeepCopyObject())
}

func TestTokenPolicyDeepCopyInto(t *testing.T) {
	src := &TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "src", Namespace: "ns"},
		Spec: TokenPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{
				{Group: "gateway.nantian.dev", Kind: "AIService", Name: "gpt4"},
			},
			TokensPerMinute: 2000,
			Scope:           "global",
			OnLimit:         "block",
		},
	}
	dst := &TokenPolicy{}
	src.DeepCopyInto(dst)
	assert.Equal(t, "src", dst.Name)
	assert.Equal(t, uint64(2000), dst.Spec.TokensPerMinute)
	assert.NotSame(t, &src.Spec.TargetRefs, &dst.Spec.TargetRefs)
}

func TestTokenPolicyListDeepCopy(t *testing.T) {
	list := &TokenPolicyList{
		Items: []TokenPolicy{
			{ObjectMeta: metav1.ObjectMeta{Name: "tp1", Namespace: "ns1"}, Spec: TokenPolicySpec{TokensPerMinute: 100}},
			{ObjectMeta: metav1.ObjectMeta{Name: "tp2", Namespace: "ns2"}, Spec: TokenPolicySpec{TokensPerMinute: 200, Scope: "model"}},
		},
	}
	copied := list.DeepCopy()
	assert.Equal(t, 2, len(copied.Items))
	assert.Equal(t, "tp1", copied.Items[0].Name)
	assert.Equal(t, uint64(200), copied.Items[1].Spec.TokensPerMinute)
}

func TestTokenPolicyListDeepCopy_Nil(t *testing.T) {
	var list *TokenPolicyList
	assert.Nil(t, list.DeepCopy())
	assert.Nil(t, list.DeepCopyObject())
}

func TestTokenPolicySpecDeepCopy(t *testing.T) {
	spec := &TokenPolicySpec{
		TargetRefs: []gatewayv1.LocalPolicyTargetReference{
			{Group: "gateway.nantian.dev", Kind: "AIService", Name: "svc1"},
		},
		TokensPerMinute:   500,
		TokensPerHour:     10000,
		RequestsPerMinute: 50,
		Scope:             "user",
		Burst:             1.5,
		OnLimit:           "block",
	}
	copied := spec.DeepCopy()
	assert.Equal(t, spec.TokensPerMinute, copied.TokensPerMinute)
	assert.Equal(t, spec.TokensPerHour, copied.TokensPerHour)
	assert.NotSame(t, &spec.TargetRefs, &copied.TargetRefs)
}

func TestTokenPolicySpecDeepCopy_Nil(t *testing.T) {
	var spec *TokenPolicySpec
	assert.Nil(t, spec.DeepCopy())
}

func TestTokenPolicyStatusDeepCopy(t *testing.T) {
	status := &TokenPolicyStatus{
		Conditions: []metav1.Condition{
			{Type: "Accepted", Status: "True", Reason: "Valid", Message: "ok"},
		},
	}
	copied := status.DeepCopy()
	assert.Equal(t, 1, len(copied.Conditions))
	assert.Equal(t, "Accepted", copied.Conditions[0].Type)
	assert.NotSame(t, &status.Conditions, &copied.Conditions)
}

func TestTokenPolicyStatusDeepCopy_Nil(t *testing.T) {
	var status *TokenPolicyStatus
	assert.Nil(t, status.DeepCopy())
}