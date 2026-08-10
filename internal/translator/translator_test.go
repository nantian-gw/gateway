package translator

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/routes"
	"github.com/nantian-gw/gateway/internal/translator/testutil"
)

func TestBuildIgnoresMissingExperimentalGatewayRouteCRDs(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	baseClient := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(&gatewayv1.GatewayClass{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: controllerName,
			},
		}).
		Build()

	xlator := New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := xlator.Build(context.Background(), missingExperimentalGatewayCRDClient{Client: baseClient})
	require.NoError(t, err, "Build should not error for missing experimental Gateway API CRDs")
}

type missingExperimentalGatewayCRDClient struct {
	client.Client
}

func (c missingExperimentalGatewayCRDClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	switch list.(type) {
	case *gatewayv1alpha2.TCPRouteList:
		return noKindMatch("TCPRoute", "v1alpha2")
	case *gatewayv1alpha2.UDPRouteList:
		return noKindMatch("UDPRoute", "v1alpha2")
	case *gatewayv1alpha2.TLSRouteList:
		return noKindMatch("TLSRoute", "v1alpha2")
	case *gatewayv1.ListenerSetList:
		return noKindMatch("ListenerSet", "v1")
	}
	return c.Client.List(ctx, list, opts...)
}

func noKindMatch(kind, version string) error {
	return &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{
			Group: "gateway.networking.k8s.io",
			Kind:  kind,
		},
		SearchedVersions: []string{version},
	}
}

func TestBuildSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	pathType := gatewayv1.PathMatchPathPrefix
	hostname := gatewayv1.Hostname("example.com")
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
							Hostname: &hostname,
						},
					},
				},
				Status: gatewayv1.GatewayStatus{
					Listeners: []gatewayv1.ListenerStatus{
						{
							Name:           "http",
							AttachedRoutes: 1,
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionProgrammed),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionResolvedRefs),
									Status:             metav1.ConditionTrue,
									Reason:             string(gatewayv1.ListenerReasonResolvedRefs),
									Message:            "Listener references are resolved",
									ObservedGeneration: 1,
								},
							},
						},
					},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route",
					Namespace: "default",
					Annotations: map[string]string{
						"gateway.nantian.dev/access-log-mode": "json",
					},
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{
							{
								Name:        "gw",
								SectionName: ptr[gatewayv1.SectionName]("http"),
							},
						},
					},
					Hostnames: []gatewayv1.Hostname{hostname},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Matches: []gatewayv1.HTTPRouteMatch{
								{
									Path: &gatewayv1.HTTPPathMatch{
										Type:  &pathType,
										Value: ptr("/"),
									},
								},
							},
							BackendRefs: []gatewayv1.HTTPBackendRef{
								{
									BackendRef: gatewayv1.BackendRef{
										BackendObjectReference: gatewayv1.BackendObjectReference{
											Name: "echo",
											Port: &portNumber,
										},
									},
								},
							},
						},
					},
				},
				Status: gatewayv1.HTTPRouteStatus{
					RouteStatus: gatewayv1.RouteStatus{
						Parents: []gatewayv1.RouteParentStatus{
							{
								ControllerName: controllerName,
								ParentRef: gatewayv1.ParentReference{
									Name:        "gw",
									SectionName: ptr[gatewayv1.SectionName]("http"),
								},
								Conditions: []metav1.Condition{
									{
										Type:               string(gatewayv1.RouteConditionAccepted),
										Status:             metav1.ConditionTrue,
										ObservedGeneration: 1,
									},
									{
										Type:               string(gatewayv1.RouteConditionResolvedRefs),
										Status:             metav1.ConditionTrue,
										Reason:             string(gatewayv1.RouteReasonResolvedRefs),
										Message:            "Route references are resolved",
										ObservedGeneration: 1,
									},
									{
										Type:               string(gatewayv1.RouteConditionPartiallyInvalid),
										Status:             metav1.ConditionTrue,
										Reason:             string(gatewayv1.RouteReasonUnsupportedValue),
										Message:            "Dropped Rule 1 because HTTPRoute rule 1 must not combine RequestRedirect and URLRewrite filters",
										ObservedGeneration: 1,
									},
								},
							},
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "http",
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						},
					},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{
					{Port: ptr[int32](8080)},
				},
				Endpoints: []discoveryv1.Endpoint{
					{
						Addresses: []string{"10.0.0.10"},
					},
				},
			},
		).
		Build()

	xlator := New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil)))
	snapshot, err := xlator.Build(context.Background(), client)
	require.NoError(t, err, "Build should not error")

	require.Len(t, snapshot.Listeners, 1)
	require.Len(t, snapshot.HTTPRoutes, 1)
	require.Len(t, snapshot.Backends, 1)

	assert.Equal(t, "json", snapshot.HTTPRoutes[0].Annotations["gateway.nantian.dev/access-log-mode"], "route annotation should be preserved")
	require.Len(t, snapshot.Listeners[0].AttachedRoutes, 1)
	assert.Equal(t, "default/route", snapshot.Listeners[0].AttachedRoutes[0])

	require.NotNil(t, snapshot.Listeners[0].Status)
	require.NotNil(t, snapshot.Listeners[0].Status.Accepted)
	assert.Equal(t, "True", snapshot.Listeners[0].Status.Accepted.Status, "listener accepted status")
	require.NotNil(t, snapshot.Listeners[0].Status.ResolvedRefs)
	assert.Equal(t, "True", snapshot.Listeners[0].Status.ResolvedRefs.Status, "listener resolved refs status")
	assert.Equal(t, int64(1), snapshot.Listeners[0].Status.ResolvedRefs.ObservedGeneration)

	require.NotNil(t, snapshot.HTTPRoutes[0].Status)
	require.Len(t, snapshot.HTTPRoutes[0].Status.Parents, 1)
	parent := snapshot.HTTPRoutes[0].Status.Parents[0]
	require.NotNil(t, parent.Accepted)
	assert.Equal(t, "True", parent.Accepted.Status, "route accepted status")
	require.NotNil(t, parent.ResolvedRefs)
	assert.Equal(t, "True", parent.ResolvedRefs.Status, "route resolved refs status")
	assert.Equal(t, int64(1), parent.ResolvedRefs.ObservedGeneration)

	partial := findCondition(parent.Conditions, string(gatewayv1.RouteConditionPartiallyInvalid))
	require.NotNil(t, partial, "expected partially invalid condition")
	assert.Equal(t, "True", partial.Status)
	assert.Equal(t, string(gatewayv1.RouteReasonUnsupportedValue), partial.Reason)
	assert.Equal(t, int64(1), partial.ObservedGeneration)
	assert.Equal(t, "Dropped Rule 1 because HTTPRoute rule 1 must not combine RequestRedirect and URLRewrite filters", partial.Message)
}

