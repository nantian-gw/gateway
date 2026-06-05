package status

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
)

func TestReconcileAppliesBackendTLSPolicyPrecedence(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	policy := func(name string) *gatewayv1alpha3.BackendTLSPolicy {
		return &gatewayv1alpha3.BackendTLSPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Generation: 1},
			Spec: gatewayv1.BackendTLSPolicySpec{
				TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
					LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
						Group: "",
						Kind:  "Service",
						Name:  "orders",
					},
				}},
				Validation: gatewayv1.BackendTLSPolicyValidation{
					Hostname:                "orders.internal.example",
					WellKnownCACertificates: &systemCA,
				},
			},
		}
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			policy("orders-tls-a"),
			policy("orders-tls-b"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	for _, tc := range []struct {
		name           string
		acceptedStatus metav1.ConditionStatus
		acceptedReason string
	}{
		{
			name:           "orders-tls-a",
			acceptedStatus: metav1.ConditionTrue,
			acceptedReason: string(gatewayv1.PolicyReasonAccepted),
		},
		{
			name:           "orders-tls-b",
			acceptedStatus: metav1.ConditionFalse,
			acceptedReason: string(gatewayv1.PolicyReasonConflicted),
		},
	} {
		var item gatewayv1alpha3.BackendTLSPolicy
		if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: tc.name}, &item); err != nil {
			t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
		}
		if len(item.Status.Ancestors) != 1 {
			t.Fatalf("expected 1 ancestor for %s, got %d", tc.name, len(item.Status.Ancestors))
		}
		assertCondition(
			t,
			item.Status.Ancestors[0].Conditions,
			string(gatewayv1.PolicyConditionAccepted),
			tc.acceptedStatus,
			tc.acceptedReason,
			1,
		)
		assertCondition(
			t,
			item.Status.Ancestors[0].Conditions,
			backendTLSPolicyConditionResolvedRefs,
			metav1.ConditionTrue,
			backendTLSPolicyReasonResolvedRefs,
			1,
		)
		switch tc.name {
		case "orders-tls-a":
			assertConditionMessage(t, item.Status.Ancestors[0].Conditions, string(gatewayv1.PolicyConditionAccepted), "BackendTLSPolicy is accepted by nantian-gw")
		case "orders-tls-b":
			assertConditionMessage(t, item.Status.Ancestors[0].Conditions, string(gatewayv1.PolicyConditionAccepted), "BackendTLSPolicy conflicts with another policy targeting the same backend")
		}
		assertConditionMessage(t, item.Status.Ancestors[0].Conditions, backendTLSPolicyConditionResolvedRefs, "BackendTLSPolicy references are resolved")
	}
}

func TestReconcileAcceptsSectionScopedAndCatchAllBackendTLSPolicies(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	systemCA := gatewayv1.WellKnownCACertificatesSystem
	https1 := gatewayv1.SectionName("https-1")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{Name: "https-1", Port: 443},
						{Name: "https-2", Port: 8443},
					},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "orders-tls-section",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
						SectionName: &https1,
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "section.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "orders-tls-all",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "all.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	for _, name := range []string{"orders-tls-section", "orders-tls-all"} {
		var item gatewayv1alpha3.BackendTLSPolicy
		if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: name}, &item); err != nil {
			t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
		}
		if len(item.Status.Ancestors) != 1 {
			t.Fatalf("expected 1 ancestor for %s, got %d", name, len(item.Status.Ancestors))
		}
		assertCondition(
			t,
			item.Status.Ancestors[0].Conditions,
			string(gatewayv1.PolicyConditionAccepted),
			metav1.ConditionTrue,
			string(gatewayv1.PolicyReasonAccepted),
			1,
		)
	}
}

func TestReconcileConflictedBackendTLSPolicyPreservesResolvedRefsFailure(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8443}},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-ca-valid", Namespace: "default"},
				Data: map[string]string{
					"ca.crt": readStatusTLSAsset(t, "client.crt"),
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "orders-tls-a",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "orders-tls-b",
					Namespace:         "default",
					Generation:        1,
					CreationTimestamp: metav1.NewTime(time.Unix(20, 0)),
				},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname: "orders.internal.example",
						CACertificateRefs: []gatewayv1.LocalObjectReference{
							{Name: "orders-ca-missing"},
							{Name: "orders-ca-valid"},
						},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls-b"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	if len(policy.Status.Ancestors) != 1 {
		t.Fatalf("expected 1 ancestor, got %d", len(policy.Status.Ancestors))
	}
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonConflicted),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionFalse,
		backendTLSPolicyReasonInvalidCACertRef,
		1,
	)
	assertConditionMessage(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		"BackendTLSPolicy conflicts with another policy targeting the same backend",
	)
	assertConditionMessage(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		"BackendTLSPolicy CA ref ConfigMap default/orders-ca-missing was not found or does not contain ca.crt",
	)
}

func TestReconcileRejectsBackendTLSPolicyWithMissingTargetService(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	systemCA := gatewayv1.WellKnownCACertificatesSystem

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1alpha3.BackendTLSPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-tls", Namespace: "default", Generation: 1},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "orders",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "orders.internal.example",
						WellKnownCACertificates: &systemCA,
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy gatewayv1alpha3.BackendTLSPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-tls"}, &policy); err != nil {
		t.Fatalf("Get BackendTLSPolicy returned error: %v", err)
	}

	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonTargetNotFound),
		1,
	)
	assertCondition(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonTargetNotFound),
		1,
	)
	assertConditionMessage(
		t,
		policy.Status.Ancestors[0].Conditions,
		string(gatewayv1.PolicyConditionAccepted),
		"BackendTLSPolicy target Service was not found",
	)
	assertConditionMessage(
		t,
		policy.Status.Ancestors[0].Conditions,
		backendTLSPolicyConditionResolvedRefs,
		"BackendTLSPolicy target Service was not found",
	)
}
