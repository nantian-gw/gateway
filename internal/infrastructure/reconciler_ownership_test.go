package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDesiredGatewayServiceIncludesOwnershipAndParameterAnnotations(t *testing.T) {
	externalTrafficPolicy := corev1.ServiceExternalTrafficPolicyLocal
	parametersRef := &gatewayv1.LocalParametersReference{
		Group: "",
		Kind:  gatewayv1.Kind("ConfigMap"),
		Name:  "public-gateway-infra",
	}

	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "public",
			Namespace:  "default",
			UID:        types.UID("gateway-uid-123"),
			Generation: 7,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
			Infrastructure: &gatewayv1.GatewayInfrastructure{
				Annotations: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
					"example.com/trace": "enabled",
				},
				ParametersRef: parametersRef,
			},
		},
	}
	params := gatewayServiceParameters{
		Type:                  corev1.ServiceTypeLoadBalancer,
		ExternalTrafficPolicy: &externalTrafficPolicy,
	}

	service := desiredGatewayService(&corev1.Service{}, gateway, params, "")

	if service.Annotations["example.com/trace"] != "enabled" {
		t.Fatalf("expected propagated gateway annotation, got %#v", service.Annotations)
	}
	if service.Annotations["nantian.dev/owner-kind"] != "Gateway" {
		t.Fatalf("owner-kind annotation = %q", service.Annotations["nantian.dev/owner-kind"])
	}
	if service.Annotations["nantian.dev/owner-namespace"] != "default" {
		t.Fatalf("owner-namespace annotation = %q", service.Annotations["nantian.dev/owner-namespace"])
	}
	if service.Annotations["nantian.dev/owner-name"] != "public" {
		t.Fatalf("owner-name annotation = %q", service.Annotations["nantian.dev/owner-name"])
	}
	if service.Annotations["nantian.dev/owner-uid"] != "gateway-uid-123" {
		t.Fatalf("owner-uid annotation = %q", service.Annotations["nantian.dev/owner-uid"])
	}
	if service.Annotations["nantian.dev/owner-generation"] != "7" {
		t.Fatalf("owner-generation annotation = %q", service.Annotations["nantian.dev/owner-generation"])
	}
	if service.Annotations["nantian.dev/gatewayclass-name"] != "nantian-gw" {
		t.Fatalf("gatewayclass-name annotation = %q", service.Annotations["nantian.dev/gatewayclass-name"])
	}
	if service.Annotations["nantian.dev/infrastructure-parameters-ref"] != "default/public-gateway-infra" {
		t.Fatalf(
			"infrastructure-parameters-ref annotation = %q",
			service.Annotations["nantian.dev/infrastructure-parameters-ref"],
		)
	}
	if service.Annotations["nantian.dev/service-parameters-hash"] != testGatewayServiceParametersHash(t, params) {
		t.Fatalf(
			"service-parameters-hash annotation = %q, want %q",
			service.Annotations["nantian.dev/service-parameters-hash"],
			testGatewayServiceParametersHash(t, params),
		)
	}
}

func TestReconcileRemovesStaleInfrastructureParameterAnnotationOnRollback(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "public",
					Namespace:  "default",
					UID:        types.UID("gateway-uid-456"),
					Generation: 5,
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
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayServiceName("public"),
					Namespace: "default",
					Labels: map[string]string{
						managedByLabel:        managedByValue,
						serviceRoleLabel:      serviceRoleGateway,
						gatewayNameLabel:      "public",
						gatewayNamespaceLabel: "default",
					},
					Annotations: map[string]string{
						"nantian.dev/infrastructure-parameters-ref": "default/stale-params",
						"nantian.dev/service-parameters-hash":       "stale",
						"nantian.dev/owner-generation":              "1",
						"example.com/trace":                         "stale",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	service, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("public")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}

	if _, ok := service.Annotations["nantian.dev/infrastructure-parameters-ref"]; ok {
		t.Fatalf("expected stale infrastructure-parameters-ref annotation to be removed, got %#v", service.Annotations)
	}
	if service.Annotations["nantian.dev/owner-generation"] != "5" {
		t.Fatalf("owner-generation annotation = %q", service.Annotations["nantian.dev/owner-generation"])
	}
	if service.Annotations["nantian.dev/owner-uid"] != "gateway-uid-456" {
		t.Fatalf("owner-uid annotation = %q", service.Annotations["nantian.dev/owner-uid"])
	}
	if service.Annotations["nantian.dev/service-parameters-hash"] != testGatewayServiceParametersHash(t, gatewayServiceParameters{}) {
		t.Fatalf("unexpected service-parameters-hash annotation %#v", service.Annotations)
	}
	if _, ok := service.Annotations["example.com/trace"]; ok {
		t.Fatalf("expected stale propagated annotation to be removed, got %#v", service.Annotations)
	}
}

