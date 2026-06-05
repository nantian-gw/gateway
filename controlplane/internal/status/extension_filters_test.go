package status

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/extensionfilter"
)

func TestEvaluateResolvedRefsAcceptsHTTPRouteExtensionRef(t *testing.T) {
	state := &clusterState{
		configMaps: []corev1.ConfigMap{{
			ObjectMeta: metav1.ObjectMeta{Name: "headers", Namespace: "default"},
			Data: map[string]string{
				extensionfilter.ConfigMapDataKey: `
type: RequestHeaderModifier
headerModifier:
  add:
    - name: x-region
      value: cn
`,
			},
		}},
	}

	result := evaluateResolvedRefs(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "headers",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionTrue {
		t.Fatalf("expected resolved refs true, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonResolvedRefs) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}

func TestEvaluateResolvedRefsRejectsMissingExtensionConfigMap(t *testing.T) {
	result := evaluateResolvedRefs(&clusterState{}, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "missing",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionFalse {
		t.Fatalf("expected resolved refs false, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonBackendNotFound) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}

func TestEvaluateResolvedRefsRejectsUnsupportedExtensionType(t *testing.T) {
	state := &clusterState{
		configMaps: []corev1.ConfigMap{{
			ObjectMeta: metav1.ObjectMeta{Name: "unsupported", Namespace: "default"},
			Data: map[string]string{
				extensionfilter.ConfigMapDataKey: `
type: LocalRateLimit
`,
			},
		}},
	}

	result := evaluateResolvedRefs(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "unsupported",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionFalse {
		t.Fatalf("expected resolved refs false, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}

func TestEvaluateResolvedRefsRejectsInvalidRequestMirrorExtensionRef(t *testing.T) {
	state := &clusterState{
		configMaps: []corev1.ConfigMap{{
			ObjectMeta: metav1.ObjectMeta{Name: "mirror", Namespace: "default"},
			Data: map[string]string{
				extensionfilter.ConfigMapDataKey: `
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
		}},
	}

	result := evaluateResolvedRefs(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "mirror",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionFalse {
		t.Fatalf("expected resolved refs false, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}

func TestEvaluateResolvedRefsRejectsInvalidRequestRedirectExtensionRef(t *testing.T) {
	state := &clusterState{
		configMaps: []corev1.ConfigMap{{
			ObjectMeta: metav1.ObjectMeta{Name: "redirect", Namespace: "default"},
			Data: map[string]string{
				extensionfilter.ConfigMapDataKey: `
type: RequestRedirect
requestRedirect:
  scheme: ftp
`,
			},
		}},
	}

	result := evaluateResolvedRefs(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "redirect",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionFalse {
		t.Fatalf("expected resolved refs false, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}

func TestEvaluateResolvedRefsAcceptsCORSExtensionRef(t *testing.T) {
	state := &clusterState{
		configMaps: []corev1.ConfigMap{{
			ObjectMeta: metav1.ObjectMeta{Name: "cors", Namespace: "default"},
			Data: map[string]string{
				extensionfilter.ConfigMapDataKey: `
type: CORS
cors:
  allowOrigins:
    - https://app.example
  allowMethods:
    - GET
`,
			},
		}},
	}

	result := evaluateResolvedRefs(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "cors",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionTrue {
		t.Fatalf("expected resolved refs true, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonResolvedRefs) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}

func TestEvaluateResolvedRefsRejectsInvalidHeaderModifierExtensionRef(t *testing.T) {
	state := &clusterState{
		configMaps: []corev1.ConfigMap{{
			ObjectMeta: metav1.ObjectMeta{Name: "headers", Namespace: "default"},
			Data: map[string]string{
				extensionfilter.ConfigMapDataKey: `
type: RequestHeaderModifier
headerModifier:
  remove:
    - ""
`,
			},
		}},
	}

	result := evaluateResolvedRefs(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "headers",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionFalse {
		t.Fatalf("expected resolved refs false, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}

func TestEvaluateResolvedRefsRejectsInvalidDirectResponseExtensionRef(t *testing.T) {
	state := &clusterState{
		configMaps: []corev1.ConfigMap{{
			ObjectMeta: metav1.ObjectMeta{Name: "maintenance", Namespace: "default"},
			Data: map[string]string{
				extensionfilter.ConfigMapDataKey: `
type: DirectResponse
directResponse:
  statusCode: 700
`,
			},
		}},
	}

	result := evaluateResolvedRefs(state, routeInput{
		kind:       routeKindHTTP,
		namespace:  "default",
		name:       "orders",
		generation: 1,
		extensionRefs: []extensionfilter.Ref{{
			Kind:      extensionfilter.ConfigMapKind,
			Namespace: "default",
			Name:      "maintenance",
		}},
	})

	if result.resolvedCondition.Status != metav1.ConditionFalse {
		t.Fatalf("expected resolved refs false, got %#v", result)
	}
	if result.resolvedCondition.Reason != string(gatewayv1.RouteReasonUnsupportedValue) {
		t.Fatalf("unexpected reason: %s", result.resolvedCondition.Reason)
	}
}
