package infrastructure

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestDesiredGatewayServiceInfersDualStackIPFamiliesFromStaticAddresses(t *testing.T) {
	ipAddressType := gatewayv1.IPAddressType

	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
			Addresses: []gatewayv1.GatewaySpecAddress{
				{Type: &ipAddressType, Value: "2001:db8::10"},
				{Type: &ipAddressType, Value: "198.51.100.10"},
			},
		},
	}

	desired := desiredGatewayService(&corev1.Service{}, gateway, gatewayServiceParameters{
		Type: corev1.ServiceTypeLoadBalancer,
	}, "")

	if desired.Spec.IPFamilyPolicy == nil || *desired.Spec.IPFamilyPolicy != corev1.IPFamilyPolicyPreferDualStack {
		t.Fatalf("ipFamilyPolicy = %#v, want PreferDualStack", desired.Spec.IPFamilyPolicy)
	}
	if got := desired.Spec.IPFamilies; len(got) != 2 || got[0] != corev1.IPv4Protocol || got[1] != corev1.IPv6Protocol {
		t.Fatalf("ipFamilies = %#v, want [IPv4, IPv6]", got)
	}
	if got := desired.Spec.ExternalIPs; len(got) != 2 || got[0] != "198.51.100.10" || got[1] != "2001:db8::10" {
		t.Fatalf("externalIPs = %#v, want sorted projected addresses", got)
	}
	if desired.Spec.LoadBalancerIP != "198.51.100.10" {
		t.Fatalf("loadBalancerIP = %q, want 198.51.100.10", desired.Spec.LoadBalancerIP)
	}
}

func TestDesiredGatewayServiceReturnsNilWithoutProgrammedListenerPorts(t *testing.T) {
	mode := gatewayv1.TLSModeTerminate

	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "aether-gateway",
			Listeners: []gatewayv1.Listener{{
				Name:     "https",
				Protocol: gatewayv1.HTTPSProtocolType,
				Port:     443,
				TLS: &gatewayv1.ListenerTLSConfig{
					Mode: &mode,
				},
			}},
		},
		Status: gatewayv1.GatewayStatus{
			Listeners: []gatewayv1.ListenerStatus{{
				Name: "https",
				Conditions: []metav1.Condition{{
					Type:   string(gatewayv1.ListenerConditionProgrammed),
					Status: metav1.ConditionFalse,
					Reason: string(gatewayv1.ListenerReasonInvalid),
				}},
			}},
		},
	}

	if desired := desiredGatewayService(&corev1.Service{}, gateway, gatewayServiceParameters{}, ""); desired != nil {
		t.Fatalf("expected gateway service to be skipped when no listener ports can be materialized, got %#v", desired.Spec.Ports)
	}
}

func TestDesiredGatewayServiceIgnoresStaleProgrammedFalseListenerStatus(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "public",
			Namespace:  "default",
			Generation: 2,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "aether-gateway",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
		Status: gatewayv1.GatewayStatus{
			Listeners: []gatewayv1.ListenerStatus{{
				Name: "http",
				Conditions: []metav1.Condition{
					{
						Type:               string(gatewayv1.ListenerConditionAccepted),
						Status:             metav1.ConditionTrue,
						ObservedGeneration: 1,
					},
					{
						Type:               string(gatewayv1.ListenerConditionProgrammed),
						Status:             metav1.ConditionFalse,
						Reason:             string(gatewayv1.ListenerReasonInvalid),
						ObservedGeneration: 1,
					},
				},
			}},
		},
	}

	desired := desiredGatewayService(&corev1.Service{}, gateway, gatewayServiceParameters{}, "")
	if desired == nil {
		t.Fatal("expected gateway service ports to be materialized from current listener spec")
	}
	if got := desired.Spec.Ports; len(got) != 1 || got[0].Port != 80 {
		t.Fatalf("ports = %#v, want one HTTP port 80", got)
	}
}

