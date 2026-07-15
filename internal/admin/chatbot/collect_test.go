package chatbot

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backendlb "github.com/nantian-gw/gateway/internal/gatewayexp/backendlb"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
)

const ragControllerName = "gateway.networking.k8s.io/nantian-gw"

// fullScheme registers the always-on Gateway API types plus the feature-gated
// Nantian CRDs, mirroring the manager scheme in experimental+AI mode.
func fullScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(gatewayv1.Install(s))
	utilruntime.Must(gatewayv1alpha2.Install(s))
	utilruntime.Must(aiservice.AddToScheme(s))
	utilruntime.Must(tokenpolicy.AddToScheme(s))
	utilruntime.Must(wasmplugin.AddToScheme(s))
	utilruntime.Must(backendlb.Install(s))
	return s
}

func hasEntry(index ClusterIndex, kind, ns, name string) bool {
	for _, e := range index.Entries {
		if e.Ref.Kind == kind && e.Ref.Namespace == ns && e.Ref.Name == name {
			return true
		}
	}
	return false
}

func managedGatewayClass() *gatewayv1.GatewayClass {
	return &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec:       gatewayv1.GatewayClassSpec{ControllerName: gatewayv1.GatewayController(ragControllerName)},
	}
}

// TestCollectIndex_L4RouteCascade verifies a TCPRoute attached to a managed
// Gateway is kept and its backend Service is pulled in via the cascade.
func TestCollectIndex_L4RouteCascade(t *testing.T) {
	t.Parallel()

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "tcp-gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("nantian-gw"),
			Listeners:        []gatewayv1.Listener{{Name: "tcp", Port: 9000, Protocol: gatewayv1.TCPProtocolType}},
		},
	}
	tcpRoute := &gatewayv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "tcp-route", Namespace: "default"},
		Spec: gatewayv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName("tcp-gw")}},
			},
			Rules: []gatewayv1alpha2.TCPRouteRule{
				{BackendRefs: []gatewayv1.BackendRef{
					{BackendObjectReference: gatewayv1.BackendObjectReference{Name: gatewayv1.ObjectName("tcp-svc")}},
				}},
			},
		},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "tcp-svc", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 9000}}},
	}
	// An unattached TCPRoute (parent is an unmanaged/nonexistent Gateway) must be excluded.
	orphan := &gatewayv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "default"},
		Spec: gatewayv1alpha2.TCPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName("ghost")}},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(fullScheme()).WithObjects(managedGatewayClass(), gw, tcpRoute, svc, orphan).Build()

	index, err := collectIndex(context.Background(), cl, ragControllerName, nil)
	if err != nil {
		t.Fatalf("collectIndex error: %v", err)
	}
	if !hasEntry(index, kindTCPRoute, "default", "tcp-route") {
		t.Error("attached TCPRoute should be indexed")
	}
	if !hasEntry(index, kindService, "default", "tcp-svc") {
		t.Error("backend Service should be pulled in via cascade")
	}
	if hasEntry(index, kindTCPRoute, "default", "orphan") {
		t.Error("TCPRoute attached to an unmanaged Gateway must be excluded")
	}
}

// TestCollectIndex_FeatureGatedPresent verifies the Nantian CRDs are indexed
// when the scheme recognizes them.
func TestCollectIndex_FeatureGatedPresent(t *testing.T) {
	t.Parallel()

	ai := &aiservice.AIService{
		ObjectMeta: metav1.ObjectMeta{Name: "gpt", Namespace: "ai"},
		Spec:       aiservice.AIServiceSpec{Provider: "openai", Model: "gpt-4o", Endpoint: "https://api.openai.com"},
	}
	tp := &tokenpolicy.TokenPolicy{ObjectMeta: metav1.ObjectMeta{Name: "tp", Namespace: "ai"}}
	wp := &wasmplugin.WasmPlugin{ObjectMeta: metav1.ObjectMeta{Name: "wp", Namespace: "ai"}}
	lb := &backendlb.BackendLBPolicy{ObjectMeta: metav1.ObjectMeta{Name: "lb", Namespace: "ai"}}

	cl := fake.NewClientBuilder().WithScheme(fullScheme()).WithObjects(managedGatewayClass(), ai, tp, wp, lb).Build()

	index, err := collectIndex(context.Background(), cl, ragControllerName, nil)
	if err != nil {
		t.Fatalf("collectIndex error: %v", err)
	}
	for _, want := range []struct{ kind, name string }{
		{kindAIService, "gpt"},
		{kindTokenPolicy, "tp"},
		{kindWasmPlugin, "wp"},
		{kindBackendLBPolicy, "lb"},
	} {
		if !hasEntry(index, want.kind, "ai", want.name) {
			t.Errorf("expected %s ai/%s to be indexed", want.kind, want.name)
		}
	}
}

// TestCollectIndex_FeatureGatedAbsent verifies collectIndex tolerates CRD types
// that are not registered in the scheme (feature disabled) — no error, no entries.
func TestCollectIndex_FeatureGatedAbsent(t *testing.T) {
	t.Parallel()

	// ragTestScheme registers core + gateway v1/v1alpha2 but NOT the Nantian CRDs.
	cl := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(managedGatewayClass()).Build()

	index, err := collectIndex(context.Background(), cl, ragControllerName, nil)
	if err != nil {
		t.Fatalf("collectIndex should tolerate unregistered CRD types: %v", err)
	}
	if !index.hasManagedClass {
		t.Error("managed GatewayClass should be detected")
	}
	if hasEntry(index, kindAIService, "ai", "gpt") {
		t.Error("no AIService entries expected when the type is unregistered")
	}
}
