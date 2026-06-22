package status

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/infrastructure"
	"github.com/nantian-gw/gateway/internal/resources"
)

func gatewayAdvertisedAddresses(state *clusterState, gateway gatewayv1.Gateway) []string {
	service, ok := state.serviceByKey[namespacedName(
		gateway.Namespace,
		infrastructure.GatewayServiceName(gateway.Name),
	)]
	if ok && gatewayServiceAdvertisementsReady(state, gateway, service) {
		if addresses := serviceAdvertisedAddresses(service); len(addresses) > 0 {
			return addresses
		}
	}

	return append([]string(nil), state.statusAddresses...)
}

func gatewayServiceAdvertisementsReady(
	state *clusterState,
	gateway gatewayv1.Gateway,
	service corev1.Service,
) bool {
	if !resources.IsManagedFrontendService(service) {
		return true
	}
	return infrastructure.GatewayServiceMetadataMatches(
		service,
		gateway,
		gatewayClassParametersReference(state, gateway),
	)
}

func gatewayPublishedAddresses(state *clusterState, gateway gatewayv1.Gateway) []string {
	serviceKey := namespacedName(
		gateway.Namespace,
		infrastructure.GatewayServiceName(gateway.Name),
	)
	service, ok := state.serviceByKey[serviceKey]
	if ok && infrastructure.GatewayServiceMetadataMatches(
		service,
		gateway,
		gatewayClassParametersReference(state, gateway),
	) {
		if addresses := serviceAdvertisedAddresses(service); len(addresses) > 0 {
			return addresses
		}
	}

	return append([]string(nil), state.statusAddresses...)
}

func gatewayInfrastructureServiceStatus(state *clusterState, gateway gatewayv1.Gateway) (bool, string) {
	convergence := gatewayInfrastructureConvergenceState(state, gateway)
	switch {
	case !convergence.serviceExists:
		return false, "Waiting for derived Gateway Service to be created"
	case !convergence.serviceReady:
		return false, "Waiting for derived Gateway Service metadata to converge"
	case !convergence.frontendEndpointSliceReady:
		return false, "Waiting for derived Gateway frontend EndpointSlices to converge"
	default:
		return true, ""
	}
}

type gatewayInfrastructureConvergence struct {
	serviceExists              bool
	serviceReady               bool
	frontendEndpointSliceReady bool
}

func gatewayInfrastructureConvergenceState(
	state *clusterState,
	gateway gatewayv1.Gateway,
) gatewayInfrastructureConvergence {
	convergence := gatewayInfrastructureConvergence{}
	serviceKey := namespacedName(
		gateway.Namespace,
		infrastructure.GatewayServiceName(gateway.Name),
	)
	service, ok := state.serviceByKey[serviceKey]
	if !ok {
		return convergence
	}
	convergence.serviceExists = true
	convergence.serviceReady = infrastructureServiceMetadataReady(state, gateway, service)
	if !convergence.serviceReady {
		return convergence
	}

	endpointSlices := state.endpointSlicesByService[serviceKey]
	if len(endpointSlices) == 0 {
		return convergence
	}
	if !hasManagedGatewayFrontendEndpointSlice(endpointSlices, service) {
		convergence.frontendEndpointSliceReady = false
		return convergence
	}

	convergence.frontendEndpointSliceReady = true
	return convergence
}

func infrastructureServiceMetadataReady(
	state *clusterState,
	gateway gatewayv1.Gateway,
	service corev1.Service,
) bool {
	return infrastructure.GatewayServiceMetadataMatches(
		service,
		gateway,
		gatewayClassParametersReference(state, gateway),
	)
}

func hasManagedGatewayFrontendEndpointSlice(
	endpointSlices []discoveryv1.EndpointSlice,
	service corev1.Service,
) bool {
	for _, endpointSlice := range endpointSlices {
		if !resources.IsManagedFrontendEndpointSlice(endpointSlice) {
			continue
		}
		if endpointSlice.Labels[resources.ServiceRoleKey] == resources.EndpointSliceRoleGatewayFrontend {
			if infrastructure.GatewayFrontendEndpointSliceMetadataMatches(endpointSlice, service) {
				return true
			}
		}
	}

	return false
}

func gatewayClassParametersReference(state *clusterState, gateway gatewayv1.Gateway) string {
	if state == nil {
		return ""
	}

	gatewayClass, ok := state.managedGatewayClasses[string(gateway.Spec.GatewayClassName)]
	if !ok {
		return ""
	}

	return infrastructure.GatewayClassParametersReference(&gatewayClass)
}

func serviceAdvertisedAddresses(service corev1.Service) []string {
	addresses := canonicalGatewayPublishedAddresses(func(appendAddress func(string)) {
		for _, ingress := range service.Status.LoadBalancer.Ingress {
			appendAddress(ingress.IP)
			appendAddress(ingress.Hostname)
		}
		for _, value := range service.Spec.ExternalIPs {
			appendAddress(value)
		}
		appendAddress(service.Spec.LoadBalancerIP)
	})

	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		out = append(out, address.Value)
	}
	return out
}