func TestBuildSnapshotSkipsInvalidGatewayListeners(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
						},
						{
							Name:     "broken-https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
						},
					},
				},
				Status: gatewayv1.GatewayStatus{
					Listeners: []gatewayv1.ListenerStatus{
						{
							Name: "http",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionResolvedRefs),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionProgrammed),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
							},
						},
						{
							Name: "broken-https",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayv1.ListenerConditionAccepted),
									Status:             metav1.ConditionFalse,
									Reason:             string(gatewayv1.ListenerReasonInvalid),
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionProgrammed),
									Status:             metav1.ConditionFalse,
									Reason:             string(gatewayv1.ListenerReasonInvalid),
									ObservedGeneration: 1,
								},
							},
						},
					},
				},
			},
		).
		Build()

	snapshot, err := New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	require.NoError(t, err, "Build should not error")

	require.Len(t, snapshot.Listeners, 1, "expected 1 materialized listener")
	assert.Equal(t, "default/gw/http", snapshot.Listeners[0].Name)
}

func TestBuildSnapshotDropsHTTPRuleWithUnsupportedExternalAuthProtocol(t *testing.T) {
	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)

	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	portNumber := gatewayv1.PortNumber(8080)

	client := testutil.NewTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{Name: "gw"}},
					},
					Rules: []gatewayv1.HTTPRouteRule{
						{
							Filters: []gatewayv1.HTTPRouteFilter{{
								Type: gatewayv1.HTTPRouteFilterExternalAuth,
								ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
									ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthProtocol("CUSTOM"),
									BackendRef: gatewayv1.BackendObjectReference{
										Name: "auth",
										Port: &portNumber,
									},
								},
							}},
						},
						{
							BackendRefs: []gatewayv1.HTTPBackendRef{{
								BackendRef: gatewayv1.BackendRef{
									BackendObjectReference: gatewayv1.BackendObjectReference{
										Name: "echo",
										Port: &portNumber,
									},
								},
							}},
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
		).
		Build()

	snapshot, err := New(string(controllerName), slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), client)
	require.NoError(t, err, "Build should not error")

	require.Len(t, snapshot.HTTPRoutes, 1)
	require.Len(t, snapshot.HTTPRoutes[0].Rules, 1, "expected unsupported ExternalAuth protocol rule to be dropped")

	backendRefs := snapshot.HTTPRoutes[0].Rules[0].BackendRefs
	require.Len(t, backendRefs, 1)
	assert.Equal(t, "echo", backendRefs[0].Name)
}

