package managedresources

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ManagedByLabel = "app.kubernetes.io/managed-by"
	ManagedByValue = "nantian-gw"
	ServiceRoleKey = "nantian.dev/service-role"

	ServiceRoleShared  = "shared-dataplane"
	ServiceRoleGateway = "gateway-metadata"

	EndpointSliceRoleSharedFrontend  = "shared-frontend-endpoints"
	EndpointSliceRoleGatewayFrontend = "gateway-frontend-endpoints"
	EndpointSliceRoleMeshFrontend    = "mesh-frontend-endpoints"
)

func ShouldAffectSnapshot(object client.Object) bool {
	switch item := object.(type) {
	case *corev1.Service:
		if item == nil {
			return false
		}
		return !IsManagedFrontendService(*item)
	case *discoveryv1.EndpointSlice:
		if item == nil {
			return false
		}
		return !IsManagedFrontendEndpointSlice(*item)
	default:
		return true
	}
}

func IsManagedFrontendService(service corev1.Service) bool {
	if service.Labels[ManagedByLabel] != ManagedByValue {
		return false
	}

	switch service.Labels[ServiceRoleKey] {
	case ServiceRoleShared, ServiceRoleGateway:
		return true
	default:
		return false
	}
}

func IsManagedFrontendEndpointSlice(endpointSlice discoveryv1.EndpointSlice) bool {
	if endpointSlice.Labels[discoveryv1.LabelManagedBy] != ManagedByValue {
		return false
	}

	switch endpointSlice.Labels[ServiceRoleKey] {
	case EndpointSliceRoleSharedFrontend, EndpointSliceRoleGatewayFrontend, EndpointSliceRoleMeshFrontend:
		return true
	default:
		return false
	}
}

func FilterServices(services []corev1.Service) []corev1.Service {
	if len(services) == 0 {
		return nil
	}

	filtered := make([]corev1.Service, 0, len(services))
	for _, service := range services {
		if IsManagedFrontendService(service) {
			continue
		}
		filtered = append(filtered, service)
	}
	return filtered
}

func FilterEndpointSlices(endpointSlices []discoveryv1.EndpointSlice) []discoveryv1.EndpointSlice {
	if len(endpointSlices) == 0 {
		return nil
	}

	filtered := make([]discoveryv1.EndpointSlice, 0, len(endpointSlices))
	for _, endpointSlice := range endpointSlices {
		if IsManagedFrontendEndpointSlice(endpointSlice) {
			continue
		}
		filtered = append(filtered, endpointSlice)
	}
	return filtered
}
