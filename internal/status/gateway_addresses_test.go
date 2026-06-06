package status

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/infrastructure"
	"github.com/nantian-gw/gateway/internal/managedresources"
)

const (
	staticAddressGatewayUID = types.UID("gateway-static-addresses-uid")
	staticAddressServiceUID = types.UID("gateway-static-addresses-service-uid")
)

func TestReconcileGatewayStaticAddressesRejectsUnsupportedAddressType(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{
				{
					Type:  addressTypePtr("test/fake-invalid-type"),
					Value: "fake address teehee!",
				},
				{
					Type:  addressTypePtr(gatewayv1.IPAddressType),
					Value: "203.0.113.13",
				},
				{
					Type:  addressTypePtr(gatewayv1.IPAddressType),
					Value: "127.0.0.1",
				},
			}),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonUnsupportedAddress),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalid),
		1,
	)
	if len(gateway.Status.Addresses) != 0 {
		t.Fatalf("expected no status addresses for unsupported gateway address type, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesMarksUnusableAddress(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{
				{
					Type:  addressTypePtr(gatewayv1.IPAddressType),
					Value: "203.0.113.13",
				},
				{
					Type:  addressTypePtr(gatewayv1.IPAddressType),
					Value: "127.0.0.1",
				},
			}),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonAccepted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonAddressNotUsable),
		1,
	)
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("expected only the usable static address in status, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesRejectsInvalidHostnameValue(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Type:  addressTypePtr(gatewayv1.HostnameAddressType),
				Value: "Bad Hostname",
			}}),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonUnsupportedAddress),
		1,
	)
}

