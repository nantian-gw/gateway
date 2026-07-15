package admin

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/infrastructure"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/noderegistry"
)

const testAuthToken = "test-admin-token"

func TestNewServerAppliesRuntimeTimeouts(t *testing.T) {
	t.Parallel()

	server := newTestServerWithOptions(t, Options{
		ReadHeaderTimeout: 6 * time.Second,
		ReadTimeout:       35 * time.Second,
		WriteTimeout:      40 * time.Second,
		IdleTimeout:       3 * time.Minute,
	})

	if got := server.server.ReadHeaderTimeout; got != 6*time.Second {
		t.Fatalf("unexpected read header timeout: %s", got)
	}
	if got := server.server.ReadTimeout; got != 35*time.Second {
		t.Fatalf("unexpected read timeout: %s", got)
	}
	if got := server.server.WriteTimeout; got != 40*time.Second {
		t.Fatalf("unexpected write timeout: %s", got)
	}
	if got := server.server.IdleTimeout; got != 3*time.Minute {
		t.Fatalf("unexpected idle timeout: %s", got)
	}
}

func TestDashboardCapabilitiesEndpointReturnsConfiguredPageGroups(t *testing.T) {
	t.Parallel()

	want := DashboardCapabilities{
		Overview:        true,
		Gateways:        true,
		AIOverview:      true,
		AIServices:      true,
		AITokenPolicies: false,
		WasmPlugins:     false,
		Chatbot:         true,
	}
	server := newTestServerWithOptions(t, Options{DashboardCapabilities: want})

	recorder := performRequest(t, server, http.MethodGet, "/v1/dashboard/capabilities", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var resp DashboardCapabilities
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp != want {
		t.Fatalf("dashboard capabilities mismatch: got %+v, want %+v", resp, want)
	}
}

func newTestServer(t *testing.T) *Server {
	return newTestServerWithRepository(t, nil, Options{})
}

func newInfrastructureTestServer(t *testing.T) *Server {
	t.Helper()

	server := newTestServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(discoveryv1.AddToScheme(scheme))
	utilruntime.Must(gatewayv1.Install(scheme))
	utilruntime.Must(gatewayv1alpha2.Install(scheme))

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)
	httpProtocol := "http"
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, "nantian.dev/infrastructure.gatewayclass.controller-name", gatewayClassControllerIndexKeys).
		WithIndex(&gatewayv1.Gateway{}, "nantian.dev/infrastructure.gateway.gatewayclass-name", gatewayGatewayClassNameIndexKeys).
		WithIndex(&gatewayv1.HTTPRoute{}, "nantian.dev/infrastructure.httproute.service-parents", func(object client.Object) []string {
			route, ok := object.(*gatewayv1.HTTPRoute)
			if !ok {
				return nil
			}
			return serviceParentRouteIndexKeys(route.Spec.ParentRefs, route.Namespace)
		}).
		WithIndex(&gatewayv1.GRPCRoute{}, "nantian.dev/infrastructure.grpcroute.service-parents", func(object client.Object) []string {
			route, ok := object.(*gatewayv1.GRPCRoute)
			if !ok {
				return nil
			}
			return serviceParentRouteIndexKeys(route.Spec.ParentRefs, route.Namespace)
		}).
		WithIndex(&gatewayv1alpha2.TCPRoute{}, "nantian.dev/infrastructure.tcproute.service-parents", func(object client.Object) []string {
			route, ok := object.(*gatewayv1alpha2.TCPRoute)
			if !ok {
				return nil
			}
			return serviceParentRouteIndexKeys(route.Spec.ParentRefs, route.Namespace)
		}).
		WithIndex(&gatewayv1alpha2.UDPRoute{}, "nantian.dev/infrastructure.udproute.service-parents", func(object client.Object) []string {
			route, ok := object.(*gatewayv1alpha2.UDPRoute)
			if !ok {
				return nil
			}
			return serviceParentRouteIndexKeys(route.Spec.ParentRefs, route.Namespace)
		}).
		WithIndex(&gatewayv1alpha2.TLSRoute{}, "nantian.dev/infrastructure.tlsroute.service-parents", func(object client.Object) []string {
			route, ok := object.(*gatewayv1alpha2.TLSRoute)
			if !ok {
				return nil
			}
			return serviceParentRouteIndexKeys(route.Spec.ParentRefs, route.Namespace)
		}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-0",
					Namespace: "nantian-gw",
					Labels:    map[string]string{"app": "nantian-gw-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("public"),
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "tcp-81",
						Port:       81,
						TargetPort: intstr.FromInt(81),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &httpProtocol,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mesh",
					Namespace: "default",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mesh.ShadowServiceName("default", "stale"),
					Namespace: "default",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":     "nantian-gw",
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: "default",
						mesh.OriginalServiceNameLabel:      "stale",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		).
		Build()

	server.SetInfrastructureInspector(infrastructure.New(k8sClient, string(controllerName), logger))
	return server
}