func TestTranslateHTTPRoutePreservesExtendedMatchesAndDefaultRule(t *testing.T) {
	method := gatewayv1.HTTPMethodPost
	pathType := gatewayv1.PathMatchExact
	queryType := gatewayv1.QueryParamMatchExact
	headerType := gatewayv1.HeaderMatchExact

	route := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "checkout",
			Namespace: "default",
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Matches: []gatewayv1.HTTPRouteMatch{{
						Path: &gatewayv1.HTTPPathMatch{
							Type:  &pathType,
							Value: ptr("/checkout"),
						},
						Method: &method,
						Headers: []gatewayv1.HTTPHeaderMatch{{
							Name:  "x-tenant",
							Value: "blue",
							Type:  &headerType,
						}},
						QueryParams: []gatewayv1.HTTPQueryParamMatch{{
							Name:  "debug",
							Value: "false",
							Type:  &queryType,
						}},
					}},
				},
				{
					Matches: nil,
				},
			},
		},
	}

	translated := routes.TranslateHTTPRoute(route)
	require.Len(t, translated.Hostnames, 0, "expected empty hostnames to be preserved")
	require.Len(t, translated.Rules, 2)
	require.Len(t, translated.Rules[0].Matches, 1)

	match := translated.Rules[0].Matches[0]
	assert.Equal(t, "/checkout", match.Path)
	assert.Equal(t, "Exact", match.PathType)
	assert.Equal(t, "POST", match.Method)

	require.Len(t, match.Headers, 1)
	assert.Equal(t, "x-tenant", match.Headers[0].Name)
	assert.Equal(t, "blue", match.Headers[0].Value)
	assert.Equal(t, "Exact", match.Headers[0].MatchType)

	require.Len(t, match.QueryParams, 1)
	assert.Equal(t, "debug", match.QueryParams[0].Name)
	assert.Equal(t, "false", match.QueryParams[0].Value)
	assert.Equal(t, "Exact", match.QueryParams[0].MatchType)

	require.Len(t, translated.Rules[1].Matches, 0, "expected default rule to keep empty matches")
}

func TestFiltersFromHTTPHeaderModifiers(t *testing.T) {
	filters := routes.FiltersFromHTTP([]gatewayv1.HTTPRouteFilter{
		{
			Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
				Set: []gatewayv1.HTTPHeader{
					{Name: "x-user", Value: "alice"},
				},
				Add: []gatewayv1.HTTPHeader{
					{Name: "x-tag", Value: "blue"},
				},
				Remove: []string{"x-remove"},
			},
		},
		{
			Type: gatewayv1.HTTPRouteFilterResponseHeaderModifier,
			ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
				Set: []gatewayv1.HTTPHeader{
					{Name: "x-response", Value: "ok"},
				},
			},
		},
	}, "default")

	require.Len(t, filters, 2)
	assert.Equal(t, "RequestHeaderModifier", filters[0].Type)
	assert.Equal(t, []any{map[string]any{"name": "x-user", "value": "alice"}}, filters[0].Config["set"])
	assert.Equal(t, []any{map[string]any{"name": "x-tag", "value": "blue"}}, filters[0].Config["add"])
	assert.Equal(t, []any{"x-remove"}, filters[0].Config["remove"])
	assert.Equal(t, "ResponseHeaderModifier", filters[1].Type)
	assert.Equal(t, []any{map[string]any{"name": "x-response", "value": "ok"}}, filters[1].Config["set"])
}