func TestReconcileGatewayStaticAddressesPublishesUsableAddress(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend)

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Type:  addressTypePtr(gatewayv1.IPAddressType),
				Value: "127.0.0.1",
			}}),
			service,
			endpointSlice,
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonAccepted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
	if len(gateway.Status.Addresses) != 1 {
		t.Fatalf("expected 1 status address, got %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("unexpected status address value: %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Type == nil || *gateway.Status.Addresses[0].Type != gatewayv1.IPAddressType {
		t.Fatalf("unexpected status address type: %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesAcceptsMultipleAdvertisedAddresses(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend)

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{
				{
					Type:  addressTypePtr(gatewayv1.IPAddressType),
					Value: "127.0.0.1",
				},
				{
					Type:  addressTypePtr(gatewayv1.IPAddressType),
					Value: "127.0.0.2",
				},
			}),
			service,
			endpointSlice,
		).
		Build()

	reconcileGatewayAddressesWithAdvertised(
		t,
		k8sClient,
		controllerName,
		[]string{"127.0.0.1", "127.0.0.2"},
	)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
	if len(gateway.Status.Addresses) != 2 {
		t.Fatalf("expected 2 status addresses, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesDeduplicatesNormalizedHostnames(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{
				{
					Type:  addressTypePtr(gatewayv1.HostnameAddressType),
					Value: "GW.EXAMPLE.COM",
				},
				{
					Type:  addressTypePtr(gatewayv1.HostnameAddressType),
					Value: "gw.example.com.",
				},
			}),
			gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses"),
		).
		Build()

	reconcileGatewayAddressesWithAdvertised(t, k8sClient, controllerName, []string{"gw.example.com"})

	gateway := getStaticAddressGateway(t, k8sClient)
	if len(gateway.Status.Addresses) != 1 {
		t.Fatalf("expected 1 normalized hostname address, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesAssignsDefaultIPAddressForEmptyValue(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend)

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Value: "",
			}}),
			service,
			endpointSlice,
		).
		Build()

	reconcileGatewayAddressesWithAdvertised(t, k8sClient, controllerName, []string{"127.0.0.1"})

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
	if len(gateway.Status.Addresses) != 1 {
		t.Fatalf("expected 1 assigned status address, got %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Type == nil || *gateway.Status.Addresses[0].Type != gatewayv1.IPAddressType {
		t.Fatalf("unexpected assigned address type: %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("unexpected assigned address value: %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesAssignsHostnameForEmptyHostnameValue(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses")
	endpointSlice := gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend)

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Type: addressTypePtr(gatewayv1.HostnameAddressType),
			}}),
			service,
			endpointSlice,
		).
		Build()

	reconcileGatewayAddressesWithAdvertised(t, k8sClient, controllerName, []string{"gw.example.com"})

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
	if len(gateway.Status.Addresses) != 1 {
		t.Fatalf("expected 1 assigned status address, got %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Type == nil || *gateway.Status.Addresses[0].Type != gatewayv1.HostnameAddressType {
		t.Fatalf("unexpected assigned address type: %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Value != "gw.example.com" {
		t.Fatalf("unexpected assigned address value: %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesMarksEmptyHostnameValueUnassigned(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Type: addressTypePtr(gatewayv1.HostnameAddressType),
			}}),
			gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses"),
		).
		Build()

	reconcileGatewayAddressesWithAdvertised(t, k8sClient, controllerName, []string{"127.0.0.1"})

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonAccepted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonAddressNotAssigned),
		1,
	)
	if len(gateway.Status.Addresses) != 0 {
		t.Fatalf("expected no assigned hostname addresses, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStatusPrefersGatewayServiceLoadBalancerIngress(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses")
	service.Spec.Type = corev1.ServiceTypeLoadBalancer
	service.Status = corev1.ServiceStatus{
		LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{
				{IP: "203.0.113.10"},
				{Hostname: "gw.example.com"},
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			service,
			gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
	if len(gateway.Status.Addresses) != 2 {
		t.Fatalf("expected 2 service-derived status addresses, got %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Value != "203.0.113.10" || gateway.Status.Addresses[1].Value != "gw.example.com" {
		t.Fatalf("unexpected status addresses: %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressesAcceptGatewayServiceExternalIP(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses")
	service.Spec.Type = corev1.ServiceTypeNodePort
	service.Spec.ExternalIPs = []string{"10.10.10.25"}

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Type:  addressTypePtr(gatewayv1.IPAddressType),
				Value: "10.10.10.25",
			}}),
			service,
			gatewayInfrastructureEndpointSliceForService(service, managedresources.EndpointSliceRoleGatewayFrontend),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "10.10.10.25" {
		t.Fatalf("expected externalIP-derived status address, got %#v", gateway.Status.Addresses)
	}
}

func TestServiceAdvertisedAddressesCanonicalizesDualStackAndHostnameValues(t *testing.T) {
	service := corev1.Service{
		Spec: corev1.ServiceSpec{
			ExternalIPs: []string{
				"2001:db8::10",
				"198.51.100.10",
				"198.51.100.10",
			},
			LoadBalancerIP: "198.51.100.10",
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{Hostname: "GW.EXAMPLE.COM."},
					{IP: "2001:db8::10"},
					{IP: "198.51.100.10"},
				},
			},
		},
	}

	got := serviceAdvertisedAddresses(service)
	want := []string{"198.51.100.10", "2001:db8::10", "gw.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("serviceAdvertisedAddresses() = %#v, want %#v", got, want)
	}
}

func TestBuildStatusAddressesCanonicalizesPublishedValues(t *testing.T) {
	got := buildStatusAddresses([]string{
		"GW.EXAMPLE.COM.",
		"gw.example.com",
		"2001:0db8::10",
		"2001:db8::10",
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 canonical addresses, got %#v", got)
	}
	if got[0].Type == nil || *got[0].Type != gatewayv1.IPAddressType || got[0].Value != "2001:db8::10" {
		t.Fatalf("unexpected canonical IP address: %#v", got[0])
	}
	if got[1].Type == nil || *got[1].Type != gatewayv1.HostnameAddressType || got[1].Value != "gw.example.com" {
		t.Fatalf("unexpected canonical hostname address: %#v", got[1])
	}
}

func TestReconcileGatewayStatusWaitsForDerivedService(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonAccepted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
}

func TestReconcileGatewayStatusWaitsForDerivedServiceMetadataConvergence(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("gateway-static-addresses"),
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":           "someone-else",
						"nantian.dev/service-role":                    "gateway-metadata",
						"gateway.networking.k8s.io/gateway-name": "gateway-static-addresses",
						"nantian.dev/gateway-namespace":               "gateway-conformance-infra",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed)); message != "Waiting for derived Gateway Service metadata to converge" {
		t.Fatalf("unexpected programmed message: %q", message)
	}
}

