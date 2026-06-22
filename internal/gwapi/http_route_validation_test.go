package gwapi

import (
	"reflect"
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestValidateHTTPRouteRulesAcceptsHTTPExternalAuthFilter(t *testing.T) {
	route := gatewayv1.HTTPRoute{
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{{
				Filters: []gatewayv1.HTTPRouteFilter{{
					Type: gatewayv1.HTTPRouteFilterExternalAuth,
					ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
						ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthHTTPProtocol,
						BackendRef: gatewayv1.BackendObjectReference{
							Name: "auth",
							Port: portNumberPtr(9000),
						},
					},
				}},
			}},
		},
	}

	summary := ValidateHTTPRouteRules(route)
	if len(summary.InvalidRuleIndexes) != 0 {
		t.Fatalf("InvalidRuleIndexes = %#v, want none", summary.InvalidRuleIndexes)
	}
	if summary.FullyInvalid(1) {
		t.Fatalf("FullyInvalid(1) = true, want false")
	}
	if message := summary.InvalidRuleMessage(); message != "" {
		t.Fatalf("InvalidRuleMessage() = %q, want empty", message)
	}
}

func TestValidateHTTPRouteRulesRejectsGRPCExternalAuthFilter(t *testing.T) {
	route := gatewayv1.HTTPRoute{
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{{
				Filters: []gatewayv1.HTTPRouteFilter{{
					Type: gatewayv1.HTTPRouteFilterExternalAuth,
					ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
						ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthGRPCProtocol,
						BackendRef: gatewayv1.BackendObjectReference{
							Name: "auth",
							Port: portNumberPtr(9000),
						},
						GRPCAuthConfig: &gatewayv1.GRPCAuthConfig{
							AllowedRequestHeaders: []string{"authorization", "x-tenant"},
						},
					},
				}},
			}},
		},
	}

	summary := ValidateHTTPRouteRules(route)
	if len(summary.InvalidRuleIndexes) != 1 {
		t.Fatalf("InvalidRuleIndexes = %#v, want [0] (GRPC ExtAuth should be rejected, only HTTP supported)", summary.InvalidRuleIndexes)
	}
	if summary.InvalidRuleIndexes[0] != 0 {
		t.Fatalf("InvalidRuleIndexes[0] = %d, want 0", summary.InvalidRuleIndexes[0])
	}
	reason := externalAuthFilterRejectionReason(route.Spec.Rules[0].Filters[0].ExternalAuth)
	if reason != "GRPC ExtAuth protocol is not yet supported (only HTTP ExtAuth)" {
		t.Fatalf("rejection reason = %q, want GRPC rejection", reason)
	}
}

func TestValidateHTTPRouteRulesRejectsExternalAuthForwardBody(t *testing.T) {
	for _, tt := range []struct {
		name   string
		filter gatewayv1.HTTPRouteFilter
	}{
		{
			name: "http forward body",
			filter: gatewayv1.HTTPRouteFilter{
				Type: gatewayv1.HTTPRouteFilterExternalAuth,
				ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
					ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthHTTPProtocol,
					BackendRef: gatewayv1.BackendObjectReference{
						Name: "auth",
						Port: portNumberPtr(9000),
					},
					ForwardBody: &gatewayv1.ForwardBodyConfig{MaxSize: 1024},
				},
			},
		},
		{
			name: "grpc forward body",
			filter: gatewayv1.HTTPRouteFilter{
				Type: gatewayv1.HTTPRouteFilterExternalAuth,
				ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
					ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthGRPCProtocol,
					BackendRef: gatewayv1.BackendObjectReference{
						Name: "auth",
						Port: portNumberPtr(9000),
					},
					ForwardBody: &gatewayv1.ForwardBodyConfig{MaxSize: 1024},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			route := gatewayv1.HTTPRoute{
				Spec: gatewayv1.HTTPRouteSpec{
					Rules: []gatewayv1.HTTPRouteRule{{Filters: []gatewayv1.HTTPRouteFilter{tt.filter}}},
				},
			}
			summary := ValidateHTTPRouteRules(route)
			if len(summary.InvalidRuleIndexes) != 1 {
				t.Fatalf("InvalidRuleIndexes = %#v, want [0] (forwardBody should be rejected)", summary.InvalidRuleIndexes)
			}
		})
	}
}

