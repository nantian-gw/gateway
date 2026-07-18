package translator

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"

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
	if _, err := xlator.Build(context.Background(), missingExperimentalGatewayCRDClient{Client: baseClient}); err != nil {
		t.Fatalf("Build returned error for missing experimental Gateway API CRDs: %v", err)
	}
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
		return noKindMatch("gateway.networking.k8s.io", "TCPRoute", "v1alpha2")
	case *gatewayv1alpha2.UDPRouteList:
		return noKindMatch("gateway.networking.k8s.io", "UDPRoute", "v1alpha2")
	case *gatewayv1alpha2.TLSRouteList:
		return noKindMatch("gateway.networking.k8s.io", "TLSRoute", "v1alpha2")
	case *gatewayv1.ListenerSetList:
		return noKindMatch("gateway.networking.k8s.io", "ListenerSet", "v1")
	}
	return c.Client.List(ctx, list, opts...)
}

func noKindMatch(group, kind, version string) error {
	return &meta.NoKindMatchError{
		GroupKind: schema.GroupKind{
			Group: group,
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
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}
	if len(snapshot.HTTPRoutes) != 1 {
		t.Fatalf("expected 1 http route, got %d", len(snapshot.HTTPRoutes))
	}
	if len(snapshot.Backends) != 1 {
		t.Fatalf("expected 1 backend, got %d", len(snapshot.Backends))
	}
	if got := snapshot.HTTPRoutes[0].Annotations["gateway.nantian.dev/access-log-mode"]; got != "json" {
		t.Fatalf("expected route annotation to be preserved, got %q", got)
	}
	if got := snapshot.Listeners[0].AttachedRoutes; len(got) != 1 || got[0] != "default/route" {
		t.Fatalf("unexpected attached routes: %#v", got)
	}
	if snapshot.Listeners[0].Status == nil || snapshot.Listeners[0].Status.Accepted == nil || snapshot.Listeners[0].Status.Accepted.Status != "True" {
		t.Fatalf("expected listener status summary, got %#v", snapshot.Listeners[0].Status)
	}
	if snapshot.Listeners[0].Status.ResolvedRefs == nil || snapshot.Listeners[0].Status.ResolvedRefs.Status != "True" || snapshot.Listeners[0].Status.ResolvedRefs.ObservedGeneration != 1 {
		t.Fatalf("expected listener resolved refs summary, got %#v", snapshot.Listeners[0].Status)
	}
	if snapshot.HTTPRoutes[0].Status == nil || len(snapshot.HTTPRoutes[0].Status.Parents) != 1 || snapshot.HTTPRoutes[0].Status.Parents[0].Accepted == nil || snapshot.HTTPRoutes[0].Status.Parents[0].Accepted.Status != "True" {
		t.Fatalf("expected route status summary, got %#v", snapshot.HTTPRoutes[0].Status)
	}

	parent := snapshot.HTTPRoutes[0].Status.Parents[0]
	if parent.ResolvedRefs == nil || parent.ResolvedRefs.Status != "True" || parent.ResolvedRefs.ObservedGeneration != 1 {
		t.Fatalf("expected route resolved refs summary, got %#v", parent)
	}
	partial := findCondition(parent.Conditions, string(gatewayv1.RouteConditionPartiallyInvalid))
	if partial == nil {
		t.Fatalf("expected partially invalid condition in %#v", parent.Conditions)
	}
	if partial.Status != "True" || partial.Reason != string(gatewayv1.RouteReasonUnsupportedValue) || partial.ObservedGeneration != 1 {
		t.Fatalf("unexpected partially invalid condition: %#v", partial)
	}
	if partial.Message != "Dropped Rule 1 because HTTPRoute rule 1 must not combine RequestRedirect and URLRewrite filters" {
		t.Fatalf("unexpected partially invalid message: %q", partial.Message)
	}
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
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 materialized listener, got %#v", snapshot.Listeners)
	}
	if snapshot.Listeners[0].Name != "default/gw/http" {
		t.Fatalf("unexpected listener name: %q", snapshot.Listeners[0].Name)
	}
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
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(snapshot.HTTPRoutes) != 1 {
		t.Fatalf("expected 1 HTTPRoute, got %d", len(snapshot.HTTPRoutes))
	}
	if len(snapshot.HTTPRoutes[0].Rules) != 1 {
		t.Fatalf("expected unsupported ExternalAuth protocol rule to be dropped, got %#v", snapshot.HTTPRoutes[0].Rules)
	}
	backendRefs := snapshot.HTTPRoutes[0].Rules[0].BackendRefs
	if len(backendRefs) != 1 || backendRefs[0].Name != "echo" {
		t.Fatalf("unexpected backend refs after dropping invalid rule: %#v", backendRefs)
	}
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

	translated := translateHTTPRoute(route)
	if len(translated.Hostnames) != 0 {
		t.Fatalf("expected empty hostnames to be preserved, got %#v", translated.Hostnames)
	}
	if len(translated.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %#v", translated.Rules)
	}
	if len(translated.Rules[0].Matches) != 1 {
		t.Fatalf("expected 1 explicit match, got %#v", translated.Rules[0].Matches)
	}

	match := translated.Rules[0].Matches[0]
	if match.Path != "/checkout" || match.PathType != "Exact" {
		t.Fatalf("unexpected path match: %#v", match)
	}
	if match.Method != "POST" {
		t.Fatalf("unexpected method match: %#v", match)
	}
	if len(match.Headers) != 1 || match.Headers[0].Name != "x-tenant" || match.Headers[0].Value != "blue" || match.Headers[0].MatchType != "Exact" {
		t.Fatalf("unexpected header matches: %#v", match.Headers)
	}
	if len(match.QueryParams) != 1 || match.QueryParams[0].Name != "debug" || match.QueryParams[0].Value != "false" || match.QueryParams[0].MatchType != "Exact" {
		t.Fatalf("unexpected query matches: %#v", match.QueryParams)
	}
	if len(translated.Rules[1].Matches) != 0 {
		t.Fatalf("expected default rule to keep empty matches, got %#v", translated.Rules[1].Matches)
	}
}