func TestReconcileGatewayStatusFallsBackToGlobalAddressesUntilDerivedServiceMetadataConverges(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("gateway-static-addresses"),
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":           "someone-else",
						"nantian.dev/service-role":                    "gateway-metadata",
						"gateway.networking.k8s.io/gateway-name": "gateway-static-addresses",
						"nantian.dev/gateway-namespace":               "gateway-conformance-infra",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:        corev1.ServiceTypeLoadBalancer,
					ExternalIPs: []string{"198.51.100.25"},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}},
					},
				},
			},
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if len(gateway.Status.Addresses) != 1 {
		t.Fatalf("expected global fallback address while metadata is drifting, got %#v", gateway.Status.Addresses)
	}
	if gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("expected global fallback address 127.0.0.1, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStatusWaitsForDerivedServiceOwnershipConvergence(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("gateway-static-addresses"),
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":           "nantian-gw",
						"nantian.dev/service-role":                    "gateway-metadata",
						"gateway.networking.k8s.io/gateway-name": "gateway-static-addresses",
						"nantian.dev/gateway-namespace":               "gateway-conformance-infra",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:        corev1.ServiceTypeLoadBalancer,
					ExternalIPs: []string{"198.51.100.25"},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}},
					},
				},
			},
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed)); message != "Waiting for derived Gateway Service metadata to converge" {
		t.Fatalf("unexpected programmed message: %q", message)
	}
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("expected global fallback address while ownership is drifting, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStatusWaitsForGatewayClassParametersReferenceConvergence(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	classNamespace := gatewayv1.Namespace("infra-system")

	service := gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses")
	service.Spec.Type = corev1.ServiceTypeLoadBalancer
	service.Status = corev1.ServiceStatus{
		LoadBalancer: corev1.LoadBalancerStatus{
			Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}},
		},
	}
	service.Annotations["nantian.dev/gatewayclass-parameters-ref"] = "infra-system/old-defaults"

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: string(classNamespace)}},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gatewayclass-defaults",
					Namespace: string(classNamespace),
				},
				Data: map[string]string{
					"service.yaml": "",
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
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
			staticAddressGateway(nil),
			service,
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed)); message != "Waiting for derived Gateway Service metadata to converge" {
		t.Fatalf("unexpected programmed message: %q", message)
	}
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("expected global fallback address while gatewayClass parameters ref is drifting, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStaticAddressRemainsPendingWhileDerivedServiceMetadataConverges(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Type:  addressTypePtr(gatewayv1.IPAddressType),
				Value: "10.10.10.25",
			}}),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("gateway-static-addresses"),
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":           "someone-else",
						"nantian.dev/service-role":                    "gateway-metadata",
						"gateway.networking.k8s.io/gateway-name": "gateway-static-addresses",
						"nantian.dev/gateway-namespace":               "gateway-conformance-infra",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:        corev1.ServiceTypeLoadBalancer,
					ExternalIPs: []string{"198.51.100.25"},
				},
			},
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed)); message != "Waiting for derived Gateway Service metadata to converge" {
		t.Fatalf("unexpected programmed message: %q", message)
	}
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "10.10.10.25" {
		t.Fatalf("expected explicit static address to remain in status, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayAssignedIPAddressFallsBackToPublishedAddressWhileDerivedServiceMetadataConverges(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Value: "",
			}}),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("gateway-static-addresses"),
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":           "someone-else",
						"nantian.dev/service-role":                    "gateway-metadata",
						"gateway.networking.k8s.io/gateway-name": "gateway-static-addresses",
						"nantian.dev/gateway-namespace":               "gateway-conformance-infra",
					},
				},
				Spec: corev1.ServiceSpec{
					Type:        corev1.ServiceTypeLoadBalancer,
					ExternalIPs: []string{"198.51.100.25"},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}},
					},
				},
			},
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "127.0.0.1" {
		t.Fatalf("expected assigned IP address to fall back to published global address, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayAssignedHostnameFallsBackToPublishedAddressWhileDerivedServiceMetadataConverges(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway([]gatewayv1.GatewaySpecAddress{{
				Type: addressTypePtr(gatewayv1.HostnameAddressType),
			}}),
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("gateway-static-addresses"),
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						"app.kubernetes.io/managed-by":           "someone-else",
						"nantian.dev/service-role":                    "gateway-metadata",
						"gateway.networking.k8s.io/gateway-name": "gateway-static-addresses",
						"nantian.dev/gateway-namespace":               "gateway-conformance-infra",
					},
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{{Hostname: "old.example.com"}},
					},
				},
			},
		).
		Build()

	reconcileGatewayAddressesWithAdvertised(t, k8sClient, controllerName, []string{"gw.example.com"})

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if len(gateway.Status.Addresses) != 1 || gateway.Status.Addresses[0].Value != "gw.example.com" {
		t.Fatalf("expected assigned hostname to fall back to published global address, got %#v", gateway.Status.Addresses)
	}
}