func newTestServerWithOptions(t *testing.T, opts Options) *Server {
	return newTestServerWithRepository(t, nil, opts)
}

func newTestServerWithRepository(t *testing.T, repo noderegistry.Repository, opts Options) *Server {
	t.Helper()

	if !IsAuthConfigured(opts) {
		opts.BearerToken = testAuthToken
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	nodes := noderegistry.NewRegistry(
		ir.NewNodeStatusStore(),
		repo,
		logger,
		noderegistry.Options{PersistTimeout: time.Second},
	)
	server := NewServer(":0", store, nodes, nil, logger, opts)

	now := time.Now().UTC()
	store.Publish(&ir.Snapshot{
		GeneratedAt: now,
		Workloads: []ir.Workload{
			{Namespace: "nantian-gw", Name: "dp-1", IP: "10.0.0.1"},
			{Namespace: "nantian-gw", Name: "dp-2", IP: "10.0.0.2"},
			{Namespace: "nantian-gw", Name: "dp-3", IP: "10.0.0.3"},
		},
		Listeners: []ir.Listener{
			{
				Name:           "web",
				Address:        "192.0.2.10",
				Addresses:      []string{"192.0.2.10", "gw.example.com"},
				Protocol:       "HTTP",
				Hostnames:      []string{"app.example.com"},
				AttachedRoutes: []string{"default/web"},
				Status: &ir.ListenerStatus{
					AttachedRoutes: 1,
					Conditions: []ir.ConditionStatus{
						{Type: "Accepted", Status: "True"},
						{Type: "Programmed", Status: "True"},
						{Type: "ResolvedRefs", Status: "True"},
					},
					Accepted:     &ir.ConditionStatus{Type: "Accepted", Status: "True"},
					Programmed:   &ir.ConditionStatus{Type: "Programmed", Status: "True"},
					ResolvedRefs: &ir.ConditionStatus{Type: "ResolvedRefs", Status: "True"},
				},
			},
			{
				Name:      "passthrough",
				Protocol:  "TLS",
				Hostnames: []string{"secure.example.com"},
			},
		},
		HTTPRoutes: []ir.HTTPRoute{
			{
				Name:      "web",
				Namespace: "default",
				Hostnames: []string{"app.example.com"},
				Rules: []ir.HTTPRule{
					{
						BackendRefs: []ir.BackendRef{{
							Name:      "api",
							Namespace: "default",
							Port:      80,
						}},
					},
				},
				Status: &ir.RouteStatus{
					Parents: []ir.RouteParentStatus{{
						ControllerName: "gateway.networking.k8s.io/nantian-gw",
						ParentRef: ir.ParentRef{
							Group:       "gateway.networking.k8s.io",
							Kind:        "Gateway",
							Namespace:   "default",
							Name:        "gw",
							SectionName: "http",
						},
						Conditions: []ir.ConditionStatus{
							{Type: "Accepted", Status: "True"},
							{Type: "ResolvedRefs", Status: "True"},
						},
						Accepted:     &ir.ConditionStatus{Type: "Accepted", Status: "True"},
						ResolvedRefs: &ir.ConditionStatus{Type: "ResolvedRefs", Status: "True"},
					}},
				},
			},
		},
		GRPCRoutes: []ir.GRPCRoute{
			{
				Name:      "grpc",
				Namespace: "default",
				Hostnames: []string{"grpc.example.com"},
			},
		},
		StreamRoutes: []ir.StreamRoute{
			{
				Name:      "passthrough",
				Namespace: "default",
				Kind:      "TLS",
				Rules: []ir.StreamRule{
					{
						Matches: []ir.StreamMatch{{SNIHostname: "secure.example.com"}},
					},
				},
				Status: &ir.RouteStatus{
					Parents: []ir.RouteParentStatus{{
						ControllerName: "gateway.networking.k8s.io/nantian-gw",
						ParentRef: ir.ParentRef{
							Group:       "gateway.networking.k8s.io",
							Kind:        "Gateway",
							Namespace:   "default",
							Name:        "gw",
							SectionName: "tls",
						},
						Conditions: []ir.ConditionStatus{
							{Type: "Accepted", Status: "True"},
							{Type: "ResolvedRefs", Status: "True"},
						},
						Accepted:     &ir.ConditionStatus{Type: "Accepted", Status: "True"},
						ResolvedRefs: &ir.ConditionStatus{Type: "ResolvedRefs", Status: "True"},
					}},
				},
			},
		},
		Backends: []ir.BackendCluster{
			{
				Name:      "api:80",
				Namespace: "default",
				Protocol:  "HTTP",
				Metadata:  map[string]string{"service": "api"},
			},
			{
				Name:      "metrics:9090",
				Namespace: "ops",
				Protocol:  "TCP",
				Metadata:  map[string]string{"service": "metrics"},
			},
		},
	})
	currentVersion := store.Current().ID

	nodes.Connect(context.Background(), "dp-1", "kind", []string{"routes", "listeners"}, now)
	nodes.ObserveAck(context.Background(), "dp-1", "kind", currentVersion, currentVersion, nil, now)
	nodes.ObserveReport(context.Background(), "dp-1", currentVersion, true, "ready", now)
	nodes.Connect(context.Background(), "dp-2", "kind", []string{"routes"}, now)
	nodes.Disconnect(context.Background(), "dp-2", now)
	nodes.ObserveReport(context.Background(), "dp-2", "v0", false, "warming", now)

	return server
}

type testNodeRepository struct {
	mu    sync.RWMutex
	items map[string]ir.NodeStatus
}

func (r *testNodeRepository) Get(_ context.Context, nodeID string) (ir.NodeStatus, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[nodeID]
	return item, ok, nil
}

func (r *testNodeRepository) List(_ context.Context) ([]ir.NodeStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ir.NodeStatus, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, item)
	}
	return out, nil
}