func TestFiltersFromHTTPHeaderModifiers(t *testing.T) {
	filters := filtersFromHTTP([]gatewayv1.HTTPRouteFilter{
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

	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}
	if filters[0].Type != "RequestHeaderModifier" {
		t.Fatalf("unexpected request filter type: %s", filters[0].Type)
	}
	if !reflect.DeepEqual(filters[0].Config["set"], []any{map[string]any{"name": "x-user", "value": "alice"}}) {
		t.Fatalf("unexpected request set config: %#v", filters[0].Config["set"])
	}
	if !reflect.DeepEqual(filters[0].Config["add"], []any{map[string]any{"name": "x-tag", "value": "blue"}}) {
		t.Fatalf("unexpected request add config: %#v", filters[0].Config["add"])
	}
	if !reflect.DeepEqual(filters[0].Config["remove"], []any{"x-remove"}) {
		t.Fatalf("unexpected request remove config: %#v", filters[0].Config["remove"])
	}
	if filters[1].Type != "ResponseHeaderModifier" {
		t.Fatalf("unexpected response filter type: %s", filters[1].Type)
	}
	if !reflect.DeepEqual(filters[1].Config["set"], []any{map[string]any{"name": "x-response", "value": "ok"}}) {
		t.Fatalf("unexpected response config: %#v", filters[1].Config["set"])
	}
}

func TestFiltersFromHTTPCORS(t *testing.T) {
	filters := filtersFromHTTPWithResolver(
		[]gatewayv1.HTTPRouteFilter{{
			Type: gatewayv1.HTTPRouteFilterType("CORS"),
		}},
		"default",
		extfilter.Resolver{},
		extfilter.TargetHTTP,
		rawHTTPRouteFilterConfigs{{
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

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Type != "CORS" {
		t.Fatalf("unexpected cors filter type: %s", filters[0].Type)
	}
	if !reflect.DeepEqual(filters[0].Config["allowOrigins"], []any{"https://app.example"}) {
		t.Fatalf("unexpected cors allowOrigins config: %#v", filters[0].Config["allowOrigins"])
	}
	if !reflect.DeepEqual(filters[0].Config["allowMethods"], []any{"GET", "POST"}) {
		t.Fatalf("unexpected cors allowMethods config: %#v", filters[0].Config["allowMethods"])
	}
	if !reflect.DeepEqual(filters[0].Config["allowHeaders"], []any{"authorization", "content-type"}) {
		t.Fatalf("unexpected cors allowHeaders config: %#v", filters[0].Config["allowHeaders"])
	}
	if !reflect.DeepEqual(filters[0].Config["exposeHeaders"], []any{"x-trace-id"}) {
		t.Fatalf("unexpected cors exposeHeaders config: %#v", filters[0].Config["exposeHeaders"])
	}
	if got := filters[0].Config["allowCredentials"]; got != true {
		t.Fatalf("unexpected cors allowCredentials config: %#v", got)
	}
	if got := filters[0].Config["maxAge"]; got != 600 {
		t.Fatalf("unexpected cors maxAge config: %#v", got)
	}
}

func TestFiltersFromHTTPExternalAuthHTTP(t *testing.T) {
	filters := filtersFromHTTP([]gatewayv1.HTTPRouteFilter{{
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

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Type != "ExternalAuth" {
		t.Fatalf("unexpected filter type: %s", filters[0].Type)
	}
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
	if !reflect.DeepEqual(filters[0].Config, want) {
		t.Fatalf("unexpected ExternalAuth config:\n got %#v\nwant %#v", filters[0].Config, want)
	}
}

func TestFiltersFromHTTPExternalAuthGRPC(t *testing.T) {
	filters := filtersFromHTTP([]gatewayv1.HTTPRouteFilter{{
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

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
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
	if !reflect.DeepEqual(filters[0].Config, want) {
		t.Fatalf("unexpected ExternalAuth GRPC config:\n got %#v\nwant %#v", filters[0].Config, want)
	}
}

func TestFiltersFromGRPCHeaderModifiers(t *testing.T) {
	filters := filtersFromGRPC([]gatewayv1.GRPCRouteFilter{
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

	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}
	if !reflect.DeepEqual(filters[0].Config["add"], []any{map[string]any{"name": "x-grpc", "value": "yes"}}) {
		t.Fatalf("unexpected grpc request config: %#v", filters[0].Config["add"])
	}
	if !reflect.DeepEqual(filters[1].Config["remove"], []any{"grpc-status-details-bin"}) {
		t.Fatalf("unexpected grpc response config: %#v", filters[1].Config["remove"])
	}
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

	filters := filtersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "maintenance",
		},
	}}, "default", resolver, extfilter.TargetHTTP, nil, 0)

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Type != extfilter.TypeExtensionRef {
		t.Fatalf("unexpected filter type: %s", filters[0].Type)
	}
	if got := filters[0].Config["extensionType"]; got != extfilter.TypeDirectResponse {
		t.Fatalf("unexpected extension type: %#v", got)
	}
	directResponse, ok := filters[0].Config["directResponse"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested direct response config, got %#v", filters[0].Config["directResponse"])
	}
	if got := directResponse["statusCode"]; got != 503 {
		t.Fatalf("unexpected status code: %#v", got)
	}
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

	filters := filtersFromGRPCWithResolver([]gatewayv1.GRPCRouteFilter{{
		Type: gatewayv1.GRPCRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "grpc-headers",
		},
	}}, "default", resolver, extfilter.TargetGRPC)

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Type != "ResponseHeaderModifier" {
		t.Fatalf("unexpected filter type: %s", filters[0].Type)
	}
	if !reflect.DeepEqual(filters[0].Config["set"], []any{
		map[string]any{"name": "grpc-status-details-bin", "value": "encoded"},
	}) {
		t.Fatalf("unexpected response header config: %#v", filters[0].Config["set"])
	}
}

func TestFiltersFromHTTPExtensionRefMissingConfigMap(t *testing.T) {
	filters := filtersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "missing",
		},
	}}, "default", extfilter.Resolver{}, extfilter.TargetHTTP, nil, 0)

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Type != extfilter.TypeExtensionRef {
		t.Fatalf("unexpected filter type: %s", filters[0].Type)
	}
	if got := filters[0].Config["resolved"]; got != false {
		t.Fatalf("expected unresolved extension ref, got %#v", got)
	}
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

	filters := filtersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "redirect",
		},
	}}, "default", resolver, extfilter.TargetHTTP, nil, 0)

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Type != string(gatewayv1.HTTPRouteFilterRequestRedirect) {
		t.Fatalf("unexpected filter type: %s", filters[0].Type)
	}
	if got := filters[0].Config["statusCode"]; got != 302 {
		t.Fatalf("unexpected status code: %#v", got)
	}
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

	filters := filtersFromHTTPWithResolver([]gatewayv1.HTTPRouteFilter{{
		Type: gatewayv1.HTTPRouteFilterExtensionRef,
		ExtensionRef: &gatewayv1.LocalObjectReference{
			Group: "",
			Kind:  "ConfigMap",
			Name:  "cors",
		},
	}}, "default", resolver, extfilter.TargetHTTP, nil, 0)

	if len(filters) != 1 {
		t.Fatalf("expected 1 filter, got %d", len(filters))
	}
	if filters[0].Type != "CORS" {
		t.Fatalf("unexpected filter type: %s", filters[0].Type)
	}
	if !reflect.DeepEqual(filters[0].Config["allowMethods"], []any{"GET", "POST"}) {
		t.Fatalf("unexpected allowMethods config: %#v", filters[0].Config["allowMethods"])
	}
	if got := filters[0].Config["maxAge"]; got != 600 {
		t.Fatalf("unexpected maxAge config: %#v", got)
	}
}