func TestReconcileGatewayStatusWaitsForDerivedFrontendEndpointSliceCreation(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses"),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed)); message != "Waiting for derived Gateway frontend EndpointSlices to converge" {
		t.Fatalf("unexpected programmed message: %q", message)
	}
}

func TestReconcileGatewayStatusWaitsForDerivedFrontendEndpointSliceConvergence(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses"),
			gatewayInfrastructureEndpointSlice(
				"gateway-conformance-infra",
				"gateway-static-addresses",
				managedresources.EndpointSliceRoleSharedFrontend,
			),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed)); message != "Waiting for derived Gateway frontend EndpointSlices to converge" {
		t.Fatalf("unexpected programmed message: %q", message)
	}
}

func TestReconcileGatewayStatusWaitsForDerivedFrontendEndpointSliceOwnershipConvergence(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses"),
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      infrastructure.GatewayServiceName("gateway-static-addresses") + "-ipv4",
					Namespace: "gateway-conformance-infra",
					Labels: map[string]string{
						discoveryv1.LabelManagedBy:               managedresources.ManagedByValue,
						discoveryv1.LabelServiceName:             infrastructure.GatewayServiceName("gateway-static-addresses"),
						managedresources.ServiceRoleKey:          managedresources.EndpointSliceRoleGatewayFrontend,
						"app.kubernetes.io/managed-by":           "nantian-gw",
						"gateway.networking.k8s.io/gateway-name": "gateway-static-addresses",
						"nantian.dev/gateway-namespace":               "gateway-conformance-infra",
					},
				},
				AddressType: discoveryv1.AddressTypeIPv4,
			},
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonPending),
		1,
	)
	if message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionProgrammed)); message != "Waiting for derived Gateway frontend EndpointSlices to converge" {
		t.Fatalf("unexpected programmed message: %q", message)
	}
}

func TestReconcileGatewayStatusAcceptsManagedFrontendEndpointSlice(t *testing.T) {
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "gateway-conformance-infra"}},
			staticAddressGatewayClass(controllerName),
			staticAddressGateway(nil),
			gatewayInfrastructureService("gateway-conformance-infra", "gateway-static-addresses"),
			gatewayInfrastructureEndpointSlice(
				"gateway-conformance-infra",
				"gateway-static-addresses",
				managedresources.EndpointSliceRoleGatewayFrontend,
			),
		).
		Build()

	reconcileGatewayAddresses(t, k8sClient, controllerName)

	gateway := getStaticAddressGateway(t, k8sClient)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
}

func staticAddressGatewayClass(controllerName gatewayv1.GatewayController) *gatewayv1.GatewayClass {
	return &gatewayv1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
		Spec: gatewayv1.GatewayClassSpec{
			ControllerName: controllerName,
		},
	}
}

func staticAddressGateway(addresses []gatewayv1.GatewaySpecAddress) *gatewayv1.Gateway {
	return &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gateway-static-addresses",
			Namespace:  "gateway-conformance-infra",
			Generation: 1,
			UID:        staticAddressGatewayUID,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Addresses:        addresses,
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     8080,
			}},
		},
	}
}

