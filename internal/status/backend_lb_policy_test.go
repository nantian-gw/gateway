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

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
)

func TestReconcileAcceptsBackendLBPolicy(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
			&backend.BackendLBPolicy{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "orders",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			&backend.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "orders-sticky", Namespace: "default", Generation: 1},
				Spec: backend.BackendLBPolicySpec{
					TargetRefs: []backend.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "orders",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy backend.BackendLBPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "orders-sticky"}, &policy); err != nil {
		t.Fatalf("Get BackendLBPolicy returned error: %v", err)
	}
	if len(policy.Status.Ancestors) != 1 {
		t.Fatalf("expected 1 ancestor, got %d", len(policy.Status.Ancestors))
	}
	assertCondition(t, policy.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), metav1.ConditionTrue, string(backend.PolicyReasonAccepted), 1)
	assertCondition(t, policy.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, metav1.ConditionTrue, backendLBPolicyReasonResolvedRefs, 1)
	assertConditionMessage(t, policy.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), "BackendLBPolicy is accepted by nantian-gw")
	assertConditionMessage(t, policy.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, "BackendLBPolicy references are resolved")
}

func TestReconcileAppliesBackendLBPolicyPrecedence(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	policy := func(name string, created time.Time) *backend.BackendLBPolicy {
		return &backend.BackendLBPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:              name,
				Namespace:         "default",
				Generation:        1,
				CreationTimestamp: metav1.NewTime(created),
			},
			Spec: backend.BackendLBPolicySpec{
				TargetRefs: []backend.LocalPolicyTargetReference{{
					Group: "",
					Kind:  "Service",
					Name:  "orders",
				}},
				SessionPersistence: &gatewayv1.SessionPersistence{},
			},
		}
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
			&gatewayv1.HTTPRoute{},
			&backend.BackendLBPolicy{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default", Generation: 1},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "orders",
									Port: portPtr(8080),
								},
							},
						}},
					}},
				},
			},
			policy("older", time.Unix(10, 0)),
			policy("newer", time.Unix(20, 0)),
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
		{name: "older", acceptedStatus: metav1.ConditionTrue, acceptedReason: string(backend.PolicyReasonAccepted)},
		{name: "newer", acceptedStatus: metav1.ConditionFalse, acceptedReason: string(backend.PolicyReasonConflicted)},
	} {
		var item backend.BackendLBPolicy
		if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: tc.name}, &item); err != nil {
			t.Fatalf("Get BackendLBPolicy returned error: %v", err)
		}
		if len(item.Status.Ancestors) != 1 {
			t.Fatalf("expected 1 ancestor for %s, got %d", tc.name, len(item.Status.Ancestors))
		}
		assertCondition(t, item.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), tc.acceptedStatus, tc.acceptedReason, 1)
		assertCondition(t, item.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, metav1.ConditionTrue, backendLBPolicyReasonResolvedRefs, 1)
		switch tc.name {
		case "older":
			assertConditionMessage(t, item.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), "BackendLBPolicy is accepted by nantian-gw")
		case "newer":
			assertConditionMessage(t, item.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), "BackendLBPolicy conflicts with another policy targeting the same backend")
		}
		assertConditionMessage(t, item.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, "BackendLBPolicy references are resolved")
	}
}

func TestReconcileRejectsBackendLBPolicyWithMissingTargetService(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&backend.BackendLBPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&backend.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "missing", Namespace: "default", Generation: 1},
				Spec: backend.BackendLBPolicySpec{
					TargetRefs: []backend.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "orders",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy backend.BackendLBPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "missing"}, &policy); err != nil {
		t.Fatalf("Get BackendLBPolicy returned error: %v", err)
	}
	if len(policy.Status.Ancestors) != 1 {
		t.Fatalf("expected 1 ancestor, got %d", len(policy.Status.Ancestors))
	}
	assertCondition(t, policy.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), 1)
	assertCondition(t, policy.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), 1)
	assertConditionMessage(t, policy.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), "BackendLBPolicy target Service was not found")
	assertConditionMessage(t, policy.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, "BackendLBPolicy target Service was not found")
}

func TestReconcileRejectsBackendLBPolicyWithInvalidConsistentHash(t *testing.T) {
	scheme := newScheme(t)

	headerKeyType := backend.HashKeyTypeHeader
	consistentHash := &backend.ConsistentHashPolicy{
		KeyType: &headerKeyType,
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&backend.BackendLBPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{Port: 8080}},
				},
			},
			&backend.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "invalid-hash", Namespace: "default", Generation: 1},
				Spec: backend.BackendLBPolicySpec{
					TargetRefs: []backend.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "orders",
					}},
					LoadBalancing: &backend.LoadBalancingPolicy{
						ConsistentHash: consistentHash,
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var policy backend.BackendLBPolicy
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "invalid-hash"}, &policy); err != nil {
		t.Fatalf("Get BackendLBPolicy returned error: %v", err)
	}
	if len(policy.Status.Ancestors) != 1 {
		t.Fatalf("expected 1 ancestor, got %d", len(policy.Status.Ancestors))
	}
	assertCondition(t, policy.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), metav1.ConditionFalse, string(backend.PolicyReasonInvalid), 1)
	assertCondition(t, policy.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, metav1.ConditionFalse, string(backend.PolicyReasonInvalid), 1)
	assertConditionMessage(t, policy.Status.Ancestors[0].Conditions, string(backend.PolicyConditionAccepted), "BackendLBPolicy consistent hash header strategy requires headerName")
	assertConditionMessage(t, policy.Status.Ancestors[0].Conditions, backendLBPolicyConditionResolvedRefs, "BackendLBPolicy consistent hash header strategy requires headerName")
}