func TestReconcilePropagatesOwnershipAnnotationsToGatewayEndpointSlices(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	classNamespace := gatewayv1.Namespace("infra-system")
	parametersRef := &gatewayv1.LocalParametersReference{
		Group: "",
		Kind:  gatewayv1.Kind("ConfigMap"),
		Name:  "public-gateway-infra",
	}

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
					ParametersRef: &gatewayv1.ParametersReference{
						Group:     "",
						Kind:      gatewayv1.Kind("ConfigMap"),
						Name:      "gatewayclass-defaults",
						Namespace: &classNamespace,
					},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gatewayclass-defaults",
					Namespace: "infra-system",
				},
				Data: map[string]string{
					serviceParametersYAMLKey: "publishNotReadyAddresses: true\n",
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public-gateway-infra",
					Namespace: "default",
				},
				Data: map[string]string{
					serviceParametersYAMLKey: "type: LoadBalancer\n",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "public",
					Namespace:  "default",
					UID:        types.UID("gateway-uid-789"),
					Generation: 3,
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: parametersRef,
					},
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
					Namespace: defaultDataplaneNamespace,
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
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	sliceName := frontendEndpointSliceName(
		gatewayEndpointSliceNamePrefix,
		"default",
		gatewayServiceName("public"),
		discoveryv1.AddressTypeIPv4,
	)
	endpointSlice := &discoveryv1.EndpointSlice{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: sliceName},
		endpointSlice,
	); err != nil {
		t.Fatalf("Get gateway EndpointSlice returned error: %v", err)
	}

	if endpointSlice.Annotations["nantian.dev/owner-kind"] != "Gateway" {
		t.Fatalf("owner-kind annotation = %q", endpointSlice.Annotations["nantian.dev/owner-kind"])
	}
	if endpointSlice.Annotations["nantian.dev/owner-name"] != "public" {
		t.Fatalf("owner-name annotation = %q", endpointSlice.Annotations["nantian.dev/owner-name"])
	}
	if endpointSlice.Annotations["nantian.dev/owner-generation"] != "3" {
		t.Fatalf("owner-generation annotation = %q", endpointSlice.Annotations["nantian.dev/owner-generation"])
	}
	if endpointSlice.Annotations["nantian.dev/infrastructure-parameters-ref"] != "default/public-gateway-infra" {
		t.Fatalf(
			"infrastructure-parameters-ref annotation = %q",
			endpointSlice.Annotations["nantian.dev/infrastructure-parameters-ref"],
		)
	}
	if endpointSlice.Annotations["nantian.dev/gatewayclass-parameters-ref"] != "infra-system/gatewayclass-defaults" {
		t.Fatalf(
			"gatewayclass-parameters-ref annotation = %q",
			endpointSlice.Annotations["nantian.dev/gatewayclass-parameters-ref"],
		)
	}
	if endpointSlice.Annotations["nantian.dev/service-parameters-hash"] != testGatewayServiceParametersHash(t, gatewayServiceParameters{
		Type:                     corev1.ServiceTypeLoadBalancer,
		PublishNotReadyAddresses: ptrBool(true),
	}) {
		t.Fatalf("unexpected service-parameters-hash annotation %#v", endpointSlice.Annotations)
	}
}

func ptrBool(value bool) *bool {
	return &value
}

func testGatewayServiceParametersHash(t *testing.T, params gatewayServiceParameters) string {
	t.Helper()
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
