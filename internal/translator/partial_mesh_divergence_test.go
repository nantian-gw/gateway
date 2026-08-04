package translator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

// TestPartialMeshRebuildMatchesFullBuild asserts that the partial rebuild path
// (BuildRoutesForSnapshot, which feeds the conformance follow-up publish) produces
// byte-for-byte identical mesh IR to the full Build path for the same cluster
// input. Divergence here is the suspected root cause of the intermittent
// "InvalidBackendRefs" 500s observed in conformance: the full build derives mesh
// frontends/backends from raw API routes across all kinds, while the partial
// rebuild derives them from the stored IR snapshot, and the two can disagree.
func TestPartialMeshRebuildMatchesFullBuild(t *testing.T) {
	scheme := testutil.BuildSupportScheme(t)

	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Service{ // mesh parent service (the frontend)
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr("http"),
					}},
				},
			},
			&corev1.Service{ // backing service
				ObjectMeta: metav1.ObjectMeta{Name: "backend", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "backend"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        8080,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr("http"),
					}},
				},
			},
			&gatewayv1.HTTPRoute{ // mesh route: parent = Service (echo), backend = backend:8080
				ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "backend",
									Port: ptr(gatewayv1.PortNumber(8080)),
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	// Full build path.
	full, err := New(string(controllerName), logger).Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("full Build returned error: %v", err)
	}

	// Partial rebuild path: treat the full snapshot as the current stored IR and
	// re-synthesize just the mesh route. This triggers RebuildMeshServiceListeners.
	partial, err := New(string(controllerName), logger).BuildRoutesForSnapshot(
		context.Background(),
		cl,
		full,
		[]types.NamespacedName{{Namespace: "default", Name: "mesh"}},
		nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	// Compare mesh listeners only.
	if diff := diffMeshListeners(full, partial); diff != "" {
		t.Errorf("mesh listeners diverged between full build and partial rebuild:\n%s", diff)
	}
	if diff := diffMeshBackends(full, partial); diff != "" {
		t.Errorf("mesh backends diverged between full build and partial rebuild:\n%s", diff)
	}
	// Compare the mesh route IR itself.
	if diff := diffMeshHTTPRoute(full, partial, "default/mesh"); diff != "" {
		t.Errorf("mesh HTTPRoute diverged between full build and partial rebuild:\n%s", diff)
	}
}

// TestPartialMeshRebuildMatchesFullBuildSelfReferentialShadow reproduces the
// conformance mesh topology: a mesh route whose parent Service == backend
// Service, resolved through mesh shadow service substitution. The full build
// derives backend clusters from all filtered services via EffectiveBackendServices
// shadow substitution; the partial rebuild (route-scoped backend keys +
// expandMeshShadowBackendKeys) must produce the identical backend set.
func TestPartialMeshRebuildMatchesFullBuildSelfReferentialShadow(t *testing.T) {
	cl, ns, _, _ := buildSelfReferentialShadowCluster(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	full, err := New(string(controllerName), logger).Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("full Build returned error: %v", err)
	}

	partial, err := New(string(controllerName), logger).BuildRoutesForSnapshot(
		context.Background(),
		cl,
		full,
		[]types.NamespacedName{{Namespace: ns, Name: "mesh-split-v1"}},
		nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("BuildRoutesForSnapshot returned error: %v", err)
	}

	if diff := diffMeshListeners(full, partial); diff != "" {
		t.Errorf("mesh listeners diverged between full build and partial rebuild:\n%s", diff)
	}
	if diff := diffMeshBackends(full, partial); diff != "" {
		t.Errorf("mesh backends diverged between full build and partial rebuild:\n%s", diff)
	}
	if diff := diffMeshHTTPRoute(full, partial, ns+"/mesh-split-v1"); diff != "" {
		t.Errorf("mesh HTTPRoute diverged between full build and partial rebuild:\n%s", diff)
	}
}

