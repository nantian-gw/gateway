package translator

import (
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
	"github.com/nantian-gw/gateway/internal/translator/shared"
	"github.com/nantian-gw/gateway/internal/translator/backends"
)

func TestTranslateGRPCRoutePreservesRegexMethodMatchType(t *testing.T) {
	matchType := gatewayv1.GRPCMethodMatchRegularExpression

	route := gatewayv1.GRPCRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grpc-regex-matching",
			Namespace: "default",
		},
		Spec: gatewayv1.GRPCRouteSpec{
			Rules: []gatewayv1.GRPCRouteRule{{
				Matches: []gatewayv1.GRPCRouteMatch{{
					Method: &gatewayv1.GRPCMethodMatch{
						Type:    &matchType,
						Service: ptr("helloworld\\..+"),
						Method:  ptr("Say(H|G).*"),
					},
				}},
			}},
		},
	}

	translated := translateGRPCRoute(route)
	if len(translated.Rules) != 1 || len(translated.Rules[0].Matches) != 1 {
		t.Fatalf("unexpected translated gRPC matches: %#v", translated.Rules)
	}

	match := translated.Rules[0].Matches[0]
	if match.MatchType != string(gatewayv1.GRPCMethodMatchRegularExpression) {
		t.Fatalf("expected regex match type, got %q", match.MatchType)
	}
	if match.Service != "helloworld\\..+" {
		t.Fatalf("unexpected service match: %q", match.Service)
	}
	if match.Method != "Say(H|G).*" {
		t.Fatalf("unexpected method match: %q", match.Method)
	}
}

func TestTranslateGatewayListenersPreservesMultipleAddresses(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Addresses: []gatewayv1.GatewaySpecAddress{
				{
					Type:  ptr[gatewayv1.AddressType](gatewayv1.IPAddressType),
					Value: "192.0.2.10",
				},
				{
					Type:  ptr[gatewayv1.AddressType](gatewayv1.HostnameAddressType),
					Value: "gw.example.com",
				},
			},
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}

	listeners := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).
		translateGatewayListeners(gateway, nil, nil, nil)
	if len(listeners) != 1 {
		t.Fatalf("expected 1 listener, got %#v", listeners)
	}
	if listeners[0].Address != "192.0.2.10" {
		t.Fatalf("unexpected primary listener address: %q", listeners[0].Address)
	}
	if got := listeners[0].Addresses; len(got) != 2 || got[0] != "192.0.2.10" || got[1] != "gw.example.com" {
		t.Fatalf("unexpected listener addresses: %#v", got)
	}
	if got := listeners[0].Metadata[listenerAddressesMetadataKey]; got != "192.0.2.10,gw.example.com" {
		t.Fatalf("unexpected listener addresses metadata: %q", got)
	}
}

func TestTranslateGatewayListenersPublishesDisplayAddressesFromStatus(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gw",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
		Status: gatewayv1.GatewayStatus{
			Addresses: []gatewayv1.GatewayStatusAddress{{
				Type:  ptr[gatewayv1.AddressType](gatewayv1.IPAddressType),
				Value: "127.0.0.1",
			}},
		},
	}

	listeners := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).
		translateGatewayListeners(gateway, nil, nil, nil)
	if len(listeners) != 1 {
		t.Fatalf("expected 1 listener, got %#v", listeners)
	}
	if listeners[0].Address != "0.0.0.0" {
		t.Fatalf("unexpected primary listener address: %q", listeners[0].Address)
	}
	if got := listeners[0].Addresses; len(got) != 1 || got[0] != "0.0.0.0" {
		t.Fatalf("unexpected listener addresses: %#v", got)
	}
	if got := listeners[0].Metadata[listenerDisplayAddressesMetadataKey]; got != "127.0.0.1" {
		t.Fatalf("unexpected listener display addresses metadata: %q", got)
	}
}

