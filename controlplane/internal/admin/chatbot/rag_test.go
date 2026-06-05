package chatbot

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func ragTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(gatewayv1.Install(s))
	return s
}

func TestBuildRAGContext_EmptyCluster(t *testing.T) {
	t.Parallel()

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).Build()
	controllerName := "gateway.networking.k8s.io/aether-gateway"

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No managed GatewayClasses found") {
		t.Errorf("expected 'No managed GatewayClasses found' in empty cluster output, got: %s", result)
	}
}

func TestBuildRAGContext_ManagedTopology(t *testing.T) {
	t.Parallel()

	controllerName := "gateway.networking.k8s.io/aether-gateway"
	sectionName := gatewayv1.SectionName("http")
	pathPrefix := gatewayv1.PathMatchPathPrefix
	pathValue := "/api"

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aether-gateway",
		},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController(controllerName),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("aether-gateway"),
			Listeners: []gatewayv1.Listener{
				{
					Name:     gatewayv1.SectionName("http"),
					Port:     gatewayv1.PortNumber(80),
					Protocol: gatewayv1.HTTPProtocolType,
				},
			},
		},
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api",
			Namespace: "default",
		},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{
						Name:        gatewayv1.ObjectName("public"),
						SectionName: &sectionName,
					},
				},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{
							Path: &gatewayv1.HTTPPathMatch{
								Type:  &pathPrefix,
								Value: &pathValue,
							},
						},
					},
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName("backend-svc"),
									Port: ptr(gatewayv1.PortNumber(8080)),
								},
							},
						},
					},
				},
			},
		},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backend-svc",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(
		gwc, gw, route, svc,
	).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify key identifiers are present in the RAG output.
	wantChecks := []string{
		"default/public",     // Gateway namespace/name
		"default/api",        // HTTPRoute namespace/name
		"default/backend-svc", // Service namespace/name
		"aether-gateway",            // GatewayClass name
		"8080",               // Service port
	}

	for _, want := range wantChecks {
		if !strings.Contains(result, want) {
			t.Errorf("RAG output missing expected identifier %q.\nOutput:\n%s", want, result)
		}
	}
}

func TestBuildRAGContext_UnmanagedGatewayClass(t *testing.T) {
	t.Parallel()

	controllerName := "gateway.networking.k8s.io/aether-gateway"

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-controller",
		},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController("some.other.controller"),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "private",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("other-controller"),
			Listeners: []gatewayv1.Listener{
				{
					Name:     gatewayv1.SectionName("http"),
					Port:     gatewayv1.PortNumber(8443),
					Protocol: gatewayv1.HTTPSProtocolType,
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(
		gwc, gw,
	).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "No managed GatewayClasses found") {
		t.Errorf("expected 'No managed GatewayClasses found' when no managed GC exists, got: %s", result)
	}
	if strings.Contains(result, "private") {
		t.Errorf("unmanaged Gateway should not appear in output:\n%s", result)
	}
}

func TestBuildRAGContext_MultipleNamespaces(t *testing.T) {
	t.Parallel()

	controllerName := "gateway.networking.k8s.io/aether-gateway"

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController(controllerName),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("aether-gateway"),
			Listeners: []gatewayv1.Listener{
				{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
			},
		},
	}

	routeDefault := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: gatewayv1.ObjectName("public")},
				},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName("backend-svc"),
								},
							},
						},
					},
				},
			},
		},
	}

	routeStaging := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "staging-api", Namespace: "staging"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: gatewayv1.ObjectName("public")},
				},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName("staging-svc"),
								},
							},
						},
					},
				},
			},
		},
	}

	svcDefault := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "backend-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
		},
	}
	svcStaging := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "staging-svc", Namespace: "staging"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP}},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(
		gwc, gw, routeDefault, routeStaging, svcDefault, svcStaging,
	).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All namespaces and resources should be represented.
	wantChecks := []string{
		"default/public",
		"default/api",
		"staging/staging-api",
		"default/backend-svc",
		"staging/staging-svc",
	}

	for _, want := range wantChecks {
		if !strings.Contains(result, want) {
			t.Errorf("RAG output missing expected identifier %q.\nOutput:\n%s", want, result)
		}
	}
}

func TestBuildRAGContext_GRPCRouteIncluded(t *testing.T) {
	t.Parallel()

	controllerName := "gateway.networking.k8s.io/aether-gateway"

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController(controllerName),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "grpc-gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("aether-gateway"),
			Listeners: []gatewayv1.Listener{
				{Name: "grpc", Port: 50051, Protocol: gatewayv1.HTTPProtocolType},
			},
		},
	}

	grpcRoute := &gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "users", Namespace: "default"},
		Spec: gatewayv1.GRPCRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: gatewayv1.ObjectName("grpc-gw")},
				},
			},
			Rules: []gatewayv1.GRPCRouteRule{
				{
					Matches: []gatewayv1.GRPCRouteMatch{
						{
							Method: &gatewayv1.GRPCMethodMatch{
								Service: ptr("users.UserService"),
							},
						},
					},
					BackendRefs: []gatewayv1.GRPCBackendRef{
						{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: gatewayv1.ObjectName("grpc-backend"),
									Port: ptr(gatewayv1.PortNumber(50051)),
								},
							},
						},
					},
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(
		gwc, gw, grpcRoute,
	).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantChecks := []string{
		"default/grpc-gw",
		"default/users",
		"users.UserService",
		"grpc-backend",
	}

	for _, want := range wantChecks {
		if !strings.Contains(result, want) {
			t.Errorf("RAG output missing expected identifier %q.\nOutput:\n%s", want, result)
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}