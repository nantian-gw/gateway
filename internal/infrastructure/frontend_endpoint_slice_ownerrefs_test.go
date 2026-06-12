package infrastructure

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDesiredFrontendEndpointSlicesSetServiceOwnerReference(t *testing.T) {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayServiceName("public"),
			Namespace: "default",
			UID:       types.UID("service-uid-123"),
			Labels: map[string]string{
				managedByLabel:   managedByValue,
				serviceRoleLabel: serviceRoleGateway,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "tcp-80",
				Port:       80,
				TargetPort: intstrFrom(80),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	pods := []corev1.Pod{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nantian-dataplane-0",
			Namespace: defaultDataplaneNamespace,
		},
		Status: corev1.PodStatus{
			PodIP: "10.0.0.50",
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}}

	slices := desiredFrontendEndpointSlices(service, pods, gatewayEndpointSliceRoleValue, gatewayEndpointSliceNamePrefix)
	if len(slices) != 1 {
		t.Fatalf("expected 1 endpoint slice, got %#v", slices)
	}
	if len(slices[0].OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %#v, want 1 service owner", slices[0].OwnerReferences)
	}
	owner := slices[0].OwnerReferences[0]
	if owner.APIVersion != "v1" || owner.Kind != "Service" || owner.Name != service.Name || owner.UID != service.UID {
		t.Fatalf("unexpected ownerReference %#v", owner)
	}
}

func TestReconcileBackfillsGatewayEndpointSliceOwnerReference(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	serviceName := gatewayServiceName("public")
	sliceName := frontendEndpointSliceName(
		gatewayEndpointSliceNamePrefix,
		"default",
		serviceName,
		discoveryv1.AddressTypeIPv4,
	)

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
					Name:      "public",
					Namespace: "default",
					UID:       types.UID("gateway-uid-123"),
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
					Name:      serviceName,
					Namespace: "default",
					UID:       types.UID("service-uid-456"),
					Labels: map[string]string{
						managedByLabel:        managedByValue,
						serviceRoleLabel:      serviceRoleGateway,
						gatewayNameLabel:      "public",
						gatewayNamespaceLabel: "default",
					},
					OwnerReferences: []metav1.OwnerReference{{
						APIVersion: gatewayv1.GroupVersion.String(),
						Kind:       "Gateway",
						Name:       "public",
						UID:        types.UID("gateway-uid-123"),
					}},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{{
						Name:       "tcp-80",
						Port:       80,
						TargetPort: intstrFrom(80),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sliceName,
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: serviceName,
						discoveryv1.LabelManagedBy:   managedByValue,
						serviceRoleLabel:             gatewayEndpointSliceRoleValue,
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.50"},
				}},
				Ports: []discoveryv1.EndpointPort{{
					Port: protocolInt32Ptr(80),
				}},
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

	endpointSlice := &discoveryv1.EndpointSlice{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: sliceName},
		endpointSlice,
	); err != nil {
		t.Fatalf("Get gateway EndpointSlice returned error: %v", err)
	}

	if len(endpointSlice.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %#v, want 1 service owner", endpointSlice.OwnerReferences)
	}
	owner := endpointSlice.OwnerReferences[0]
	if owner.APIVersion != "v1" || owner.Kind != "Service" || owner.Name != serviceName || owner.UID != types.UID("service-uid-456") {
		t.Fatalf("unexpected ownerReference %#v", owner)
	}
}

func protocolInt32Ptr(value int32) *int32 {
	return &value
}