func TestHTTPRouteTimeoutsParsesRequestAndBackendRequest(t *testing.T) {
	request := gatewayv1.Duration("12s")
	backendRequest := gatewayv1.Duration("3s")

	timeouts := shared.HTTPRouteTimeouts(&gatewayv1.HTTPRouteTimeouts{
		Request:        &request,
		BackendRequest: &backendRequest,
	})

	if timeouts == nil {
		t.Fatal("expected route timeouts to be translated")
	}
	if timeouts.Request == nil || *timeouts.Request != 12*time.Second {
		t.Fatalf("unexpected request timeout: %#v", timeouts.Request)
	}
	if timeouts.BackendRequest == nil || *timeouts.BackendRequest != 3*time.Second {
		t.Fatalf("unexpected backend request timeout: %#v", timeouts.BackendRequest)
	}
}

func TestHTTPRouteTimeoutsPreservesExplicitZeroDuration(t *testing.T) {
	request := gatewayv1.Duration("0s")

	timeouts := shared.HTTPRouteTimeouts(&gatewayv1.HTTPRouteTimeouts{
		Request: &request,
	})

	if timeouts == nil {
		t.Fatal("expected route timeouts to be translated")
	}
	if timeouts.Request == nil || *timeouts.Request != 0 {
		t.Fatalf("unexpected zero request timeout: %#v", timeouts.Request)
	}
	if timeouts.BackendRequest != nil {
		t.Fatalf("unexpected backend timeout: %#v", timeouts.BackendRequest)
	}
}

func TestHTTPRouteRetryParsesAttemptsCodesAndBackoff(t *testing.T) {
	attempts := 3
	backoff := gatewayv1.Duration("150ms")

	retry := httpRouteRetry(&gatewayv1.HTTPRouteRetry{
		Codes:    []gatewayv1.HTTPRouteRetryStatusCode{500, 503, 504},
		Attempts: &attempts,
		Backoff:  &backoff,
	})

	if retry == nil {
		t.Fatal("expected retry policy to be translated")
	}
	if !reflect.DeepEqual(retry.Codes, []uint32{500, 503, 504}) {
		t.Fatalf("unexpected retry codes: %#v", retry.Codes)
	}
	if retry.Attempts != 3 {
		t.Fatalf("unexpected retry attempts: %d", retry.Attempts)
	}
	if retry.Backoff == nil || *retry.Backoff != 150*time.Millisecond {
		t.Fatalf("unexpected retry backoff: %#v", retry.Backoff)
	}
}

func TestTranslateHTTPRouteDropsInvalidRulesMarkedPartiallyInvalid(t *testing.T) {
	redirectScheme := "https"
	rewriteHostname := gatewayv1.PreciseHostname("example.com")
	backendPort := gatewayv1.PortNumber(8080)

	route := gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orders",
			Namespace: "default",
		},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{
				{
					Filters: []gatewayv1.HTTPRouteFilter{
						{
							Type: gatewayv1.HTTPRouteFilterRequestRedirect,
							RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
								Scheme: &redirectScheme,
							},
						},
						{
							Type: gatewayv1.HTTPRouteFilterURLRewrite,
							URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
								Hostname: &rewriteHostname,
							},
						},
					},
				},
				{
					BackendRefs: []gatewayv1.HTTPBackendRef{{
						BackendRef: gatewayv1.BackendRef{
							BackendObjectReference: gatewayv1.BackendObjectReference{
								Name: "backend",
								Port: &backendPort,
							},
						},
					}},
				},
			},
		},
	}

	translated := translateHTTPRoute(route)
	if len(translated.Rules) != 1 {
		t.Fatalf("expected 1 translated rule after dropping invalid rule, got %#v", translated.Rules)
	}
	if len(translated.Rules[0].BackendRefs) != 1 {
		t.Fatalf("unexpected translated backend refs: %#v", translated.Rules[0].BackendRefs)
	}
	if translated.Rules[0].BackendRefs[0].Name != "backend" {
		t.Fatalf("unexpected translated backend ref: %#v", translated.Rules[0].BackendRefs[0])
	}
}

