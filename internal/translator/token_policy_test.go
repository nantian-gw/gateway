package translator

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
)

func TestTranslateTokenPolicy_Basic(t *testing.T) {
	policy := tokenpolicy.TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orders-rate-limit",
			Namespace: "default",
		},
		Spec: tokenpolicy.TokenPolicySpec{
			TokensPerMinute:   1000,
			TokensPerHour:     10000,
			RequestsPerMinute: 60,
			Scope:             "per-user",
			Burst:             1.5,
			OnLimit:           "deny",
		},
	}

	cfg := translateTokenPolicy(policy)

	if cfg.TokensPerMinute != 1000 {
		t.Fatalf("expected TokensPerMinute=1000, got %d", cfg.TokensPerMinute)
	}
	if cfg.TokensPerHour != 10000 {
		t.Fatalf("expected TokensPerHour=10000, got %d", cfg.TokensPerHour)
	}
	if cfg.RequestsPerMinute != 60 {
		t.Fatalf("expected RequestsPerMinute=60, got %d", cfg.RequestsPerMinute)
	}
	if cfg.Scope != "per-user" {
		t.Fatalf("expected Scope=per-user, got %q", cfg.Scope)
	}
	if cfg.Burst != 1.5 {
		t.Fatalf("expected Burst=1.5, got %f", cfg.Burst)
	}
	if cfg.OnLimit != "deny" {
		t.Fatalf("expected OnLimit=deny, got %q", cfg.OnLimit)
	}
}

func TestTranslateTokenPolicy_Empty(t *testing.T) {
	policy := tokenpolicy.TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-policy",
			Namespace: "default",
		},
	}

	cfg := translateTokenPolicy(policy)

	if cfg.TokensPerMinute != 0 {
		t.Fatalf("expected TokensPerMinute=0, got %d", cfg.TokensPerMinute)
	}
	if cfg.TokensPerHour != 0 {
		t.Fatalf("expected TokensPerHour=0, got %d", cfg.TokensPerHour)
	}
	if cfg.RequestsPerMinute != 0 {
		t.Fatalf("expected RequestsPerMinute=0, got %d", cfg.RequestsPerMinute)
	}
	if cfg.Scope != "" {
		t.Fatalf("expected Scope empty, got %q", cfg.Scope)
	}
	if cfg.Burst != 0 {
		t.Fatalf("expected Burst=0, got %f", cfg.Burst)
	}
	if cfg.OnLimit != "" {
		t.Fatalf("expected OnLimit empty, got %q", cfg.OnLimit)
	}
}

func TestTranslateTokenPolicyList(t *testing.T) {
	policies := []tokenpolicy.TokenPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-a-limit",
				Namespace: "ns1",
			},
			Spec: tokenpolicy.TokenPolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Group: "", Kind: "Service", Name: "svc-a"},
				},
				TokensPerMinute: 500,
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc-b-limit",
				Namespace: "ns1",
			},
			Spec: tokenpolicy.TokenPolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Group: "", Kind: "Service", Name: "svc-b"},
				},
				TokensPerMinute: 200,
			},
		},
	}

	services := map[string]struct{}{
		"ns1/svc-a": {},
		"ns1/svc-b": {},
	}

	result := translateTokenPolicies(policies, services, nil, nil)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if cfg, ok := result["ns1/svc-a"]; !ok {
		t.Fatal("expected result for svc-a")
	} else if cfg.TokensPerMinute != 500 {
		t.Fatalf("unexpected TokensPerMinute for svc-a: %d", cfg.TokensPerMinute)
	}
	if cfg, ok := result["ns1/svc-b"]; !ok {
		t.Fatal("expected result for svc-b")
	} else if cfg.TokensPerMinute != 200 {
		t.Fatalf("unexpected TokensPerMinute for svc-b: %d", cfg.TokensPerMinute)
	}
}

func TestTranslateTokenPolicies_HTTPRouteTargetRef(t *testing.T) {
	policies := []tokenpolicy.TokenPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-limit",
				Namespace: "ns1",
			},
			Spec: tokenpolicy.TokenPolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Name: "route-a"},
				},
				TokensPerMinute: 1000,
			},
		},
	}

	services := map[string]struct{}{
		"ns1/svc-a": {},
		"ns1/svc-b": {},
	}

	httpRoutes := map[string][]string{
		"ns1/route-a": {"ns1/svc-a", "ns1/svc-b"},
	}

	result := translateTokenPolicies(policies, services, nil, httpRoutes)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if cfg, ok := result["ns1/svc-a"]; !ok {
		t.Fatal("expected result for svc-a")
	} else if cfg.TokensPerMinute != 1000 {
		t.Fatalf("unexpected TokensPerMinute for svc-a: %d", cfg.TokensPerMinute)
	}
	if cfg, ok := result["ns1/svc-b"]; !ok {
		t.Fatal("expected result for svc-b")
	} else if cfg.TokensPerMinute != 1000 {
		t.Fatalf("unexpected TokensPerMinute for svc-b: %d", cfg.TokensPerMinute)
	}
}

func TestTranslateTokenPolicies_HTTPRouteNoBackendMatch(t *testing.T) {
	policies := []tokenpolicy.TokenPolicy{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "missing-route-limit",
				Namespace: "ns1",
			},
			Spec: tokenpolicy.TokenPolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReference{
					{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Name: "route-missing"},
				},
				TokensPerMinute: 500,
			},
		},
	}

	services := map[string]struct{}{
		"ns1/svc-a": {},
	}

	httpRoutes := map[string][]string{
		"ns1/route-a": {"ns1/svc-a"},
	}

	result := translateTokenPolicies(policies, services, nil, httpRoutes)

	if len(result) != 0 {
		t.Fatalf("expected 0 results (route not found), got %d", len(result))
	}
}

func TestBuildRouteBackendServices(t *testing.T) {
	nsOther := gatewayv1.Namespace("other-ns")
	grpCore := gatewayv1.Group("")
	kindSvc := gatewayv1.Kind("Service")

	routes := []gatewayv1.HTTPRoute{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "route-a",
				Namespace: "ns1",
			},
			Spec: gatewayv1.HTTPRouteSpec{
				Rules: []gatewayv1.HTTPRouteRule{
					{
						BackendRefs: []gatewayv1.HTTPBackendRef{
							{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Name:  "svc-a",
										Group: &grpCore,
										Kind:  &kindSvc,
									},
								},
							},
							{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Name:      "svc-b",
										Namespace: &nsOther,
										Group:     &grpCore,
										Kind:      &kindSvc,
									},
								},
							},
						},
					},
					{
						BackendRefs: []gatewayv1.HTTPBackendRef{
							{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Name:  "svc-c",
										Group: &grpCore,
										Kind:  &kindSvc,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	result := buildRouteBackendServices(routes)

	if len(result) != 1 {
		t.Fatalf("expected 1 route, got %d", len(result))
	}
	backends, ok := result["ns1/route-a"]
	if !ok {
		t.Fatal("expected route ns1/route-a")
	}
	if len(backends) != 3 {
		t.Fatalf("expected 3 backends, got %d", len(backends))
	}
	expected := []string{"ns1/svc-a", "other-ns/svc-b", "ns1/svc-c"}
	for i, exp := range expected {
		if backends[i] != exp {
			t.Fatalf("backend[%d]: expected %q, got %q", i, exp, backends[i])
		}
	}
}