func TestFiltersFromHTTPCORS(t *testing.T) {
	filters := routes.FiltersFromHTTPWithResolver(
		[]gatewayv1.HTTPRouteFilter{{
			Type: gatewayv1.HTTPRouteFilterType("CORS"),
		}},
		"default",
		extfilter.Resolver{},
		extfilter.TargetHTTP,
		routes.RawHTTPRouteFilterConfigs{{
			{
				"type": "CORS",
				"cors": map[string]any{
					"allowOrigins":     []any{"https://app.example"},
					"allowMethods":     []any{"GET", "POST"},
					"allowHeaders":     []any{"authorization", "content-type"},
					"exposeHeaders":    []any{"x-trace-id"},
					"allowCredentials": true,
					"maxAge":           600,
				},
			},
		}},
		0,
	)

	require.Len(t, filters, 1)
	assert.Equal(t, "CORS", filters[0].Type)
	assert.Equal(t, []any{"https://app.example"}, filters[0].Config["allowOrigins"])
	assert.Equal(t, []any{"GET", "POST"}, filters[0].Config["allowMethods"])
	assert.Equal(t, []any{"authorization", "content-type"}, filters[0].Config["allowHeaders"])
	assert.Equal(t, []any{"x-trace-id"}, filters[0].Config["exposeHeaders"])
	assert.Equal(t, true, filters[0].Config["allowCredentials"])
	assert.Equal(t, 600, filters[0].Config["maxAge"])
}

func TestFiltersFromHTTPExternalAuthHTTP(t *testing.T) {
	filters := routes.FiltersFromHTTP([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExternalAuth,
		ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
			ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthHTTPProtocol,
			BackendRef: gatewayv1.BackendObjectReference{
				Name:      "auth-service",
				Namespace: ptr(gatewayv1.Namespace("security")),
				Port:      ptr(gatewayv1.PortNumber(9000)),
			},
			HTTPAuthConfig: &gatewayv1.HTTPAuthConfig{
				Path:                   "/check",
				AllowedRequestHeaders:  []string{"authorization", "x-tenant"},
				AllowedResponseHeaders: []string{"x-user", "x-scope"},
			},
			ForwardBody: &gatewayv1.ForwardBodyConfig{MaxSize: 4096},
		},
	}}, "default")

	require.Len(t, filters, 1)
	assert.Equal(t, "ExternalAuth", filters[0].Type)
	want := map[string]any{
		"protocol": "HTTP",
		"backendRef": map[string]any{
			"namespace": "security",
			"name":      "auth-service",
			"port":      9000,
		},
		"http": map[string]any{
			"path":                   "/check",
			"allowedHeaders":         []any{"authorization", "x-tenant"},
			"allowedResponseHeaders": []any{"x-user", "x-scope"},
		},
		"forwardBodyMaxSize": 4096,
	}
	assert.Equal(t, want, filters[0].Config)
}

func TestFiltersFromHTTPExternalAuthGRPC(t *testing.T) {
	filters := routes.FiltersFromHTTP([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExternalAuth,
		ExternalAuth: &gatewayv1.HTTPExternalAuthFilter{
			ExternalAuthProtocol: gatewayv1.HTTPRouteExternalAuthGRPCProtocol,
			BackendRef: gatewayv1.BackendObjectReference{
				Name: "grpc-auth",
				Port: ptr(gatewayv1.PortNumber(9000)),
			},
			GRPCAuthConfig: &gatewayv1.GRPCAuthConfig{
				AllowedRequestHeaders: []string{"authorization", "x-tenant"},
			},
			ForwardBody: &gatewayv1.ForwardBodyConfig{MaxSize: 8192},
		},
	}}, "default")

	require.Len(t, filters, 1)
	want := map[string]any{
		"protocol": "GRPC",
		"backendRef": map[string]any{
			"namespace": "default",
			"name":      "grpc-auth",
			"port":      9000,
		},
		"grpc": map[string]any{
			"allowedHeaders": []any{"authorization", "x-tenant"},
		},
		"forwardBodyMaxSize": 8192,
	}
	assert.Equal(t, want, filters[0].Config)
}

func TestFiltersFromGRPCHeaderModifiers(t *testing.T) {
	filters := routes.FiltersFromGRPC([]gatewayv1.GRPCRouteFilter{
		{
			Type: gatewayv1.GRPCRouteFilterRequestHeaderModifier,
			RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
				Add: []gatewayv1.HTTPHeader{
					{Name: "x-grpc", Value: "yes"},
				},
			},
		},
		{
			Type: gatewayv1.GRPCRouteFilterResponseHeaderModifier,
			ResponseHeaderModifier: &gatewayv1.HTTPHeaderFilter{
				Remove: []string{"grpc-status-details-bin"},
			},
		},
	}, "default")

	require.Len(t, filters, 2)
	assert.Equal(t, []any{map[string]any{"name": "x-grpc", "value": "yes"}}, filters[0].Config["add"])
	assert.Equal(t, []any{"grpc-status-details-bin"}, filters[1].Config["remove"])
}