func TestBackendProtocolUsesAppProtocolHints(t *testing.T) {
	appH2C := "kubernetes.io/h2c"
	appWS := "kubernetes.io/ws"
	appGRPC := "grpc"
	services := []corev1.Service{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:        "h2c",
						Port:        8080,
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &appH2C,
					},
					{
						Name:        "grpc",
						Port:        9090,
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &appGRPC,
					},
					{
						Name:        "ws",
						Port:        8081,
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &appWS,
					},
					{
						Name:     "plain",
						Port:     8082,
						Protocol: corev1.ProtocolTCP,
					},
					{
						Name:     "udp",
						Port:     53,
						Protocol: corev1.ProtocolUDP,
					},
				},
			},
		},
	}

	backends := backends.TranslateBackends(services, nil, nil, nil, nil, nil, backends.DefaultConnectTimeout)
	if len(backends) != 5 {
		t.Fatalf("expected 5 backends, got %d", len(backends))
	}

	if backends[0].Protocol != "H2C" {
		t.Fatalf("expected H2C protocol, got %q", backends[0].Protocol)
	}
	if backends[1].Protocol != "GRPC" {
		t.Fatalf("expected GRPC protocol, got %q", backends[1].Protocol)
	}
	if backends[2].Protocol != "HTTP" {
		t.Fatalf("expected websocket appProtocol to map to HTTP, got %q", backends[2].Protocol)
	}
	if backends[3].Protocol != "TCP" {
		t.Fatalf("expected TCP protocol fallback without appProtocol, got %q", backends[3].Protocol)
	}
	if backends[4].Protocol != "UDP" {
		t.Fatalf("expected UDP protocol fallback, got %q", backends[4].Protocol)
	}
}

func TestTranslateBackendsIncludesServiceImports(t *testing.T) {
	appGRPCS := "grpcs"
	serviceImports := []mcsv1alpha1.ServiceImport{{
		ObjectMeta: metav1.ObjectMeta{Name: "payments", Namespace: "default"},
		Spec: mcsv1alpha1.ServiceImportSpec{
			Type: mcsv1alpha1.ClusterSetIP,
			Ports: []mcsv1alpha1.ServicePort{{
				Name:        "grpc",
				Port:        9443,
				Protocol:    corev1.ProtocolTCP,
				AppProtocol: &appGRPCS,
			}},
		},
	}}
	slices := []discoveryv1.EndpointSlice{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "payments-1",
			Namespace: "default",
			Labels: map[string]string{
				mcsv1alpha1.LabelServiceName: "payments",
			},
		},
		Ports: []discoveryv1.EndpointPort{{
			Name: ptr("grpc"),
			Port: ptr[int32](19443),
		}},
		Endpoints: []discoveryv1.Endpoint{{
			Addresses: []string{"10.0.0.44"},
		}},
	}}

	backends := backends.TranslateBackends(nil, serviceImports, slices, nil, nil, nil, backends.DefaultConnectTimeout)
	if len(backends) != 1 {
		t.Fatalf("expected 1 serviceimport backend, got %d", len(backends))
	}

	backend := backends[0]
	if backend.Name != "payments:9443" {
		t.Fatalf("unexpected backend name: %q", backend.Name)
	}
	if backend.Protocol != "GRPCS" {
		t.Fatalf("unexpected serviceimport app protocol mapping: %q", backend.Protocol)
	}
	if len(backend.Endpoints) != 1 || backend.Endpoints[0].Port != 19443 {
		t.Fatalf("unexpected serviceimport endpoints: %#v", backend.Endpoints)
	}
}