// TestPartialMeshServiceDependencyScopeMatchesFullBuild replicates the
// controller's Service-dependency rebuild scope (Backends | RouteBackendRefs |
// MeshListeners) exactly as syncer_scope.go executes it: BuildBackendsForSnapshot
// -> RefreshBackendRefMetadataForBackends -> RebuildMeshServiceListeners. This is
// the follow-up path that repairs the intermittent InvalidBackendRefs in
// conformance, and it must reproduce the full build's mesh listeners, backends,
// and route backend refs byte-for-byte.
func TestPartialMeshServiceDependencyScopeMatchesFullBuild(t *testing.T) {
	cl, ns, echoName, _ := buildSelfReferentialShadowCluster(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	tr := New(string(controllerName), logger)

	full, err := tr.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("full Build returned error: %v", err)
	}

	// Controller scope snapshotBuildScopeBackends | snapshotBuildScopeRouteBackendRefs
	// | snapshotBuildScopeMeshListeners with the changed mesh Service key.
	next := full
	serviceKeys := []client.ObjectKey{{Namespace: ns, Name: echoName}}

	backends, err := tr.BuildBackendsForSnapshot(context.Background(), cl, next, serviceKeys, nil)
	if err != nil {
		t.Fatalf("BuildBackendsForSnapshot returned error: %v", err)
	}
	next = ApplyPartialSnapshot(next, backends, nil)

	httpRoutes, grpcRoutes, streamRoutes, err := tr.RefreshBackendRefMetadataForBackends(
		context.Background(), cl, next, serviceKeys, nil, nil,
	)
	if err != nil {
		t.Fatalf("RefreshBackendRefMetadataForBackends returned error: %v", err)
	}
	next = ApplyPartialSnapshot(next, nil, nil)
	next.HTTPRoutes = httpRoutes
	next.GRPCRoutes = grpcRoutes
	next.StreamRoutes = streamRoutes

	listeners, err := tr.RebuildMeshServiceListeners(context.Background(), cl, next)
	if err != nil {
		t.Fatalf("RebuildMeshServiceListeners returned error: %v", err)
	}
	next = ApplyPartialSnapshot(next, nil, listeners)

	if diff := diffMeshListeners(full, next); diff != "" {
		t.Errorf("mesh listeners diverged between full build and Service-dependency scope:\n%s", diff)
	}
	if diff := diffMeshBackends(full, next); diff != "" {
		t.Errorf("mesh backends diverged between full build and Service-dependency scope:\n%s", diff)
	}
	if diff := diffMeshHTTPRoute(full, next, ns+"/mesh-split-v1"); diff != "" {
		t.Errorf("mesh HTTPRoute diverged between full build and Service-dependency scope:\n%s", diff)
	}
}

// TestPartialMeshServiceDependencyScopeTwoServices reproduces the conformance
// mesh-ports topology faithfully: TWO self-referential mesh services (echo-v1,
// echo-v2), each simultaneously the mesh parent (frontend), the shadow backend
// source, and the route backend, each with its own shadow service. This exercises
// assignFrontendPorts across two service-port keys (collision resolution) and
// multi-service shadow substitution, and must produce byte-identical mesh IR
// between the full build and the controller's Service-dependency scope when BOTH
// mesh services change together. The full build derives frontend parent keys from
// all raw API routes; RebuildMeshServiceListeners derives them from the stored
// snapshot. A divergence in the key set causes a different port assignment for
// the same services -> different listener IR -> different digest -> dataplane
// revalidation against a mismatched subset.
func TestPartialMeshServiceDependencyScopeTwoServices(t *testing.T) {
	cl, ns, echoV1, echoV2 := buildTwoServiceMeshCluster(t)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	tr := New(string(controllerName), logger)

	full, err := tr.Build(context.Background(), cl)
	if err != nil {
		t.Fatalf("full Build returned error: %v", err)
	}

	// Controller scope snapshotBuildScopeBackends | snapshotBuildScopeRouteBackendRefs
	// | snapshotBuildScopeMeshListeners with BOTH changed mesh Service keys.
	next := full
	serviceKeys := []client.ObjectKey{
		{Namespace: ns, Name: echoV1},
		{Namespace: ns, Name: echoV2},
	}

	backends, err := tr.BuildBackendsForSnapshot(context.Background(), cl, next, serviceKeys, nil)
	if err != nil {
		t.Fatalf("BuildBackendsForSnapshot returned error: %v", err)
	}
	next = ApplyPartialSnapshot(next, backends, nil)

	httpRoutes, grpcRoutes, streamRoutes, err := tr.RefreshBackendRefMetadataForBackends(
		context.Background(), cl, next, serviceKeys, nil, nil,
	)
	if err != nil {
		t.Fatalf("RefreshBackendRefMetadataForBackends returned error: %v", err)
	}
	next = ApplyPartialSnapshot(next, nil, nil)
	next.HTTPRoutes = httpRoutes
	next.GRPCRoutes = grpcRoutes
	next.StreamRoutes = streamRoutes

	listeners, err := tr.RebuildMeshServiceListeners(context.Background(), cl, next)
	if err != nil {
		t.Fatalf("RebuildMeshServiceListeners returned error: %v", err)
	}
	next = ApplyPartialSnapshot(next, nil, listeners)

	if diff := diffMeshListeners(full, next); diff != "" {
		t.Errorf("2-service mesh listeners diverged between full build and Service-dependency scope:\n%s", diff)
	}
	if diff := diffMeshBackends(full, next); diff != "" {
		t.Errorf("2-service mesh backends diverged between full build and Service-dependency scope:\n%s", diff)
	}
	for _, rk := range []string{ns + "/mesh-split-v1", ns + "/mesh-split-v2"} {
		if diff := diffMeshHTTPRoute(full, next, rk); diff != "" {
			t.Errorf("2-service mesh HTTPRoute %s diverged:\n%s", rk, diff)
		}
	}
}