func TestDesiredGatewayServiceInfersSingleStackIPv6FamilyFromStaticAddress(t *testing.T) {
	ipAddressType := gatewayv1.IPAddressType

	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public-v6",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
			Addresses: []gatewayv1.GatewaySpecAddress{
				{Type: &ipAddressType, Value: "2001:db8::10"},
			},
		},
	}

	desired := desiredGatewayService(&corev1.Service{}, gateway, gatewayServiceParameters{
		Type: corev1.ServiceTypeLoadBalancer,
	}, "")

	if desired.Spec.IPFamilyPolicy == nil || *desired.Spec.IPFamilyPolicy != corev1.IPFamilyPolicySingleStack {
		t.Fatalf("ipFamilyPolicy = %#v, want SingleStack", desired.Spec.IPFamilyPolicy)
	}
	if got := desired.Spec.IPFamilies; len(got) != 1 || got[0] != corev1.IPv6Protocol {
		t.Fatalf("ipFamilies = %#v, want [IPv6]", got)
	}
	if got := desired.Spec.ExternalIPs; len(got) != 1 || got[0] != "2001:db8::10" {
		t.Fatalf("externalIPs = %#v, want IPv6 static address", got)
	}
	if desired.Spec.LoadBalancerIP != "2001:db8::10" {
		t.Fatalf("loadBalancerIP = %q, want 2001:db8::10", desired.Spec.LoadBalancerIP)
	}
}

func TestDesiredGatewayServiceClearsInferredIPFamiliesWhenStaticAddressesDisappear(t *testing.T) {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
			Addresses: nil,
		},
	}

	current := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gatewayServiceName("public"),
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:           corev1.ServiceTypeLoadBalancer,
			IPFamilyPolicy: ptrIPFamilyPolicy(corev1.IPFamilyPolicyPreferDualStack),
			IPFamilies:     []corev1.IPFamily{corev1.IPv4Protocol, corev1.IPv6Protocol},
			ExternalIPs:    []string{"198.51.100.10", "2001:db8::10"},
			LoadBalancerIP: "198.51.100.10",
			ClusterIP:      "10.96.0.1",
			ClusterIPs:     []string{"10.96.0.1"},
		},
	}

	desired := desiredGatewayService(current, gateway, gatewayServiceParameters{
		Type: corev1.ServiceTypeLoadBalancer,
	}, "")

	if desired.Spec.IPFamilyPolicy != nil {
		t.Fatalf("ipFamilyPolicy = %#v, want nil after static addresses disappear", desired.Spec.IPFamilyPolicy)
	}
	if len(desired.Spec.IPFamilies) != 0 {
		t.Fatalf("ipFamilies = %#v, want empty after static addresses disappear", desired.Spec.IPFamilies)
	}
	if len(desired.Spec.ExternalIPs) != 0 {
		t.Fatalf("externalIPs = %#v, want empty after static addresses disappear", desired.Spec.ExternalIPs)
	}
	if desired.Spec.LoadBalancerIP != "" {
		t.Fatalf("loadBalancerIP = %q, want empty after static addresses disappear", desired.Spec.LoadBalancerIP)
	}
}

func TestDesiredGatewayServiceLetsExplicitParametersOverrideStaticAddressInference(t *testing.T) {
	ipAddressType := gatewayv1.IPAddressType
	ipv6Only := corev1.IPv6Protocol
	customPolicy := corev1.IPFamilyPolicySingleStack

	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
			Addresses: []gatewayv1.GatewaySpecAddress{
				{Type: &ipAddressType, Value: "198.51.100.10"},
				{Type: &ipAddressType, Value: "2001:db8::10"},
			},
		},
	}

	desired := desiredGatewayService(&corev1.Service{}, gateway, gatewayServiceParameters{
		Type:           corev1.ServiceTypeLoadBalancer,
		IPFamilyPolicy: &customPolicy,
		IPFamilies:     []corev1.IPFamily{ipv6Only},
		LoadBalancerIP: ptrString("203.0.113.10"),
		ExternalIPs:    []string{"203.0.113.10"},
	}, "")

	if desired.Spec.IPFamilyPolicy == nil || *desired.Spec.IPFamilyPolicy != corev1.IPFamilyPolicySingleStack {
		t.Fatalf("ipFamilyPolicy = %#v, want explicit SingleStack override", desired.Spec.IPFamilyPolicy)
	}
	if got := desired.Spec.IPFamilies; len(got) != 1 || got[0] != corev1.IPv6Protocol {
		t.Fatalf("ipFamilies = %#v, want explicit IPv6 override", got)
	}
	if got := desired.Spec.ExternalIPs; len(got) != 2 || got[0] != "198.51.100.10" || got[1] != "2001:db8::10" {
		t.Fatalf("externalIPs = %#v, want static addresses to continue taking precedence", got)
	}
	if desired.Spec.LoadBalancerIP != "198.51.100.10" {
		t.Fatalf("loadBalancerIP = %q, want static address precedence", desired.Spec.LoadBalancerIP)
	}
}

func ptrString(value string) *string {
	return &value
}