func TestTranslateBackendsUsesServiceImportAppProtocolHints(t *testing.T) {
	appH2C := "kubernetes.io/h2c"
	appWS := "kubernetes.io/ws"

	serviceImports := []mcsv1alpha1.ServiceImport{{
		ObjectMeta: metav1.ObjectMeta{Name: "orders", Namespace: "default"},
		Spec: mcsv1alpha1.ServiceImportSpec{
			Type: mcsv1alpha1.ClusterSetIP,
			Ports: []mcsv1alpha1.ServicePort{
				{
					Name:        "h2c",
					Port:        8080,
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: &appH2C,
				},
				{
					Name:        "ws",
					Port:        8081,
					Protocol:    corev1.ProtocolTCP,
					AppProtocol: &appWS,
				},
				{
					Name:     "plain",
					Port:     8082,
					Protocol: corev1.ProtocolTCP,
				},
			},
		},
	}}

	backends := backends.TranslateBackends(nil, serviceImports, nil, nil, nil, nil, backends.DefaultConnectTimeout)
	if len(backends) != 3 {
		t.Fatalf("expected 3 serviceimport backends, got %d", len(backends))
	}

	if backends[0].Protocol != "H2C" {
		t.Fatalf("expected H2C protocol, got %q", backends[0].Protocol)
	}
	if backends[1].Protocol != "HTTP" {
		t.Fatalf("expected websocket appProtocol to map to HTTP, got %q", backends[1].Protocol)
	}
	if backends[2].Protocol != "TCP" {
		t.Fatalf("expected TCP protocol fallback without appProtocol, got %q", backends[2].Protocol)
	}
}

