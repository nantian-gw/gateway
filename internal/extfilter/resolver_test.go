package extfilter

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestResolveHeaderModifierExtensionRef(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "headers", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestHeaderModifier
headerModifier:
  add:
    - name: x-tenant
      value: blue
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "headers",
	}, TargetGRPC)
	if !result.Resolved {
		t.Fatalf("expected extension ref to resolve, got %#v", result)
	}
	if result.Type != string(gatewayv1.HTTPRouteFilterRequestHeaderModifier) {
		t.Fatalf("unexpected filter type: %s", result.Type)
	}
	if got := result.Config["add"]; !reflect.DeepEqual(got, []any{
		map[string]any{"name": "x-tenant", "value": "blue"},
	}) {
		t.Fatalf("unexpected add config: %#v", got)
	}
}

func TestResolveRejectsHeaderModifierExtensionRefWithEmptyHeaderName(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "headers", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestHeaderModifier
headerModifier:
  add:
    - name: ""
      value: blue
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "headers",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid header modifier to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveDirectResponseExtensionRef(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "maintenance", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: DirectResponse
directResponse:
  statusCode: 503
  contentType: text/plain; charset=utf-8
  body: maintenance
  headers:
    - name: retry-after
      value: "60"
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "maintenance",
	}, TargetHTTP)
	if !result.Resolved {
		t.Fatalf("expected direct response to resolve, got %#v", result)
	}
	if result.Type != TypeExtensionRef {
		t.Fatalf("unexpected filter type: %s", result.Type)
	}
	if got := result.Config["extensionType"]; got != TypeDirectResponse {
		t.Fatalf("unexpected extension type: %#v", got)
	}
	directResponse, ok := result.Config["directResponse"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested directResponse config, got %#v", result.Config["directResponse"])
	}
	if got := directResponse["statusCode"]; got != 503 {
		t.Fatalf("unexpected status code: %#v", got)
	}
}

func TestResolveRejectsDirectResponseExtensionRefWithInvalidStatusCode(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "maintenance", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: DirectResponse
directResponse:
  statusCode: 700
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "maintenance",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid direct response to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsDirectResponseExtensionRefWithInvalidHeaderName(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "maintenance", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: DirectResponse
directResponse:
  statusCode: 503
  headers:
    - name: "bad header"
      value: "1"
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "maintenance",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid direct response header to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsDirectResponseOnGRPC(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "maintenance", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: DirectResponse
directResponse:
  statusCode: 503
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "maintenance",
	}, TargetGRPC)
	if result.Resolved {
		t.Fatalf("expected grpc direct response to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRequestRedirectExtensionRef(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "redirect", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestRedirect
requestRedirect:
  scheme: https
  hostname: app.example.com
  statusCode: 302
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "redirect",
	}, TargetHTTP)
	if !result.Resolved {
		t.Fatalf("expected request redirect to resolve, got %#v", result)
	}
	if result.Type != string(gatewayv1.HTTPRouteFilterRequestRedirect) {
		t.Fatalf("unexpected filter type: %s", result.Type)
	}
	if got := result.Config["statusCode"]; got != 302 {
		t.Fatalf("unexpected status code: %#v", got)
	}
}

func TestResolveCORSExtensionRef(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "cors", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: CORS
cors:
  allowOrigins:
    - https://app.example
  allowMethods:
    - GET
    - POST
  allowHeaders:
    - authorization
    - content-type
  exposeHeaders:
    - x-trace-id
  allowCredentials: true
  maxAge: 600
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "cors",
	}, TargetHTTP)
	if !result.Resolved {
		t.Fatalf("expected cors extension ref to resolve, got %#v", result)
	}
	if result.Type != "CORS" {
		t.Fatalf("unexpected filter type: %s", result.Type)
	}
	if got := result.Config["allowMethods"]; !reflect.DeepEqual(got, []any{"GET", "POST"}) {
		t.Fatalf("unexpected allowMethods config: %#v", got)
	}
	if got := result.Config["maxAge"]; got != 600 {
		t.Fatalf("unexpected maxAge config: %#v", got)
	}
}

func TestResolveRejectsCORSExtensionRefOnGRPC(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "cors", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: CORS
cors:
  allowOrigins:
    - https://app.example
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "cors",
	}, TargetGRPC)
	if result.Resolved {
		t.Fatalf("expected grpc cors extension ref to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsCORSExtensionRefWithNegativeMaxAge(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "cors", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: CORS
cors:
  maxAge: -1
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "cors",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid cors extension ref to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsRequestRedirectExtensionRefWithInvalidScheme(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "redirect", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestRedirect
requestRedirect:
  scheme: ftp
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "redirect",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid request redirect to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsRequestRedirectExtensionRefWithInvalidStatusCode(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "redirect", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestRedirect
requestRedirect:
  statusCode: 308
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "redirect",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid request redirect to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsRequestRedirectExtensionRefWithInvalidPathModifier(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "redirect", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestRedirect
requestRedirect:
  path:
    type: ReplacePrefixMatch
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "redirect",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid request redirect path modifier to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRequestMirrorExtensionRef(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "mirror", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestMirror
requestMirror:
  backendRef:
    name: shadow
    port: 8080
  percent: 25
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "mirror",
	}, TargetGRPC)
	if !result.Resolved {
		t.Fatalf("expected request mirror to resolve, got %#v", result)
	}
	if result.Type != string(gatewayv1.HTTPRouteFilterRequestMirror) {
		t.Fatalf("unexpected filter type: %s", result.Type)
	}
	backendRef, ok := result.Config["backendRef"].(map[string]any)
	if !ok {
		t.Fatalf("expected backendRef config, got %#v", result.Config["backendRef"])
	}
	if got := backendRef["namespace"]; got != "default" {
		t.Fatalf("unexpected backend namespace: %#v", got)
	}
}

func TestResolveRejectsRequestMirrorExtensionRefWithoutBackendName(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "mirror", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestMirror
requestMirror:
  backendRef:
    port: 8080
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "mirror",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid request mirror extension to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsRequestMirrorExtensionRefWithBothPercentAndFraction(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "mirror", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestMirror
requestMirror:
  backendRef:
    name: shadow
    port: 8080
  percent: 25
  fraction:
    numerator: 1
    denominator: 2
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "mirror",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid request mirror extension to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsRequestMirrorExtensionRefWithUnsupportedBackendKind(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "mirror", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: RequestMirror
requestMirror:
  backendRef:
    group: example.com
    kind: Bucket
    name: shadow
    port: 8080
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "mirror",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid request mirror extension to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsMissingConfigMap(t *testing.T) {
	resolver := NewResolver(nil)
	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "missing",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected missing configmap to fail")
	}
	if result.Reason != string(gatewayv1.RouteReasonBackendNotFound) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}

func TestResolveRejectsURLRewriteExtensionRefWithInvalidPathModifier(t *testing.T) {
	resolver := NewResolver([]corev1.ConfigMap{{
		ObjectMeta: metav1.ObjectMeta{Name: "rewrite", Namespace: "default"},
		Data: map[string]string{
			ConfigMapDataKey: `
type: URLRewrite
urlRewrite:
  path:
    replaceFullPath: /members
`,
		},
	}})

	result := resolver.Resolve(Ref{
		Kind:      ConfigMapKind,
		Namespace: "default",
		Name:      "rewrite",
	}, TargetHTTP)
	if result.Resolved {
		t.Fatalf("expected invalid URL rewrite path modifier to be rejected, got %#v", result)
	}
	if result.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.Reason)
	}
}