func (r *testNodeRepository) Upsert(_ context.Context, status ir.NodeStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.items == nil {
		r.items = make(map[string]ir.NodeStatus)
	}
	r.items[status.NodeID] = status
	return nil
}

func performRequest(t *testing.T, server *Server, method, path string, target any) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	recorder := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(recorder, req)

	if target != nil && recorder.Code == http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
	}

	return recorder
}

func listenerNames(listeners []ir.Listener) []string {
	out := make([]string, 0, len(listeners))
	for _, listener := range listeners {
		out = append(out, listener.Name)
	}
	return out
}

func backendKeys(backends []ir.BackendCluster) []string {
	out := make([]string, 0, len(backends))
	for _, backend := range backends {
		out = append(out, backend.Namespace+"/"+backend.Name)
	}
	return out
}

func nodeIDs(nodes []ir.NodeStatus) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.NodeID)
	}
	return out
}

func httpRouteKeys(routes []ir.HTTPRoute) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.Namespace+"/"+route.Name)
	}
	return out
}

func grpcRouteKeys(routes []ir.GRPCRoute) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.Namespace+"/"+route.Name)
	}
	return out
}

func streamRouteKeys(routes []ir.StreamRoute) []string {
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.Namespace+"/"+route.Name)
	}
	return out
}

func infrastructureResourceKeys(items []infrastructure.InfrastructureResource) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Namespace+"/"+item.Name)
	}
	return out
}

func histogramVecSampleCount(t *testing.T, vec *prometheus.HistogramVec, labelValues ...string) uint64 {
	t.Helper()

	observer, err := vec.GetMetricWithLabelValues(labelValues...)
	if err != nil {
		t.Fatalf("get histogram metric: %v", err)
	}

	metric, ok := observer.(prometheus.Metric)
	if !ok {
		t.Fatal("histogram observer does not implement prometheus.Metric")
	}

	dtoMetric := &dto.Metric{}
	if err := metric.Write(dtoMetric); err != nil {
		t.Fatalf("write histogram metric: %v", err)
	}
	return dtoMetric.GetHistogram().GetSampleCount()
}

func ptr[T any](value T) *T {
	return &value
}

func gatewayClassControllerIndexKeys(object client.Object) []string {
	gatewayClass, ok := object.(*gatewayv1.GatewayClass)
	if !ok || gatewayClass.Spec.ControllerName == "" {
		return nil
	}
	return []string{string(gatewayClass.Spec.ControllerName)}
}

func gatewayGatewayClassNameIndexKeys(object client.Object) []string {
	gateway, ok := object.(*gatewayv1.Gateway)
	if !ok || gateway.Spec.GatewayClassName == "" {
		return nil
	}
	return []string{string(gateway.Spec.GatewayClassName)}
}

func serviceParentRouteIndexKeys(
	parentRefs []gatewayv1.ParentReference,
	defaultNamespace string,
) []string {
	values := make(map[string]struct{}, len(parentRefs)+1)
	for _, parentRef := range parentRefs {
		kind := "Service"
		if parentRef.Kind != nil {
			kind = string(*parentRef.Kind)
		}
		if kind != "Service" {
			continue
		}
		namespace := defaultNamespace
		if parentRef.Namespace != nil && string(*parentRef.Namespace) != "" {
			namespace = string(*parentRef.Namespace)
		}
		if namespace == "" || parentRef.Name == "" {
			continue
		}
		values["__service_parent__"] = struct{}{}
		values[namespace+"/"+string(parentRef.Name)] = struct{}{}
	}
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	return out
}