func TestFiltersFromHTTPExtensionRefDirectResponse(t *testing.T) {
	resolver := extfilter.NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "maintenance", Namespace: "default"},
		Data: map[string]string{
			extfilter.ConfigMapDataKey: `
type: DirectResponse
directResponse:
  statusCode: 503
  body: maintenance
  contentType: text/plain
`,
		},
	}})

	filters := routes.FiltersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "maintenance",
		},
	}}, "default", resolver, extfilter.TargetHTTP, nil, 0)

	require.Len(t, filters, 1)
	assert.Equal(t, extfilter.TypeExtensionRef, filters[0].Type)
	assert.Equal(t, extfilter.TypeDirectResponse, filters[0].Config["extensionType"])

	directResponse, ok := filters[0].Config["directResponse"].(map[string]any)
	require.True(t, ok, "expected nested direct response config")
	assert.Equal(t, 503, directResponse["statusCode"])
}

func TestFiltersFromGRPCExtensionRefHeaderModifier(t *testing.T) {
	resolver := extfilter.NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "grpc-headers", Namespace: "default"},
		Data: map[string]string{
			extfilter.ConfigMapDataKey: `
type: ResponseHeaderModifier
headerModifier:
  set:
    - name: grpc-status-details-bin
      value: encoded
`,
		},
	}})

	filters := routes.FiltersFromGRPCWithResolver([]gatewayv1.GRPCRouteFilter{{
		Type: gatewayv1.GRPCRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "grpc-headers",
		},
	}}, "default", resolver, extfilter.TargetGRPC)

	require.Len(t, filters, 1)
	assert.Equal(t, "ResponseHeaderModifier", filters[0].Type)
	assert.Equal(t, []any{
		map[string]any{"name": "grpc-status-details-bin", "value": "encoded"},
	}, filters[0].Config["set"])
}

func TestFiltersFromHTTPExtensionRefMissingConfigMap(t *testing.T) {
	filters := routes.FiltersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "missing",
		},
	}}, "default", extfilter.Resolver{}, extfilter.TargetHTTP, nil, 0)

	require.Len(t, filters, 1)
	assert.Equal(t, extfilter.TypeExtensionRef, filters[0].Type)
	assert.Equal(t, false, filters[0].Config["resolved"], "expected unresolved extension ref")
}

func TestFiltersFromHTTPExtensionRefRequestRedirect(t *testing.T) {
	resolver := extfilter.NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "redirect", Namespace: "default"},
		Data: map[string]string{
			extfilter.ConfigMapDataKey: `
type: RequestRedirect
requestRedirect:
  scheme: https
  hostname: app.example.com
  statusCode: 302
`,
		},
	}})

	filters := routes.FiltersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "redirect",
		},
	}}, "default", resolver, extfilter.TargetHTTP, nil, 0)

	require.Len(t, filters, 1)
	assert.Equal(t, string(gatewayv1.HTTPRouteFilterRequestRedirect), filters[0].Type)
	assert.Equal(t, 302, filters[0].Config["statusCode"])
}

func TestFiltersFromHTTPExtensionRefCORS(t *testing.T) {
	resolver := extfilter.NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "cors", Namespace: "default"},
		Data: map[string]string{
			extfilter.ConfigMapDataKey: `
type: CORS
cors:
  allowOrigins:
    - https://app.example
  allowMethods:
    - GET
    - POST
  maxAge: 600
`,
		},
	}})

	filters := routes.FiltersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "cors",
		},
	}}, "default", resolver, extfilter.TargetHTTP, nil, 0)

	require.Len(t, filters, 1)
	assert.Equal(t, "CORS", filters[0].Type)
	assert.Equal(t, []any{"GET", "POST"}, filters[0].Config["allowMethods"])
	assert.Equal(t, 600, filters[0].Config["maxAge"])
}

