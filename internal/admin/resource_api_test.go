package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodeinfo"
)

func newTestServerWithResourceManager(t *testing.T, resources *ResourceManager) *Server {
	return newTestServerWithResourceManagerAndLogger(t, resources, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func newTestServerWithResourceManagerAndOptions(t *testing.T, resources *ResourceManager, opts Options) *Server {
	return newTestServerWithResourceManagerAndLoggerAndOptions(
		t,
		resources,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		opts,
	)
}

func newTestServerWithResourceManagerAndLogger(t *testing.T, resources *ResourceManager, logger *slog.Logger) *Server {
	return newTestServerWithResourceManagerAndLoggerAndOptions(t, resources, logger, Options{})
}

func newTestServerWithResourceManagerAndLoggerAndOptions(
	t *testing.T,
	resources *ResourceManager,
	logger *slog.Logger,
	opts Options,
) *Server {
	t.Helper()

	if !IsAuthConfigured(opts) {
		opts.BearerToken = testAuthToken
	}

	store := ir.NewSnapshotStore(logger)
	nodes := nodeinfo.NewRegistry(
		ir.NewNodeStatusStore(),
		nil,
		logger,
		nodeinfo.Options{PersistTimeout: time.Second},
	)
	server := NewServer(":0", store, nodes, resources, logger, opts)

	now := time.Unix(1_700_000_000, 0).UTC()
	store.Publish(&ir.Snapshot{
		ID:          "v1",
		GeneratedAt: now,
		Listeners: []ir.Listener{
			{Name: "web", Address: "0.0.0.0", Port: 80, Protocol: "HTTP", AttachedRoutes: []string{"default/web"}},
			{Name: "passthrough", Address: "0.0.0.0", Port: 443, Protocol: "TLS"},
		},
		HTTPRoutes: []ir.HTTPRoute{
			{
				Name:      "web",
				Namespace: "default",
				Rules: []ir.HTTPRule{{
					BackendRefs: []ir.BackendRef{{Name: "api", Port: 80}},
				}},
			},
		},
		Backends: []ir.BackendCluster{
			{
				Name:      "api:80",
				Namespace: "default",
				Protocol:  "HTTP",
				Endpoints: []ir.BackendEndpoint{
					{Address: "10.0.0.1", Port: 80, Healthy: true},
				},
				Metadata: map[string]string{"service": "api"},
			},
		},
	})

	nodes.Connect(context.Background(), "dp-1", "kind", []string{"*"}, now)
	nodes.ObserveReport(context.Background(), "dp-1", "v1", true, "ready", now)

	return server
}

func resourceManagerForTest(t *testing.T) *ResourceManager {
	return resourceManagerForTestWithLogger(t, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func resourceManagerForTestWithLogger(t *testing.T, logger *slog.Logger) *ResourceManager {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := gatewayv1.Install(scheme); err != nil {
		t.Fatalf("install gateway v1 scheme: %v", err)
	}
	if err := gatewayv1alpha2.Install(scheme); err != nil {
		t.Fatalf("install gateway v1alpha2 scheme: %v", err)
	}
	if err := backendlb.Install(scheme); err != nil {
		t.Fatalf("install backendlb v1alpha2 scheme: %v", err)
	}
	if err := gatewayv1alpha3.Install(scheme); err != nil {
		t.Fatalf("install gateway v1alpha3 scheme: %v", err)
	}
	if err := gatewayv1beta1.Install(scheme); err != nil {
		t.Fatalf("install gateway v1beta1 scheme: %v", err)
	}
	if err := mcsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("install mcs v1alpha1 scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("install core v1 scheme: %v", err)
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&gatewayv1.Gateway{
				TypeMeta:   metav1TypeMeta("gateway.networking.k8s.io/v1", "Gateway"),
				ObjectMeta: metav1ObjectMeta("default", "edge"),
			},
			&gatewayv1.GatewayClass{
				TypeMeta: metav1TypeMeta("gateway.networking.k8s.io/v1", "GatewayClass"),
				ObjectMeta: metav1.ObjectMeta{
					Name: "nantian-gw",
				},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("gateway.nantian.dev/controller"),
				},
			},
			&gatewayv1.HTTPRoute{
				TypeMeta:   metav1TypeMeta("gateway.networking.k8s.io/v1", "HTTPRoute"),
				ObjectMeta: metav1ObjectMeta("default", "web"),
			},
			&corev1.Service{
				TypeMeta:   metav1TypeMeta("v1", "Service"),
				ObjectMeta: metav1ObjectMeta("default", "orders"),
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(8080),
						},
						{
							Name:       "https",
							Port:       8443,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromString("https"),
						},
					},
				},
			},
			&corev1.Namespace{
				TypeMeta:   metav1TypeMeta("v1", "Namespace"),
				ObjectMeta: metav1.ObjectMeta{Name: "backend"},
			},
		).
		Build()

	return NewResourceManager(client, logger)
}

func topologyNodeIDs(nodes []TopologyNode) []string {
	out := make([]string, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node.ID)
	}
	return out
}

func topologyEdgeIDs(edges []TopologyEdge) []string {
	out := make([]string, 0, len(edges))
	for _, edge := range edges {
		out = append(out, edge.ID)
	}
	return out
}

type noMatchListClient struct {
	client.Client
}

func (c noMatchListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	switch list.(type) {
	case *backendlb.BackendLBPolicyList:
		return &meta.NoKindMatchError{
			GroupKind:        schema.GroupKind{Group: gatewayv1.GroupName, Kind: "BackendLBPolicy"},
			SearchedVersions: []string{"v1alpha2"},
		}
	default:
		return c.Client.List(ctx, list, opts...)
	}
}

type resourceErrorClient struct {
	client.Client
	createErr error
	updateErr error
	deleteErr error
	getErr    error
	listErr   error
}

func (c resourceErrorClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if c.createErr != nil {
		return c.createErr
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c resourceErrorClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if c.updateErr != nil {
		return c.updateErr
	}
	return c.Client.Update(ctx, obj, opts...)
}

func (c resourceErrorClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if c.deleteErr != nil {
		return c.deleteErr
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func (c resourceErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if c.getErr != nil {
		return c.getErr
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c resourceErrorClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if c.listErr != nil {
		return c.listErr
	}
	return c.Client.List(ctx, list, opts...)
}

type countingResourceClient struct {
	client.Client
	mu        sync.Mutex
	getCalls  int
	listCalls int
}

func (c *countingResourceClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	c.mu.Lock()
	c.getCalls++
	c.mu.Unlock()
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *countingResourceClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	c.mu.Lock()
	c.listCalls++
	c.mu.Unlock()
	return c.Client.List(ctx, list, opts...)
}

func (c *countingResourceClient) GetCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.getCalls
}

func (c *countingResourceClient) ListCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listCalls
}

func metav1TypeMeta(apiVersion, kind string) metav1.TypeMeta {
	return metav1.TypeMeta{APIVersion: apiVersion, Kind: kind}
}

func metav1ObjectMeta(namespace, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: namespace}
}

func createServiceForTest(t *testing.T, manager *ResourceManager, svc *corev1.Service) {
	t.Helper()

	if err := manager.client.Create(context.Background(), svc); err != nil {
		t.Fatalf("create service: %v", err)
	}
}

func serviceCatalogKeys(items []ServiceCatalogEntry) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Namespace+"/"+item.Name)
	}
	return out
}

func performRequestWithBody(
	t *testing.T,
	server *Server,
	method string,
	path string,
	body []byte,
	target any,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
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
