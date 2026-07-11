package chatbot

import (
	"context"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func ragTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(s))
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(gatewayv1.Install(s))
	utilruntime.Must(gatewayv1alpha2.Install(s))
	return s
}

func TestBuildRAGContext_EmptyCluster(t *testing.T) {
	t.Parallel()

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).Build()
	controllerName := "gateway.networking.k8s.io/nantian-gw"

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName, "list gateways")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No managed GatewayClasses found") {
		t.Errorf("expected 'No managed GatewayClasses found' in empty cluster output, got: %s", result)
	}
}

func TestBuildRAGContext_ManagedTopology(t *testing.T) {
	t.Parallel()

	controllerName := "gateway.networking.k8s.io/nantian-gw"
	sectionName := gatewayv1.SectionName("http")
	pathPrefix := gatewayv1.PathMatchPathPrefix
	pathValue := "/api"

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController(controllerName),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("nantian-gw"),
			Listeners: []gatewayv1.Listener{
				{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
			},
		},
	}

	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{
					{Name: gatewayv1.ObjectName("public"), SectionName: &sectionName},
				},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{
						{Path: &gatewayv1.HTTPPathMatch{Type: &pathPrefix, Value: &pathValue}},
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
		ObjectMeta: metav1.ObjectMeta{Name: "backend-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(8080)},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(gwc, gw, route, svc).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName, "public api backend-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantChecks := []string{
		"default/public",      // Gateway namespace/name
		"default/api",         // HTTPRoute namespace/name
		"default/backend-svc", // Service namespace/name
		"nantian-gw",          // GatewayClass name (in Gateway detail summary)
		"8080",                // Service port (in Service detail summary)
	}
	for _, want := range wantChecks {
		if !strings.Contains(result, want) {
			t.Errorf("RAG output missing expected identifier %q.\nOutput:\n%s", want, result)
		}
	}
}

func TestBuildRAGContext_UnmanagedGatewayClass(t *testing.T) {
	t.Parallel()

	controllerName := "gateway.networking.k8s.io/nantian-gw"

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "other-controller"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController("some.other.controller"),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "private", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("other-controller"),
			Listeners: []gatewayv1.Listener{
				{Name: "http", Port: 8443, Protocol: gatewayv1.HTTPSProtocolType},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(gwc, gw).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName, "private")
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

	controllerName := "gateway.networking.k8s.io/nantian-gw"
	defaultNS := gatewayv1.Namespace("default")

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController(controllerName),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("nantian-gw"),
			Listeners: []gatewayv1.Listener{
				{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType},
			},
		},
	}

	routeDefault := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName("public")}},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{BackendRefs: []gatewayv1.HTTPBackendRef{
					{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: gatewayv1.ObjectName("backend-svc")}}},
				}},
			},
		},
	}

	// staging-api attaches cross-namespace to the managed default/public Gateway.
	routeStaging := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "staging-api", Namespace: "staging"},
		Spec: gatewayv1.HTTPRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName("public"), Namespace: &defaultNS}},
			},
			Rules: []gatewayv1.HTTPRouteRule{
				{BackendRefs: []gatewayv1.HTTPBackendRef{
					{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: gatewayv1.ObjectName("staging-svc")}}},
				}},
			},
		},
	}

	svcDefault := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "backend-svc", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}}},
	}
	svcStaging := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "staging-svc", Namespace: "staging"},
		Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "http", Port: 8080, Protocol: corev1.ProtocolTCP}}},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(
		gwc, gw, routeDefault, routeStaging, svcDefault, svcStaging,
	).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName, "api staging-api backend-svc staging-svc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	controllerName := "gateway.networking.k8s.io/nantian-gw"

	gwc := &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: gatewayv1.GatewayController(controllerName),
		},
	}

	gw := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "grpc-gw", Namespace: "default"},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: gatewayv1.ObjectName("nantian-gw"),
			Listeners: []gatewayv1.Listener{
				{Name: "grpc", Port: 50051, Protocol: gatewayv1.HTTPProtocolType},
			},
		},
	}

	grpcRoute := &gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "users", Namespace: "default"},
		Spec: gatewayv1.GRPCRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName("grpc-gw")}},
			},
			Rules: []gatewayv1.GRPCRouteRule{
				{
					Matches: []gatewayv1.GRPCRouteMatch{
						{Method: &gatewayv1.GRPCMethodMatch{Service: ptr("users.UserService")}},
					},
					BackendRefs: []gatewayv1.GRPCBackendRef{
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{
							Name: gatewayv1.ObjectName("grpc-backend"),
							Port: ptr(gatewayv1.PortNumber(50051)),
						}}},
					},
				},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(gwc, gw, grpcRoute).Build()

	result, err := BuildRAGContext(context.Background(), k8sClient, controllerName, "grpc users route")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantChecks := []string{
		"default/grpc-gw",
		"default/users",
		"GRPCRoute default/users",
	}
	for _, want := range wantChecks {
		if !strings.Contains(result, want) {
			t.Errorf("RAG output missing expected identifier %q.\nOutput:\n%s", want, result)
		}
	}
}

func TestBuildRAGContext_LargeClusterStaysBounded(t *testing.T) {
	t.Parallel()

	controllerName := "gateway.networking.k8s.io/nantian-gw"

	objs := []client.Object{
		&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec:       gatewayv1.GatewayClassSpec{ControllerName: gatewayv1.GatewayController(controllerName)},
		},
		&gatewayv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "shop"},
			Spec: gatewayv1.GatewaySpec{
				GatewayClassName: gatewayv1.ObjectName("nantian-gw"),
				Listeners:        []gatewayv1.Listener{{Name: "http", Port: 80, Protocol: gatewayv1.HTTPProtocolType}},
			},
		},
		&gatewayv1.HTTPRoute{
			ObjectMeta: metav1.ObjectMeta{Name: "checkout-route", Namespace: "shop"},
			Spec: gatewayv1.HTTPRouteSpec{
				CommonRouteSpec: gatewayv1.CommonRouteSpec{
					ParentRefs: []gatewayv1.ParentReference{{Name: gatewayv1.ObjectName("gw")}},
				},
				Rules: []gatewayv1.HTTPRouteRule{
					{BackendRefs: []gatewayv1.HTTPBackendRef{
						{BackendRef: gatewayv1.BackendRef{BackendObjectReference: gatewayv1.BackendObjectReference{Name: gatewayv1.ObjectName("checkout-svc")}}},
					}},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "checkout-svc", Namespace: "shop"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8080}}},
		},
	}
	// 300 unreferenced services: correctly dropped by the cascade, keeping output bounded.
	for i := 0; i < 300; i++ {
		objs = append(objs, &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc-" + strconv.Itoa(i), Namespace: "default"},
			Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 80}}},
		})
	}

	cl := fake.NewClientBuilder().WithScheme(ragTestScheme()).WithObjects(objs...).Build()

	out, err := BuildRAGContext(context.Background(), cl, controllerName, "tell me about checkout-svc in shop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Service shop/checkout-svc") {
		t.Errorf("targeted service should be selected into the output:\n%s", out)
	}
	if !strings.Contains(out, "## Relevant Resources") {
		t.Error("expected detail section header")
	}
	if strings.Contains(out, "default/svc-0") {
		t.Errorf("unreferenced service should be dropped by cascade:\n%s", out)
	}
}

func ptr[T any](v T) *T {
	return &v
}
