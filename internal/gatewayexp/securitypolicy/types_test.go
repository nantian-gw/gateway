package securitypolicy

import (
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestSecurityPolicyDeepCopy(t *testing.T) {
	p := &SecurityPolicy{
		Spec: SecurityPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{{Name: "gw"}},
			AuthN: &SecurityAuthNConfig{
				OIDC: &OIDCConfig{ProviderAuthorizationURL: "https://idp", ClientID: "cid"},
			},
			RateLimit: []RateLimitRule{{Scope: "global", RequestsPerSecond: 100}},
		},
	}
	cp := p.DeepCopy()
	cp.Spec.AuthN.OIDC.ProviderAuthorizationURL = "changed"
	cp.Spec.RateLimit[0].Scope = "route"
	if p.Spec.AuthN.OIDC.ProviderAuthorizationURL != "https://idp" { t.Fatal("DeepCopy shares pointer") }
	if p.Spec.RateLimit[0].Scope != "global" { t.Fatal("DeepCopy shares slice") }
}