func TestTranslateBackendsDoesNotInjectDefaultRequestTimeout(t *testing.T) {
	services := []corev1.Service{{
		ObjectMeta: metav1.ObjectMeta{Name: "mcp", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Port:     80,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}}
	serviceImports := []mcsv1alpha1.ServiceImport{{
		ObjectMeta: metav1.ObjectMeta{Name: "remote-mcp", Namespace: "default"},
		Spec: mcsv1alpha1.ServiceImportSpec{
			Type: mcsv1alpha1.ClusterSetIP,
			Ports: []mcsv1alpha1.ServicePort{{
				Name:     "http",
				Port:     80,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}}

	backends := backends.TranslateBackends(services, serviceImports, nil, nil, nil, nil, backends.DefaultConnectTimeout)
	if len(backends) != 2 {
		t.Fatalf("expected 2 backends, got %d", len(backends))
	}

	for _, backend := range backends {
		if backend.ConnectTimeout != 5*time.Second {
			t.Fatalf("backend %s connect timeout = %s, want 5s", backend.Name, backend.ConnectTimeout)
		}
		if backend.RequestTimeout != 0 {
			t.Fatalf("backend %s request timeout = %s, want no default timeout", backend.Name, backend.RequestTimeout)
		}
	}
}

func TestFiltersFromHTTPRedirectAndRewrite(t *testing.T) {
	redirectScheme := "https"
	redirectHost := gatewayv1.PreciseHostname("www.example.com")
	redirectCode := 301
	redirectPort := gatewayv1.PortNumber(8443)
	fullPath := "/moved"
	rewriteHost := gatewayv1.PreciseHostname("backend.internal")
	prefixPath := "/api"

	filters := filtersFromHTTP([]gatewayv1.HTTPRouteFilter{
		{
			Type: gatewayv1.HTTPRouteFilterRequestRedirect,
			RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
				Scheme:     &redirectScheme,
				Hostname:   &redirectHost,
				Port:       &redirectPort,
				StatusCode: &redirectCode,
				Path: &gatewayv1.HTTPPathModifier{
					Type:            gatewayv1.FullPathHTTPPathModifier,
					ReplaceFullPath: &fullPath,
				},
			},
		},
		{
			Type: gatewayv1.HTTPRouteFilterURLRewrite,
			URLRewrite: &gatewayv1.HTTPURLRewriteFilter{
				Hostname: &rewriteHost,
				Path: &gatewayv1.HTTPPathModifier{
					Type:               gatewayv1.PrefixMatchHTTPPathModifier,
					ReplacePrefixMatch: &prefixPath,
				},
			},
		},
	}, "default")

	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %d", len(filters))
	}
	if filters[0].Type != "RequestRedirect" {
		t.Fatalf("unexpected redirect filter type: %s", filters[0].Type)
	}
	if filters[0].Config["scheme"] != "https" {
		t.Fatalf("unexpected redirect scheme: %#v", filters[0].Config["scheme"])
	}
	if filters[0].Config["hostname"] != "www.example.com" {
		t.Fatalf("unexpected redirect hostname: %#v", filters[0].Config["hostname"])
	}
	if filters[0].Config["port"] != int(8443) {
		t.Fatalf("unexpected redirect port: %#v", filters[0].Config["port"])
	}
	if filters[0].Config["statusCode"] != 301 {
		t.Fatalf("unexpected redirect status: %#v", filters[0].Config["statusCode"])
	}
	if !reflect.DeepEqual(filters[0].Config["path"], map[string]any{
		"type":            "ReplaceFullPath",
		"replaceFullPath": "/moved",
	}) {
		t.Fatalf("unexpected redirect path config: %#v", filters[0].Config["path"])
	}

	if filters[1].Type != "URLRewrite" {
		t.Fatalf("unexpected rewrite filter type: %s", filters[1].Type)
	}
	if filters[1].Config["hostname"] != "backend.internal" {
		t.Fatalf("unexpected rewrite hostname: %#v", filters[1].Config["hostname"])
	}
	if !reflect.DeepEqual(filters[1].Config["path"], map[string]any{
		"type":               "ReplacePrefixMatch",
		"replacePrefixMatch": "/api",
	}) {
		t.Fatalf("unexpected rewrite path config: %#v", filters[1].Config["path"])
	}
}

func TestHTTPRequestRedirectPreservesGatewayAPIStatusCodes(t *testing.T) {
	redirectScheme := "https"
	for _, code := range []int{303, 307, 308} {
		code := code
		t.Run(fmt.Sprintf("status-%d", code), func(t *testing.T) {
			filters := filtersFromHTTP([]gatewayv1.HTTPRouteFilter{{
				Type: gatewayv1.HTTPRouteFilterRequestRedirect,
				RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
					Scheme:     &redirectScheme,
					StatusCode: &code,
				},
			}}, "default")

			if got := filters[0].Config["statusCode"]; got != code {
				t.Fatalf("statusCode = %#v, want %d", got, code)
			}
		})
	}
}

func TestRuleSessionPersistenceDefaultsToCookieAndStableName(t *testing.T) {
	policy := backends.RuleSessionPersistence("http", "default", "route", 1, &gatewayv1.SessionPersistence{})
	if policy == nil {
		t.Fatal("expected session persistence policy")
	}
	if policy.Type != "Cookie" {
		t.Fatalf("unexpected default session type: %s", policy.Type)
	}
	if policy.Cookie == nil || policy.Cookie.LifetimeType != "Session" {
		t.Fatalf("unexpected default cookie config: %#v", policy.Cookie)
	}
	if policy.SessionName != backends.DefaultRouteSessionName("http", "default", "route", 1) {
		t.Fatalf("unexpected default session name: %q", policy.SessionName)
	}
}

func TestRuleSessionPersistenceParsesHeaderAndTimeouts(t *testing.T) {
	absolute := gatewayv1.Duration("5m")
	idle := gatewayv1.Duration("30s")
	sessionType := gatewayv1.HeaderBasedSessionPersistence
	sessionName := "x-nantian-gw-session"

	policy := backends.RuleSessionPersistence("grpc", "default", "greeter", 0, &gatewayv1.SessionPersistence{
		SessionName:     &sessionName,
		AbsoluteTimeout: &absolute,
		IdleTimeout:     &idle,
		Type:            &sessionType,
	})
	if policy == nil {
		t.Fatal("expected session persistence policy")
	}
	if policy.Type != "Header" {
		t.Fatalf("unexpected session type: %s", policy.Type)
	}
	if policy.Cookie != nil {
		t.Fatalf("expected header-based persistence to omit cookie config, got %#v", policy.Cookie)
	}
	if policy.SessionName != "x-nantian-gw-session" {
		t.Fatalf("unexpected session name: %s", policy.SessionName)
	}
	if policy.AbsoluteTimeout == nil || *policy.AbsoluteTimeout != 5*time.Minute {
		t.Fatalf("unexpected absolute timeout: %#v", policy.AbsoluteTimeout)
	}
	if policy.IdleTimeout == nil || *policy.IdleTimeout != 30*time.Second {
		t.Fatalf("unexpected idle timeout: %#v", policy.IdleTimeout)
	}
}