func TestValidateHTTPRouteRulesAcceptsBackendExternalAuthFilter(t *testing.T) {
	route := gatewayv1.HTTPRoute{
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{{
				BackendRefs: []gatewayv1.HTTPBackendRef{{
					BackendRef: gatewayv1.BackendRef{
						BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: "backend",
							Port: portNumberPtr(8080),
						},
					},
					Filters: []gatewayv1.HTTPRouteFilter{{
						Type: gatewayv1.HTTPRouteFilterExternalAuth,
						ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
							ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthHTTPProtocol,
							BackendRef: gatewayv1.BackendObjectReference{
								Name: "auth",
								Port: portNumberPtr(9000),
							},
						},
					}},
				}},
			}},
		},
	}

	summary := ValidateHTTPRouteRules(route)
	if len(summary.InvalidRuleIndexes) != 0 {
		t.Fatalf("InvalidRuleIndexes = %#v, want none", summary.InvalidRuleIndexes)
	}
	if message := summary.InvalidRuleMessage(); message != "" {
		t.Fatalf("InvalidRuleMessage() = %q, want empty", message)
	}
}

func TestValidateHTTPRouteRulesBuildsPartialInvalidMessages(t *testing.T) {
	route := gatewayv1.HTTPRoute{
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Filters: []gatewayv1.HTTPRouteFilter{{
						Type: gatewayv1.HTTPRouteFilterType("Unsupported"),
					}},
				},
				{
					Filters: []gatewayv1.HTTPRouteFilter{
						{Type: gatewayv1.HTTPRouteFilterRequestRedirect},
						{Type: gatewayv1.HTTPRouteFilterURLRewrite},
					},
				},
				{},
			},
		},
	}

	summary := ValidateHTTPRouteRules(route)
	if !reflect.DeepEqual(summary.InvalidRuleIndexes, []int{0, 1}) {
		t.Fatalf("InvalidRuleIndexes = %#v, want [0 1]", summary.InvalidRuleIndexes)
	}
	if summary.FullyInvalid(len(route.Spec.Rules)) {
		t.Fatal("FullyInvalid() = true, want false for partially invalid route")
	}
	if !summary.PartiallyInvalid(len(route.Spec.Rules)) {
		t.Fatal("PartiallyInvalid() = false, want true")
	}

	wantInvalid := "HTTPRoute rules 1, 2 are invalid: rule 1 uses unsupported Unsupported filter; rule 2 must not combine RequestRedirect and URLRewrite filters"
	if got := summary.InvalidRuleMessage(); got != wantInvalid {
		t.Fatalf("InvalidRuleMessage() = %q, want %q", got, wantInvalid)
	}

	wantAccepted := "HTTPRoute rule 1 uses unsupported Unsupported filter"
	if got := summary.AcceptedErrorMessage(); got != wantAccepted {
		t.Fatalf("AcceptedErrorMessage() = %q, want %q", got, wantAccepted)
	}

	wantDropped := "Dropped Rules 1, 2 because " + wantInvalid
	if got := summary.DroppedRulesMessage(); got != wantDropped {
		t.Fatalf("DroppedRulesMessage() = %q, want %q", got, wantDropped)
	}
}

func TestHTTPRouteRuleMessageCompactsRepeatedMessages(t *testing.T) {
	summary := HTTPRouteRuleValidationSummary{
		InvalidRuleIndexes:  []int{0, 2},
		invalidRuleMessages: []string{"uses unsupported Foo filter", "uses unsupported Foo filter"},
	}

	want := "HTTPRoute rules 1, 3 uses unsupported Foo filter"
	if got := summary.InvalidRuleMessage(); got != want {
		t.Fatalf("InvalidRuleMessage() = %q, want %q", got, want)
	}
}

func portNumberPtr(port int32) *gatewayv1.PortNumber {
	value := gatewayv1.PortNumber(port)
	return &value
}
