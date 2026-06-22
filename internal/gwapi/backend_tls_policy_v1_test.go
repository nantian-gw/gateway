package gwapi

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

func TestBackendTLSPolicyV1ObjectConstructorsSetGVK(t *testing.T) {
	if got := NewBackendTLSPolicyV1Object().GroupVersionKind(); got != BackendTLSPolicyV1GVK {
		t.Fatalf("NewBackendTLSPolicyV1Object() GVK = %s, want %s", got, BackendTLSPolicyV1GVK)
	}
	wantListGVK := BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList")
	if got := NewBackendTLSPolicyV1List().GroupVersionKind(); got != wantListGVK {
		t.Fatalf("NewBackendTLSPolicyV1List() GVK = %s, want %s", got, wantListGVK)
	}
}

func TestEncodeDecodeBackendTLSPolicyV1RoundTripsTypedPolicy(t *testing.T) {
	systemCA := gatewayv1.WellKnownCACertificatesSystem
	policy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "apps"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Kind: "Service",
					Name: "orders",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "orders.internal.example",
				WellKnownCACertificates: &systemCA,
			},
		},
	}

	raw, err := EncodeBackendTLSPolicyV1(policy)
	if err != nil {
		t.Fatalf("EncodeBackendTLSPolicyV1() returned error: %v", err)
	}
	if got := raw.GroupVersionKind(); got != BackendTLSPolicyV1GVK {
		t.Fatalf("encoded policy GVK = %s, want %s", got, BackendTLSPolicyV1GVK)
	}

	decoded, err := DecodeBackendTLSPolicyV1(raw)
	if err != nil {
		t.Fatalf("DecodeBackendTLSPolicyV1() returned error: %v", err)
	}
	if decoded.Namespace != "apps" || decoded.Name != "orders-tls" {
		t.Fatalf("decoded identity = %s/%s, want apps/orders-tls", decoded.Namespace, decoded.Name)
	}
	if decoded.Spec.Validation.Hostname != "orders.internal.example" {
		t.Fatalf("decoded hostname = %q, want orders.internal.example", decoded.Spec.Validation.Hostname)
	}
}

func TestListAndGetBackendTLSPolicyV1FallbackToTypedObjectsForFakeClient(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := gatewayv1alpha3.Install(scheme); err != nil {
		t.Fatalf("install gateway v1alpha3 scheme: %v", err)
	}

	policy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "apps"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Kind: "Service",
					Name: "orders",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{Hostname: "orders.internal.example"},
		},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(policy).Build()

	items, err := ListBackendTLSPoliciesV1WithOptions(ctx, cl, client.InNamespace("apps"))
	if err != nil {
		t.Fatalf("ListBackendTLSPoliciesV1WithOptions() returned error: %v", err)
	}
	if len(items) != 1 || items[0].Name != "orders-tls" {
		t.Fatalf("ListBackendTLSPoliciesV1WithOptions() = %#v, want orders-tls", items)
	}

	raw, typed, err := GetBackendTLSPolicyV1(ctx, cl, client.ObjectKey{Namespace: "apps", Name: "orders-tls"})
	if err != nil {
		t.Fatalf("GetBackendTLSPolicyV1() returned error: %v", err)
	}
	if raw == nil || raw.GroupVersionKind() != BackendTLSPolicyV1GVK {
		t.Fatalf("GetBackendTLSPolicyV1() raw GVK = %#v, want %s", raw, BackendTLSPolicyV1GVK)
	}
	if typed.Name != "orders-tls" || typed.Spec.Validation.Hostname != "orders.internal.example" {
		t.Fatalf("GetBackendTLSPolicyV1() typed = %#v, want orders-tls with hostname", typed)
	}
}