func TestFiltersFromHTTPAndGRPCRequestMirror(t *testing.T) {
	percent := int32(25)
	denominator := int32(1000)

	httpFilters := routes.FiltersFromHTTP([]gatewayv1.HTTPRouteFilter{
		{
			Type: gatewayv1.HTTPRouteFilterRequestMirror,
			RequestMirror: &gatewayv1.HTTPRequestMirrorFilter{
				BackendRef: gatewayv1.BackendObjectReference{
					Name: "shadow-http",
					Port: ptr(gatewayv1.PortNumber(8080)),
				},
				Percent: &percent,
			},
		},
	}, "default")

	grpcFilters := routes.FiltersFromGRPC([]gatewayv1.GRPCRouteFilter{
		{
			Type: gatewayv1.GRPCRouteFilterRequestMirror,
			RequestMirror: &gatewayv1.HTTPRequestMirrorFilter{
				BackendRef: gatewayv1.BackendObjectReference{
					Name:      "shadow-grpc",
					Namespace: ptr(gatewayv1.Namespace("observability")),
					Port:      ptr(gatewayv1.PortNumber(9090)),
				},
				Fraction: &gatewayv1.Fraction{
					Numerator:   1,
					Denominator: &denominator,
				},
			},
		},
	}, "default")

	assert.Equal(t, map[string]any{
		"namespace": "default",
		"name":      "shadow-http",
		"port":      8080,
	}, httpFilters[0].Config["backendRef"])
	assert.Equal(t, 25, httpFilters[0].Config["percent"])
	assert.Equal(t, map[string]any{
		"namespace": "observability",
		"name":      "shadow-grpc",
		"port":      9090,
	}, grpcFilters[0].Config["backendRef"])
	assert.Equal(t, map[string]any{
		"numerator":   1,
		"denominator": 1000,
	}, grpcFilters[0].Config["fraction"])
}

func TestBackendRefsFromGRPCIncludesBackendFilters(t *testing.T) {
	ref := gatewayv1.GRPCBackendRef{
		BackendRef: gatewayv1.BackendRef{
			BackendObjectReference: gatewayv1.BackendObjectReference{
				Name: "grpc-backend",
				Port: ptr(gatewayv1.PortNumber(9443)),
			},
		},
		Filters: []gatewayv1.GRPCRouteFilter{
			{
				Type: gatewayv1.GRPCRouteFilterRequestHeaderModifier,
				RequestHeaderModifier: &gatewayv1.HTTPHeaderFilter{
					Set: []gatewayv1.HTTPHeader{
						{Name: "x-backend", Value: "selected"},
					},
				},
			},
		},
	}

	refs := routes.BackendRefsFromGRPC([]gatewayv1.GRPCBackendRef{ref}, "default")
	require.Len(t, refs, 1)
	require.Len(t, refs[0].Filters, 1, "expected backend filters to be translated")
	assert.Equal(t, "RequestHeaderModifier", refs[0].Filters[0].Type)
	assert.Equal(t, []any{
		map[string]any{"name": "x-backend", "value": "selected"},
	}, refs[0].Filters[0].Config["set"])
}

func TestTranslateGRPCRouteAllowsHeaderOnlyMatches(t *testing.T) {
	route := gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grpc-header-matching",
			Namespace: "default",
		},
		Spec: gatewayv1.GRPCRouteSpec{
			CommonRouteSpec: gatewayv1.CommonRouteSpec{
				ParentRefs: []gatewayv1.ParentReference{{
					Name: "gw",
				}},
			},
			Rules: []gatewayv1.GRPCRouteRule{{
				Matches: []gatewayv1.GRPCRouteMatch{{
					Headers: []gatewayv1.GRPCHeaderMatch{{
						Name:  "version",
						Value: "one",
					}},
				}},
			}},
		},
	}

	translated := routes.TranslateGRPCRoute(route)
	require.Len(t, translated.Rules, 1)
	require.Len(t, translated.Rules[0].Matches, 1)

	match := translated.Rules[0].Matches[0]
	assert.Equal(t, "", match.Service, "expected empty service for header-only match")
	assert.Equal(t, "", match.Method, "expected empty method for header-only match")
	require.Len(t, match.Headers, 1)
	assert.Equal(t, "version", match.Headers[0].Name)
	assert.Equal(t, "one", match.Headers[0].Value)
}

func must(err error, t *testing.T) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func findCondition(conditions []ir.ConditionStatus, target string) *ir.ConditionStatus {
	for i := range conditions {
		if conditions[i].Type != target {
			continue
		}
		return &conditions[i]
	}
	return nil
}

func ptr[T any](value T) *T {
	return &value
}