func TestFiltersFromHTTPAndGRPCRequestMirror(t *testing.T) {
	percent := int32(25)
	denominator := int32(1000)

	httpFilters := filtersFromHTTP([]gatewayv1.HTTPRouteFilter{
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

	grpcFilters := filtersFromGRPC([]gatewayv1.GRPCRouteFilter{
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

	if got := httpFilters[0].Config["backendRef"]; !reflect.DeepEqual(got, map[string]any{
		"namespace": "default",
		"name":      "shadow-http",
		"port":      8080,
	}) {
		t.Fatalf("unexpected http request mirror backend ref: %#v", got)
	}
	if got := httpFilters[0].Config["percent"]; got != 25 {
		t.Fatalf("unexpected http request mirror percent: %#v", got)
	}
	if got := grpcFilters[0].Config["backendRef"]; !reflect.DeepEqual(got, map[string]any{
		"namespace": "observability",
		"name":      "shadow-grpc",
		"port":      9090,
	}) {
		t.Fatalf("unexpected grpc request mirror backend ref: %#v", got)
	}
	if got := grpcFilters[0].Config["fraction"]; !reflect.DeepEqual(got, map[string]any{
		"numerator":   1,
		"denominator": 1000,
	}) {
		t.Fatalf("unexpected grpc request mirror fraction: %#v", got)
	}
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

	refs := backendRefsFromGRPC([]gatewayv1.GRPCBackendRef{ref}, "default")
	if len(refs) != 1 {
		t.Fatalf("expected 1 grpc backend ref, got %d", len(refs))
	}
	if len(refs[0].Filters) != 1 {
		t.Fatalf("expected backend filters to be translated, got %#v", refs[0].Filters)
	}
	if refs[0].Filters[0].Type != "RequestHeaderModifier" {
		t.Fatalf("unexpected grpc backend filter type: %s", refs[0].Filters[0].Type)
	}
	if !reflect.DeepEqual(refs[0].Filters[0].Config["set"], []any{
		map[string]any{"name": "x-backend", "value": "selected"},
	}) {
		t.Fatalf("unexpected grpc backend filter config: %#v", refs[0].Filters[0].Config)
	}
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

	translated := translateGRPCRoute(route)
	if len(translated.Rules) != 1 || len(translated.Rules[0].Matches) != 1 {
		t.Fatalf("unexpected translated gRPC matches: %#v", translated.Rules)
	}

	match := translated.Rules[0].Matches[0]
	if match.Service != "" {
		t.Fatalf("expected empty service for header-only match, got %q", match.Service)
	}
	if match.Method != "" {
		t.Fatalf("expected empty method for header-only match, got %q", match.Method)
	}
	if len(match.Headers) != 1 {
		t.Fatalf("expected 1 header match, got %#v", match.Headers)
	}
	if match.Headers[0].Name != "version" || match.Headers[0].Value != "one" {
		t.Fatalf("unexpected header match: %#v", match.Headers[0])
	}
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