// buildTwoServiceMeshCluster returns the conformance mesh-ports topology: two
// self-referential shadow mesh services, each with its own shadow service and
// endpoint slice, referenced by mirror routes (v1 parentRef carries port 80,
// v2 parentRef has no port), matching mesh-ports.yaml.
// buildTwoServiceMeshCluster returns the conformance mesh-ports topology: two
// self-referential shadow mesh services, each with its own shadow service and
// endpoint slice, referenced by mirror routes (v1 parentRef carries port 80,
// v2 parentRef has no port), matching mesh-ports.yaml.
func buildTwoServiceMeshCluster(t *testing.T) (client.Client, string, string, string) {
	t.Helper()
	scheme := testutil.BuildSupportScheme(t)

	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)
	ns := "gateway-conformance-mesh"

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-v1",
					Namespace: ns,
					Annotations: map[string]string{
						mesh.ManagedServiceAnnotation: "true",
						mesh.ShadowServiceAnnotation:  "nantian-gw-shadow-echo-v1",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo-v1"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr("http"),
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-echo-v1",
					Namespace: ns,
					Labels: map[string]string{
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: ns,
						mesh.OriginalServiceNameLabel:      "echo-v1",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(18081),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-echo-v1-1",
					Namespace: ns,
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "nantian-gw-shadow-echo-v1",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](18081)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.21"},
				}},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-v2",
					Namespace: ns,
					Annotations: map[string]string{
						mesh.ManagedServiceAnnotation: "true",
						mesh.ShadowServiceAnnotation:  "nantian-gw-shadow-echo-v2",
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo-v2"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr("http"),
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-echo-v2",
					Namespace: ns,
					Labels: map[string]string{
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: ns,
						mesh.OriginalServiceNameLabel:      "echo-v2",
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(18082),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-gw-shadow-echo-v2-1",
					Namespace: ns,
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "nantian-gw-shadow-echo-v2",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](18082)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.22"},
				}},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "mesh-split-v1", Namespace: ns},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo-v1",
							Port: &servicePort,
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo-v1",
									Port: &servicePort,
								},
							},
						}},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "mesh-split-v2", Namespace: ns},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo-v2",
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo-v2",
									Port: &servicePort,
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	return cl, ns, "echo-v1", "echo-v2"
}

// buildSelfReferentialShadowCluster returns a translator client with the
// conformance mesh-frontend topology: an echo Service that is simultaneously the
// mesh parent (frontend), the shadow backend source, and the route backend.
func buildSelfReferentialShadowCluster(t *testing.T) (client.Client, string, string, string) {
	t.Helper()
	scheme := testutil.BuildSupportScheme(t)

	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	const (
		ns         = "apps"
		echoName   = "echo"
		shadowName = "nantian-gw-shadow-echo"
	)

	cl := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}},
			&corev1.Service{ // the mesh parent (frontend) service
				ObjectMeta: metav1.ObjectMeta{
					Name:      echoName,
					Namespace: ns,
					Annotations: map[string]string{
						mesh.ManagedServiceAnnotation: "true",
						mesh.ShadowServiceAnnotation:  shadowName,
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: ptr("http"),
					}},
				},
			},
			&corev1.Service{ // the shadow backend service
				ObjectMeta: metav1.ObjectMeta{
					Name:      shadowName,
					Namespace: ns,
					Labels: map[string]string{
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: ns,
						mesh.OriginalServiceNameLabel:      echoName,
					},
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       80,
						TargetPort: intstr.FromInt(18080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{ // endpoints for the shadow service
				ObjectMeta: metav1.ObjectMeta{
					Name:      shadowName + "-1",
					Namespace: ns,
					Labels: map[string]string{
						discoveryv1.LabelServiceName: shadowName,
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](18080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.20"},
				}},
			},
			&gatewayv1.HTTPRoute{ // self-referential mesh route: parent == backend == echo
				ObjectMeta: metav1.ObjectMeta{Name: "mesh-split-v1", Namespace: ns},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &servicePort,
								},
							},
						}},
					}},
				},
			},
		).
		Build()

	return cl, ns, echoName, shadowName
}

