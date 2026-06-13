package gatewayapi

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDefaultGatewayScopeHelpers(t *testing.T) {
	if UsesDefaultGateways("") {
		t.Fatal("empty default scope should not use default gateways")
	}
	if UsesDefaultGateways(gatewayv1.GatewayDefaultScopeNone) {
		t.Fatal("None default scope should not use default gateways")
	}
	if !UsesDefaultGateways(gatewayv1.GatewayDefaultScopeAll) {
		t.Fatal("All default scope should use default gateways")
	}

	gateway := gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{DefaultScope: gatewayv1.GatewayDefaultScopeAll},
	}
	if !GatewayActsAsDefault(gateway) {
		t.Fatal("gateway with All default scope should act as default")
	}
	if !GatewayMatchesDefaultScope(gateway, gatewayv1.GatewayDefaultScopeAll) {
		t.Fatal("gateway should match its default scope")
	}
	if GatewayMatchesDefaultScope(gateway, gatewayv1.GatewayDefaultScopeNone) {
		t.Fatal("gateway should not match disabled default scope")
	}
}

func TestDefaultGatewayParentRefsAppendsMatchingGatewaysAndDeduplicates(t *testing.T) {
	group := gatewayv1.Group(gatewayv1.GroupName)
	kind := gatewayv1.Kind("Gateway")
	existing := gatewayv1.ParentReference{
		Group: &group,
		Kind:  &kind,
		Name:  "same-namespace",
	}

	got := DefaultGatewayParentRefs(
		[]gatewayv1.ParentReference{existing},
		"apps",
		gatewayv1.GatewayDefaultScopeAll,
		[]gatewayv1.Gateway{
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "same-namespace"},
				Spec:       gatewayv1.GatewaySpec{DefaultScope: gatewayv1.GatewayDefaultScopeAll},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "infra", Name: "shared"},
				Spec:       gatewayv1.GatewaySpec{DefaultScope: gatewayv1.GatewayDefaultScopeAll},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "disabled"},
				Spec:       gatewayv1.GatewaySpec{DefaultScope: gatewayv1.GatewayDefaultScopeNone},
			},
		},
	)

	if len(got) != 2 {
		t.Fatalf("DefaultGatewayParentRefs() returned %d refs, want 2: %#v", len(got), got)
	}
	if !reflect.DeepEqual(got[0], existing) {
		t.Fatalf("first parent ref changed: %#v, want %#v", got[0], existing)
	}
	if got[1].Namespace == nil || *got[1].Namespace != "infra" {
		t.Fatalf("cross-namespace default gateway ref namespace = %#v, want infra", got[1].Namespace)
	}
	if string(got[1].Name) != "shared" {
		t.Fatalf("cross-namespace default gateway ref name = %q, want shared", got[1].Name)
	}
}

func TestDefaultGatewayParentRefsReturnsCopyWhenScopeDisabled(t *testing.T) {
	ref := gatewayv1.ParentReference{Name: "edge"}
	got := DefaultGatewayParentRefs(
		[]gatewayv1.ParentReference{ref},
		"apps",
		gatewayv1.GatewayDefaultScopeNone,
		[]gatewayv1.Gateway{{
			ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "default"},
			Spec:       gatewayv1.GatewaySpec{DefaultScope: gatewayv1.GatewayDefaultScopeAll},
		}},
	)

	if len(got) != 1 || string(got[0].Name) != "edge" {
		t.Fatalf("DefaultGatewayParentRefs() = %#v, want only original ref", got)
	}
	got[0].Name = "mutated"
	if ref.Name != "edge" {
		t.Fatal("DefaultGatewayParentRefs() should return a copy of parent refs")
	}
}
