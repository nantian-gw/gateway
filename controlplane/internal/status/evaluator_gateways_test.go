package status

import (
	"context"
	"testing"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/managedresources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestReconcileAcceptsTLSMixedTerminationListeners(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	terminateMode := gatewayv1.TLSModeTerminate
	passthroughMode := gatewayv1.TLSModePassthrough
	terminateHostname := gatewayv1.Hostname("tls.example.com")
	passthroughHostname := gatewayv1.Hostname("abc.example.com")
	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-tlsroute-mixed-termination")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway-tlsroute-mixed-termination", Namespace: "gateway-conformance-infra", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "tls-terminate",
							Protocol: gatewayv1.TLSProtocolType,
							Port:     8883,
							Hostname: &terminateHostname,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{
									From: ptr(gatewayv1.NamespacesFromSame),
								},
								Kinds: []gatewayv1.RouteGroupKind{{
									Kind: "TLSRoute",
								}},
							},
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &terminateMode,
								CertificateRefs: []gatewayv1.SecretObjectReference{{
									Name: "tls-terminate-checks-certificate",
								}},
							},
						},
						{
							Name:     "tls-passthrough",
							Protocol: gatewayv1.TLSProtocolType,
							Port:     8883,
							Hostname: &passthroughHostname,
							AllowedRoutes: &gatewayv1.AllowedRoutes{
								Namespaces: &gatewayv1.RouteNamespaces{
									From: ptr(gatewayv1.NamespacesFromSame),
								},
								Kinds: []gatewayv1.RouteGroupKind{{
									Kind: "TLSRoute",
								}},
							},
							TLS: &gatewayv1.ListenerTLSConfig{
								Mode: &passthroughMode,
							},
						},
					},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tls-terminate-checks-certificate",
					Namespace: "gateway-conformance-infra",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
			service,
			endpointSlice,
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gateway-tlsroute-mixed-termination"},
		&gateway,
	); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 2 {
		t.Fatalf("expected 2 listener statuses, got %d", len(gateway.Status.Listeners))
	}

	for _, listener := range gateway.Status.Listeners {
		if len(listener.SupportedKinds) != 1 || listener.SupportedKinds[0].Kind != "TLSRoute" {
			t.Fatalf("expected listener %s to support TLSRoute, got %#v", listener.Name, listener.SupportedKinds)
		}
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionAccepted), metav1.ConditionTrue, string(gatewayv1.ListenerReasonAccepted), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionResolvedRefs), metav1.ConditionTrue, string(gatewayv1.ListenerReasonResolvedRefs), 1)
		assertCondition(t, listener.Conditions, string(gatewayv1.ListenerConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.ListenerReasonProgrammed), 1)
		assertConditionAbsent(t, listener.Conditions, string(gatewayv1.ListenerConditionConflicted))
	}

	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 1)
	assertCondition(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed), metav1.ConditionTrue, string(gatewayv1.GatewayReasonProgrammed), 1)
}
