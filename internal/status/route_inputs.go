package status

import (
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
)

func httpRouteInput(route gatewayv1.HTTPRoute) routeInput {
	validation := gatewayapi.ValidateHTTPRouteRules(route)

	resolvedRefsErrorMessage := ""
	if validation.FullyInvalid(len(route.Spec.Rules)) && validation.AcceptedErrorMessage() == "" {
		resolvedRefsErrorMessage = validation.InvalidRuleMessage()
	}

	partiallyInvalidErrorMessage := ""
	if validation.PartiallyInvalid(len(route.Spec.Rules)) {
		partiallyInvalidErrorMessage = validation.DroppedRulesMessage()
	}

	return routeInput{
		kind:                         routeKindHTTP,
		namespace:                    route.Namespace,
		name:                         route.Name,
		generation:                   route.Generation,
		hostnames:                    route.Spec.Hostnames,
		parentRefs:                   route.Spec.ParentRefs,
		defaultGatewayScope:          route.Spec.UseDefaultGateways,
		backends:                     httpRouteBackends(route),
		extensionRefs:                httpRouteExtensionRefs(route),
		acceptedErrorMessage:         validation.AcceptedErrorMessage(),
		resolvedRefsErrorMessage:     resolvedRefsErrorMessage,
		partiallyInvalidErrorMessage: partiallyInvalidErrorMessage,
	}
}

func grpcRouteInput(route gatewayv1.GRPCRoute) routeInput {
	return routeInput{
		kind:                routeKindGRPC,
		namespace:           route.Namespace,
		name:                route.Name,
		generation:          route.Generation,
		hostnames:           route.Spec.Hostnames,
		parentRefs:          route.Spec.ParentRefs,
		defaultGatewayScope: route.Spec.UseDefaultGateways,
		backends:            grpcRouteBackends(route),
		extensionRefs:       grpcRouteExtensionRefs(route),
	}
}

func tcpRouteInput(route gatewayv1alpha2.TCPRoute) routeInput {
	return routeInput{
		kind:                routeKindTCP,
		namespace:           route.Namespace,
		name:                route.Name,
		generation:          route.Generation,
		parentRefs:          route.Spec.ParentRefs,
		defaultGatewayScope: route.Spec.UseDefaultGateways,
		backends:            tcpRouteBackends(route),
	}
}

func udpRouteInput(route gatewayv1alpha2.UDPRoute) routeInput {
	return routeInput{
		kind:                routeKindUDP,
		namespace:           route.Namespace,
		name:                route.Name,
		generation:          route.Generation,
		parentRefs:          route.Spec.ParentRefs,
		defaultGatewayScope: route.Spec.UseDefaultGateways,
		backends:            udpRouteBackends(route),
	}
}

func tlsRouteInput(route gatewayv1alpha2.TLSRoute) routeInput {
	return routeInput{
		kind:                routeKindTLS,
		namespace:           route.Namespace,
		name:                route.Name,
		generation:          route.Generation,
		hostnames:           route.Spec.Hostnames,
		parentRefs:          route.Spec.ParentRefs,
		defaultGatewayScope: route.Spec.UseDefaultGateways,
		backends:            tlsRouteBackends(route),
	}
}