func meshListeners(s *ir.Snapshot) []ir.Listener {
	var out []ir.Listener
	for _, l := range s.Listeners {
		if l.Metadata[mesh.FrontendKindMetadataKey] == mesh.FrontendKindService {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func diffMeshListeners(a, b *ir.Snapshot) string {
	la, lb := meshListeners(a), meshListeners(b)
	if !reflect.DeepEqual(la, lb) {
		return "full:\n" + dumpMeshListeners(la) + "partial:\n" + dumpMeshListeners(lb)
	}
	return ""
}

func dumpMeshListeners(ls []ir.Listener) string {
	var s string
	for _, l := range ls {
		s += "  listener " + l.Name +
			" port=" + strconv.FormatUint(uint64(l.Port), 10) +
			" proto=" + l.Protocol +
			" attached=" + listAttached(l.AttachedRoutes) +
			" fport=" + l.Metadata[mesh.FrontendPortMetadataKey] + "\n"
	}
	return s
}

// meshBackends are the clusters referenced by any Service-parented (mesh) route
// in the snapshot. Backends are keyed by namespace + Metadata["service"], not by
// FrontendKindMetadataKey (which backends do not carry).
func meshBackends(s *ir.Snapshot) []ir.BackendCluster {
	want := make(map[string]struct{})
	for _, r := range s.HTTPRoutes {
		for _, rule := range r.Rules {
			for _, ref := range rule.BackendRefs {
				want[ref.Namespace+"/"+ref.Name] = struct{}{}
			}
		}
	}
	for _, r := range s.GRPCRoutes {
		for _, rule := range r.Rules {
			for _, ref := range rule.BackendRefs {
				want[ref.Namespace+"/"+ref.Name] = struct{}{}
			}
		}
	}
	for _, r := range s.StreamRoutes {
		for _, rule := range r.Rules {
			for _, ref := range rule.BackendRefs {
				want[ref.Namespace+"/"+ref.Name] = struct{}{}
			}
		}
	}

	var out []ir.BackendCluster
	for _, b := range s.Backends {
		name := b.Metadata["service"]
		if name == "" {
			name, _, _ = strings.Cut(b.Name, ":")
		}
		if _, ok := want[b.Namespace+"/"+name]; !ok {
			continue
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func diffMeshBackends(a, b *ir.Snapshot) string {
	ba, bb := meshBackends(a), meshBackends(b)
	if !reflect.DeepEqual(ba, bb) {
		return "full:\n" + dumpMeshBackends(ba) + "partial:\n" + dumpMeshBackends(bb)
	}
	return ""
}

func dumpMeshBackends(bs []ir.BackendCluster) string {
	var s string
	for _, b := range bs {
		s += "  backend " + b.Name +
			" ns=" + b.Namespace +
			" proto=" + b.Protocol +
			" endpoints=" + listEndpoints(b.Endpoints) +
			" meta=" + fmt.Sprintf("%v", b.Metadata) + "\n"
	}
	return s
}

func listEndpoints(endpoints []ir.BackendEndpoint) string {
	out := "["
	for i, e := range endpoints {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%s:%d(healthy=%v)", e.Address, e.Port, e.Healthy)
	}
	return out + "]"
}

func diffMeshHTTPRoute(a, b *ir.Snapshot, routeKey string) string {
	var ra, rb *ir.HTTPRoute
	for i := range a.HTTPRoutes {
		if a.HTTPRoutes[i].Namespace+"/"+a.HTTPRoutes[i].Name == routeKey {
			ra = &a.HTTPRoutes[i]
			break
		}
	}
	for i := range b.HTTPRoutes {
		if b.HTTPRoutes[i].Namespace+"/"+b.HTTPRoutes[i].Name == routeKey {
			rb = &b.HTTPRoutes[i]
			break
		}
	}
	if ra == nil || rb == nil {
		return fmt.Sprintf("route %s missing in full=%v partial=%v", routeKey, ra != nil, rb != nil)
	}
	if !apiequality.Semantic.DeepEqual(ra, rb) {
		return "full:\n" + fmt.Sprintf("%#v\n", *ra) + "partial:\n" + fmt.Sprintf("%#v\n", *rb)
	}
	return ""
}

func listAttached(routes []string) string {
	if len(routes) == 0 {
		return "[]"
	}
	out := "["
	for i, r := range routes {
		if i > 0 {
			out += ","
		}
		out += r
	}
	return out + "]"
}
