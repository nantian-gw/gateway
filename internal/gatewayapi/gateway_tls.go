package gatewayapi

import gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

func GatewayBackendTLS(gateway gatewayv1.Gateway) *gatewayv1.GatewayBackendTLS {
	if gateway.Spec.TLS == nil {
		return nil
	}

	return gateway.Spec.TLS.Backend
}

func FrontendValidationForListener(
	gateway gatewayv1.Gateway,
	listener gatewayv1.Listener,
) *gatewayv1.FrontendTLSValidation {
	if listener.TLS == nil ||
		listener.Protocol != gatewayv1.HTTPSProtocolType ||
		gateway.Spec.TLS == nil ||
		gateway.Spec.TLS.Frontend == nil {
		return nil
	}

	for _, item := range gateway.Spec.TLS.Frontend.PerPort {
		if item.Port == listener.Port {
			return item.TLS.Validation
		}
	}

	return gateway.Spec.TLS.Frontend.Default.Validation
}