func gatewayInfrastructureService(namespace, gatewayName string) *corev1.Service {
	controller := true
	blockOwnerDeletion := true
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      infrastructure.GatewayServiceName(gatewayName),
			Namespace: namespace,
			UID:       staticAddressServiceUID,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by":           "nantian-gw",
				"nantian.dev/service-role":                    "gateway-metadata",
				"gateway.networking.k8s.io/gateway-name": gatewayName,
				"nantian.dev/gateway-namespace":               namespace,
			},
			Annotations: map[string]string{
				"nantian.dev/owner-kind":        "Gateway",
				"nantian.dev/owner-namespace":   namespace,
				"nantian.dev/owner-name":        gatewayName,
				"nantian.dev/owner-uid":         string(staticAddressGatewayUID),
				"nantian.dev/owner-generation":  "1",
				"nantian.dev/gatewayclass-name": "nantian-gw",
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         gatewayv1.GroupVersion.String(),
				Kind:               "Gateway",
				Name:               gatewayName,
				UID:                staticAddressGatewayUID,
				Controller:         &controller,
				BlockOwnerDeletion: &blockOwnerDeletion,
			}},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func gatewayInfrastructureServiceForGateway(gateway gatewayv1.Gateway) *corev1.Service {
	service := gatewayInfrastructureService(gateway.Namespace, gateway.Name)
	service.Annotations["nantian.dev/owner-generation"] = strconv.FormatInt(gateway.Generation, 10)
	service.Annotations["nantian.dev/gatewayclass-name"] = string(gateway.Spec.GatewayClassName)
	if gateway.Spec.Infrastructure != nil && gateway.Spec.Infrastructure.ParametersRef != nil {
		service.Annotations["nantian.dev/infrastructure-parameters-ref"] = gateway.Namespace + "/" + string(gateway.Spec.Infrastructure.ParametersRef.Name)
	} else {
		delete(service.Annotations, "nantian.dev/infrastructure-parameters-ref")
	}

	if gateway.UID == "" {
		delete(service.Annotations, "nantian.dev/owner-uid")
		service.OwnerReferences = nil
		return service
	}

	service.Annotations["nantian.dev/owner-uid"] = string(gateway.UID)
	controller := true
	blockOwnerDeletion := true
	service.OwnerReferences = []metav1.OwnerReference{{
		APIVersion:         gatewayv1.GroupVersion.String(),
		Kind:               "Gateway",
		Name:               gateway.Name,
		UID:                gateway.UID,
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}}
	return service
}

func gatewayInfrastructureEndpointSlice(namespace, gatewayName, role string) *discoveryv1.EndpointSlice {
	service := gatewayInfrastructureService(namespace, gatewayName)
	return gatewayInfrastructureEndpointSliceForService(service, role)
}

func gatewayInfrastructureEndpointSliceForService(service *corev1.Service, role string) *discoveryv1.EndpointSlice {
	controller := true
	blockOwnerDeletion := true
	labels := make(map[string]string, len(service.Labels)+2)
	for key, value := range service.Labels {
		labels[key] = value
	}
	labels[discoveryv1.LabelManagedBy] = managedresources.ManagedByValue
	labels[discoveryv1.LabelServiceName] = service.Name
	labels[managedresources.ServiceRoleKey] = role

	annotations := make(map[string]string, len(service.Annotations))
	for key, value := range service.Annotations {
		annotations[key] = value
	}

	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:        service.Name + "-ipv4",
			Namespace:   service.Namespace,
			Labels:      labels,
			Annotations: annotations,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         "v1",
				Kind:               "Service",
				Name:               service.Name,
				UID:                service.UID,
				Controller:         &controller,
				BlockOwnerDeletion: &blockOwnerDeletion,
			}},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
	}
}

func reconcileGatewayAddresses(t *testing.T, k8sClient client.Client, controllerName gatewayv1.GatewayController) {
	t.Helper()

	reconcileGatewayAddressesWithAdvertised(t, k8sClient, controllerName, []string{"127.0.0.1"})
}

func reconcileGatewayAddressesWithAdvertised(
	t *testing.T,
	k8sClient client.Client,
	controllerName gatewayv1.GatewayController,
	advertised []string,
) {
	t.Helper()

	reconciler := NewWithAddresses(k8sClient, string(controllerName), advertised, discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
}

func getStaticAddressGateway(t *testing.T, k8sClient client.Client) gatewayv1.Gateway {
	t.Helper()

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "gateway-conformance-infra", Name: "gateway-static-addresses"},
		&gateway,
	); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}

	return gateway
}

func addressTypePtr(value gatewayv1.AddressType) *gatewayv1.AddressType {
	return &value
}
