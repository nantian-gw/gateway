package status

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
)

func TestReconcileAIServiceObjectWritesAcceptedCondition(t *testing.T) {
	scheme := newScheme(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aiservice.AIService{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&aiservice.AIService{
				ObjectMeta: metav1.ObjectMeta{Name: "my-ai", Namespace: "default", Generation: 1},
				Spec: aiservice.AIServiceSpec{
					Provider: "openai",
					Model:    "gpt-4",
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
	key := client.ObjectKey{Namespace: "default", Name: "my-ai"}
	if err := reconciler.reconcileAIServiceObject(context.Background(), key); err != nil {
		t.Fatalf("reconcileAIServiceObject returned error: %v", err)
	}

	var obj aiservice.AIService
	if err := k8sClient.Get(context.Background(), key, &obj); err != nil {
		t.Fatalf("Get AIService returned error: %v", err)
	}
	if len(obj.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(obj.Status.Conditions))
	}
	assertCondition(t, obj.Status.Conditions, "Accepted", metav1.ConditionTrue, "Accepted", 1)
	assertConditionMessage(t, obj.Status.Conditions, "Accepted", "AIService is accepted by nantian-gw")
}

func TestReconcileTokenPolicyObjectWritesAcceptedCondition(t *testing.T) {
	scheme := newScheme(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&tokenpolicy.TokenPolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&tokenpolicy.TokenPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tokens", Namespace: "default", Generation: 1},
				Spec: tokenpolicy.TokenPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReference{},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
	key := client.ObjectKey{Namespace: "default", Name: "my-tokens"}
	if err := reconciler.reconcileTokenPolicyObject(context.Background(), key); err != nil {
		t.Fatalf("reconcileTokenPolicyObject returned error: %v", err)
	}

	var obj tokenpolicy.TokenPolicy
	if err := k8sClient.Get(context.Background(), key, &obj); err != nil {
		t.Fatalf("Get TokenPolicy returned error: %v", err)
	}
	if len(obj.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(obj.Status.Conditions))
	}
	assertCondition(t, obj.Status.Conditions, "Accepted", metav1.ConditionTrue, "Accepted", 1)
	assertConditionMessage(t, obj.Status.Conditions, "Accepted", "TokenPolicy is accepted by nantian-gw")
}

func TestReconcileWasmPluginObjectWritesAcceptedCondition(t *testing.T) {
	scheme := newScheme(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&wasmplugin.WasmPlugin{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&wasmplugin.WasmPlugin{
				ObjectMeta: metav1.ObjectMeta{Name: "my-wasm", Namespace: "default", Generation: 1},
				Spec: wasmplugin.WasmPluginSpec{
					Wasm: wasmplugin.WasmSource{
						URL: "https://example.com/plugin.wasm",
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
	key := client.ObjectKey{Namespace: "default", Name: "my-wasm"}
	if err := reconciler.reconcileWasmPluginObject(context.Background(), key); err != nil {
		t.Fatalf("reconcileWasmPluginObject returned error: %v", err)
	}

	var obj wasmplugin.WasmPlugin
	if err := k8sClient.Get(context.Background(), key, &obj); err != nil {
		t.Fatalf("Get WasmPlugin returned error: %v", err)
	}
	if len(obj.Status.Conditions) != 2 {
		t.Fatalf("expected 2 conditions, got %d", len(obj.Status.Conditions))
	}
	assertCondition(t, obj.Status.Conditions, "Accepted", metav1.ConditionTrue, "Accepted", 1)
	assertCondition(t, obj.Status.Conditions, "Programmed", metav1.ConditionTrue, "Programmed", 1)
	assertConditionMessage(t, obj.Status.Conditions, "Accepted", "WasmPlugin is accepted by nantian-gw")
	assertConditionMessage(t, obj.Status.Conditions, "Programmed", "WasmPlugin has been programmed into the data plane")
}

func TestReconcileRoutePolicyObjectWritesAcceptedCondition(t *testing.T) {
	scheme := newScheme(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&routepolicy.RoutePolicy{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&routepolicy.RoutePolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "my-rp", Namespace: "default", Generation: 1},
				Spec: routepolicy.RoutePolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReference{},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
	key := client.ObjectKey{Namespace: "default", Name: "my-rp"}
	if err := reconciler.reconcileRoutePolicyObject(context.Background(), key); err != nil {
		t.Fatalf("reconcileRoutePolicyObject returned error: %v", err)
	}

	var obj routepolicy.RoutePolicy
	if err := k8sClient.Get(context.Background(), key, &obj); err != nil {
		t.Fatalf("Get RoutePolicy returned error: %v", err)
	}
	if len(obj.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(obj.Status.Conditions))
	}
	assertCondition(t, obj.Status.Conditions, "Accepted", metav1.ConditionTrue, "Accepted", 1)
	assertConditionMessage(t, obj.Status.Conditions, "Accepted", "RoutePolicy is accepted by nantian-gw")
}

func TestReconcileAIServiceObjectSkipsNotFound(t *testing.T) {
	scheme := newScheme(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&aiservice.AIService{}).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
	if err := reconciler.reconcileAIServiceObject(context.Background(),
		client.ObjectKey{Namespace: "default", Name: "nonexistent"}); err != nil {
		t.Fatalf("expected nil for NotFound, got: %v", err)
	}
}
