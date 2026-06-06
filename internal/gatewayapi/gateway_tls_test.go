package gatewayapi

import (
	"testing"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestFrontendValidationForListenerUsesPerPortOverrideForHTTPS(t *testing.T) {
	terminate := gatewayv1.TLSModeTerminate
	defaultValidation := &gatewayv1.FrontendTLSValidation{
		CACertificateRefs: []gatewayv1.ObjectReference{{Name: "default-ca"}},
	}
	overrideValidation := &gatewayv1.FrontendTLSValidation{
		CACertificateRefs: []gatewayv1.ObjectReference{{Name: "override-ca"}},
	}

	gateway := gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			TLS: &gatewayv1.GatewayTLSConfig{
				Frontend: &gatewayv1.FrontendTLSConfig{
					Default: gatewayv1.TLSConfig{Validation: defaultValidation},
					PerPort: []gatewayv1.TLSPortConfig{{
						Port: 8443,
						TLS: gatewayv1.TLSConfig{
							Validation: overrideValidation,
						},
					}},
				},
			},
			Listeners: []gatewayv1.Listener{{
				Name:     "https-alt",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     8443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: &terminate,
				},
			}},
		},
	}

	got := FrontendValidationForListener(gateway, gateway.Spec.Listeners[0])
	if got != overrideValidation {
		t.Fatalf("expected per-port validation override, got %#v", got)
	}
}

func TestFrontendValidationForListenerIgnoresNonHTTPSListeners(t *testing.T) {
	passthrough := gatewayv1.TLSModePassthrough

	gateway := gatewayv1.Gateway{
		Spec: gatewayv1.GatewaySpec{
			TLS: &gatewayv1.GatewayTLSConfig{
				Frontend: &gatewayv1.FrontendTLSConfig{
					Default: gatewayv1.TLSConfig{
						Validation: &gatewayv1.FrontendTLSValidation{
							CACertificateRefs: []gatewayv1.ObjectReference{{Name: "client-ca"}},
						},
					},
				},
			},
			Listeners: []gatewayv1.Listener{{
				Name:     "tls-pass",
				Protocol: gatewayv1.TLSProtocolType,
				Port:     443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: &passthrough,
				},
			}},
		},
	}

	if got := FrontendValidationForListener(gateway, gateway.Spec.Listeners[0]); got != nil {
		t.Fatalf("expected non-HTTPS listener to ignore frontend validation, got %#v", got)
	}
}
