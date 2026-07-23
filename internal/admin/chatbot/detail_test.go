package chatbot

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
)

func TestRenderDetail_Gateway(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate
	host := gatewayv1.Hostname("api.example.com")
	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name: "https", Port: 443, Protocol: gatewayv1.HTTPSProtocolType, Hostname: &host,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode:            &mode,
					CertificateRefs: []gatewayv1.SecretObjectReference{{Name: "cert"}},
				},
			}},
		},
		Status: gatewayv1.GatewayStatus{Conditions: []metav1.Condition{
			{Type: "Programmed", Status: metav1.ConditionTrue, Reason: "Programmed"},
		}},
	}
	out := renderDetail(gw, IndexEntry{})
	for _, want := range []string{"class=nantian-gw", "https", "443", "HTTPS", "api.example.com", "tls=Terminate", "Programmed=True"} {
		if !strings.Contains(out, want) {
			t.Errorf("Gateway detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_Service(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.5",
			Ports: []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(8080)}},
		},
	}
	out := renderDetail(svc, IndexEntry{})
	for _, want := range []string{"type=ClusterIP", "10.0.0.5", "80", "8080"} {
		if !strings.Contains(out, want) {
			t.Errorf("Service detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_DefaultFallback(t *testing.T) {
	out := renderDetail(nil, IndexEntry{Summary: "some summary", StatusSummary: "Accepted=True"})
	if !strings.Contains(out, "some summary") || !strings.Contains(out, "Accepted=True") {
		t.Errorf("fallback should use entry summary/status, got %q", out)
	}
}

func TestRenderDetail_HTTPRoute(t *testing.T) {
	pt := gatewayv1.PathMatchPathPrefix
	pv := "/api"
	w := int32(70)
	r := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			Hostnames: []gatewayv1.Hostname{"api.example.com"},
			Rules: []gatewayv1.HTTPRouteRule{{
				Matches: []gatewayv1.HTTPRouteMatch{{Path: &gatewayv1.HTTPPathMatch{Type: &pt, Value: &pv}}},
				BackendRefs: []gatewayv1.HTTPBackendRef{{BackendRef: gatewayv1.BackendRef{
					BackendObjectReference: gatewayv1.BackendObjectReference{Name: "svc", Port: ptr(gatewayv1.PortNumber(8080))},
					Weight:                 &w,
				}}},
			}},
		},
		Status: gatewayv1.HTTPRouteStatus{RouteStatus: gatewayv1.RouteStatus{Parents: []gatewayv1.RouteParentStatus{{
			ParentRef:  gatewayv1.ParentReference{Name: "public"},
			Conditions: []metav1.Condition{{Type: "Accepted", Status: metav1.ConditionTrue, Reason: "Accepted"}},
		}}}},
	}
	out := renderDetail(r, IndexEntry{})
	for _, want := range []string{"api.example.com", "PathPrefix=/api", "svc", "8080", "weight=70", "parent[public]", "Accepted=True"} {
		if !strings.Contains(out, want) {
			t.Errorf("HTTPRoute detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_GRPCRoute(t *testing.T) {
	svc := "users.UserService"
	r := &gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "users", Namespace: "default"},
		Spec: gatewayv1.GRPCRouteSpec{Rules: []gatewayv1.GRPCRouteRule{{
			Matches: []gatewayv1.GRPCRouteMatch{{Method: &gatewayv1.GRPCMethodMatch{Service: &svc}}},
			BackendRefs: []gatewayv1.GRPCBackendRef{{BackendRef: gatewayv1.BackendRef{
				BackendObjectReference: gatewayv1.BackendObjectReference{Name: "grpc-svc", Port: ptr(gatewayv1.PortNumber(50051))},
			}}},
		}}},
	}
	out := renderDetail(r, IndexEntry{})
	for _, want := range []string{"service=users.UserService", "grpc-svc", "50051"} {
		if !strings.Contains(out, want) {
			t.Errorf("GRPCRoute detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_TCPRoute(t *testing.T) {
	r := &gatewayv1alpha2.TCPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "tcp", Namespace: "default"},
		Spec: gatewayv1alpha2.TCPRouteSpec{Rules: []gatewayv1alpha2.TCPRouteRule{{
			BackendRefs: []gatewayv1.BackendRef{{BackendObjectReference: gatewayv1.BackendObjectReference{Name: "tcp-svc", Port: ptr(gatewayv1.PortNumber(9000))}}},
		}}},
	}
	out := renderDetail(r, IndexEntry{})
	for _, want := range []string{"tcp-svc", "9000"} {
		if !strings.Contains(out, want) {
			t.Errorf("TCPRoute detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_AIService(t *testing.T) {
	ai := &aiservice.AIService{
		ObjectMeta: metav1.ObjectMeta{Name: "gpt", Namespace: "ai"},
		Spec:       aiservice.AIServiceSpec{Provider: "openai", Model: "gpt-4o", Format: "openai", Endpoint: "https://api.openai.com", Timeout: "30s"},
	}
	out := renderDetail(ai, IndexEntry{})
	for _, want := range []string{"provider=openai", "model=gpt-4o", "endpoint=https://api.openai.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("AIService detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_TokenPolicy(t *testing.T) {
	p := &tokenpolicy.TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "tp", Namespace: "ai"},
		Spec:       tokenpolicy.TokenPolicySpec{RequestsPerMinute: 100, OnLimit: "reject"},
	}
	out := renderDetail(p, IndexEntry{})
	for _, want := range []string{"rpm=100", "onLimit=reject"} {
		if !strings.Contains(out, want) {
			t.Errorf("TokenPolicy detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_WasmPlugin(t *testing.T) {
	wp := &wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "wp", Namespace: "ai"},
		Spec: wasmplugin.WasmPluginSpec{
			Wasm:    wasmplugin.WasmSource{URL: "oci://example/plugin"},
			Hooks:   []wasmplugin.WasmHook{wasmplugin.HookOnRequest},
			Sandbox: wasmplugin.WasmSandbox{MaxMemoryBytes: 1048576},
		},
	}
	out := renderDetail(wp, IndexEntry{})
	for _, want := range []string{"oci://example/plugin", "onRequest", "maxMemoryBytes=1048576"} {
		if !strings.Contains(out, want) {
			t.Errorf("WasmPlugin detail missing %q\n%s", want, out)
		}
	}
}

func TestRenderDetail_BackendLBPolicy(t *testing.T) {
	typ := backend.LoadBalancingStrategyTypeConsistentHash
	lb := &backend.BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "lb", Namespace: "ai"},
		Spec:       backend.BackendLBPolicySpec{LoadBalancing: &backend.LoadBalancingPolicy{Type: &typ}},
	}
	out := renderDetail(lb, IndexEntry{})
	if !strings.Contains(out, "lb=ConsistentHash") {
		t.Errorf("BackendLBPolicy detail missing lb type\n%s", out)
	}
}
