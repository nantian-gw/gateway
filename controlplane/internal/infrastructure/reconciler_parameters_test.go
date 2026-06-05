package infrastructure

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestReconcileResolvesGatewayServiceParametersOncePerGateway(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	configMapKind := gatewayv1.Kind("ConfigMap")
	configMapNamespace := gatewayv1.Namespace(defaultDataplaneNamespace)

	baseClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gatewayclass-params",
					Namespace: defaultDataplaneNamespace,
				},
				Data: map[string]string{
					serviceParametersYAMLKey: "type: ClusterIP\n",
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-params",
					Namespace: "default",
				},
				Data: map[string]string{
					serviceParametersYAMLKey: "externalIPs:\n- 192.0.2.10\n",
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
					ParametersRef: &gatewayv1.ParametersReference{
						Group:     "",
						Kind:      configMapKind,
						Name:      "gatewayclass-params",
						Namespace: &configMapNamespace,
					},
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: &gatewayv1.LocalParametersReference{
							Group: "",
							Kind:  configMapKind,
							Name:  "gateway-params",
						},
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
					Labels:    map[string]string{"app": "nantian-dataplane"},
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

	counts := make(map[string]int)
	reconciler := New(
		countingGetClient{
			Client: baseClient,
			onGet: func(key client.ObjectKey, obj client.Object) {
				switch obj.(type) {
				case *gatewayv1.GatewayClass:
					counts["gatewayclass:"+key.String()]++
				case *corev1.ConfigMap:
					counts["configmap:"+key.String()]++
				}
			},
		},
		string(controllerName),
		discardLogger(),
	)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if counts["gatewayclass:/nantian-gw"] != 1 {
		t.Fatalf("GatewayClass get count = %d, want 1", counts["gatewayclass:/nantian-gw"])
	}
	if counts["configmap:"+defaultDataplaneNamespace+"/gatewayclass-params"] != 1 {
		t.Fatalf(
			"GatewayClass parameters ConfigMap get count = %d, want 1",
			counts["configmap:"+defaultDataplaneNamespace+"/gatewayclass-params"],
		)
	}
	if counts["configmap:default/gateway-params"] != 1 {
		t.Fatalf(
			"Gateway parameters ConfigMap get count = %d, want 1",
			counts["configmap:default/gateway-params"],
		)
	}
}
func TestReconcileCachesGatewayClassParametersAcrossGateways(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	configMapKind := gatewayv1.Kind("ConfigMap")
	configMapNamespace := gatewayv1.Namespace(defaultDataplaneNamespace)

	baseClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gatewayclass-params",
					Namespace: defaultDataplaneNamespace,
				},
				Data: map[string]string{
					serviceParametersYAMLKey: "type: ClusterIP\n",
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
					ParametersRef: &gatewayv1.ParametersReference{
						Group:     "",
						Kind:      configMapKind,
						Name:      "gatewayclass-params",
						Namespace: &configMapNamespace,
					},
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public-a",
					Namespace: "default",
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public-b",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
				},
			},
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-dataplane"},
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

	counts := make(map[string]int)
	reconciler := New(
		countingGetClient{
			Client: baseClient,
			onGet: func(key client.ObjectKey, obj client.Object) {
				switch obj.(type) {
				case *gatewayv1.GatewayClass:
					counts["gatewayclass:"+key.String()]++
				case *corev1.ConfigMap:
					counts["configmap:"+key.String()]++
				}
			},
		},
		string(controllerName),
		discardLogger(),
	)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if counts["gatewayclass:/nantian-gw"] != 1 {
		t.Fatalf("shared GatewayClass get count = %d, want 1", counts["gatewayclass:/nantian-gw"])
	}
	if counts["configmap:"+defaultDataplaneNamespace+"/gatewayclass-params"] != 1 {
		t.Fatalf(
			"shared GatewayClass parameters ConfigMap get count = %d, want 1",
			counts["configmap:"+defaultDataplaneNamespace+"/gatewayclass-params"],
		)
	}
}
func TestReconcileAppliesGatewayInfrastructureParametersRef(t *testing.T) {
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
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public-gateway-infra",
					Namespace: "default",
				},
				Data: map[string]string{
					serviceParametersYAMLKey: `
type: LoadBalancer
externalTrafficPolicy: Local
sessionAffinity: ClientIP
sessionAffinityConfig:
  clientIP:
    timeoutSeconds: 7200
internalTrafficPolicy: Local
ipFamilyPolicy: PreferDualStack
ipFamilies:
  - IPv6
  - IPv4
publishNotReadyAddresses: true
externalIPs:
  - 203.0.113.10
  - 203.0.113.20
loadBalancerIP: 203.0.113.10
healthCheckNodePort: 32080
loadBalancerClass: internal.example.com/vip
loadBalancerSourceRanges:
  - 203.0.113.0/24
  - 198.51.100.0/24
allocateLoadBalancerNodePorts: false
`,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: &gatewayv1.LocalParametersReference{
							Group: "",
							Kind:  gatewayv1.Kind("ConfigMap"),
							Name:  "public-gateway-infra",
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
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

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("service type = %s, want LoadBalancer", service.Spec.Type)
	}
	if service.Spec.Selector != nil {
		t.Fatalf("gateway service selector = %#v, want nil with managed EndpointSlices", service.Spec.Selector)
	}
	if service.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
		t.Fatalf("externalTrafficPolicy = %s, want Local", service.Spec.ExternalTrafficPolicy)
	}
	if service.Spec.InternalTrafficPolicy == nil ||
		*service.Spec.InternalTrafficPolicy != corev1.ServiceInternalTrafficPolicyLocal {
		t.Fatalf("internalTrafficPolicy = %#v, want Local", service.Spec.InternalTrafficPolicy)
	}
	if service.Spec.IPFamilyPolicy == nil || *service.Spec.IPFamilyPolicy != corev1.IPFamilyPolicyPreferDualStack {
		t.Fatalf("ipFamilyPolicy = %#v, want PreferDualStack", service.Spec.IPFamilyPolicy)
	}
	if got := service.Spec.IPFamilies; len(got) != 2 || got[0] != corev1.IPv4Protocol || got[1] != corev1.IPv6Protocol {
		t.Fatalf("ipFamilies = %#v", got)
	}
	if service.Spec.SessionAffinity != corev1.ServiceAffinityClientIP {
		t.Fatalf("sessionAffinity = %s, want ClientIP", service.Spec.SessionAffinity)
	}
	if service.Spec.SessionAffinityConfig == nil || service.Spec.SessionAffinityConfig.ClientIP == nil ||
		service.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds == nil ||
		*service.Spec.SessionAffinityConfig.ClientIP.TimeoutSeconds != 7200 {
		t.Fatalf("sessionAffinityConfig = %#v", service.Spec.SessionAffinityConfig)
	}
	if !service.Spec.PublishNotReadyAddresses {
		t.Fatalf("expected publishNotReadyAddresses=true")
	}
	if got := service.Spec.ExternalIPs; len(got) != 2 || got[0] != "203.0.113.10" || got[1] != "203.0.113.20" {
		t.Fatalf("externalIPs = %#v", got)
	}
	if service.Spec.LoadBalancerIP != "203.0.113.10" {
		t.Fatalf("loadBalancerIP = %q, want 203.0.113.10", service.Spec.LoadBalancerIP)
	}
	if service.Spec.HealthCheckNodePort != 32080 {
		t.Fatalf("healthCheckNodePort = %d, want 32080", service.Spec.HealthCheckNodePort)
	}
	if service.Spec.LoadBalancerClass == nil || *service.Spec.LoadBalancerClass != "internal.example.com/vip" {
		t.Fatalf("loadBalancerClass = %#v", service.Spec.LoadBalancerClass)
	}
	if service.Spec.AllocateLoadBalancerNodePorts == nil || *service.Spec.AllocateLoadBalancerNodePorts {
		t.Fatalf("allocateLoadBalancerNodePorts = %#v, want false", service.Spec.AllocateLoadBalancerNodePorts)
	}
	if got := service.Spec.LoadBalancerSourceRanges; len(got) != 2 ||
		got[0] != "198.51.100.0/24" ||
		got[1] != "203.0.113.0/24" {
		t.Fatalf("loadBalancerSourceRanges = %#v", got)
	}
}
func TestReconcileProgramsGatewayStaticIPAddressesOntoService(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	ipAddressType := gatewayv1.IPAddressType
	hostnameAddressType := gatewayv1.HostnameAddressType

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
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
					Name:      "public-static-ip",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Addresses: []gatewayv1.GatewaySpecAddress{
						{Type: &ipAddressType, Value: "203.0.113.20"},
						{Type: &ipAddressType, Value: "203.0.113.10"},
						{Type: &hostnameAddressType, Value: "gw.example.com"},
					},
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: &gatewayv1.LocalParametersReference{
							Group: "",
							Kind:  gatewayv1.Kind("ConfigMap"),
							Name:  "public-gateway-infra",
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
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
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("public-static-ip")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("service type = %s, want LoadBalancer", service.Spec.Type)
	}
	if got := service.Spec.ExternalIPs; len(got) != 2 || got[0] != "203.0.113.10" || got[1] != "203.0.113.20" {
		t.Fatalf("externalIPs = %#v", got)
	}
	if service.Spec.LoadBalancerIP != "203.0.113.10" {
		t.Fatalf("loadBalancerIP = %q, want 203.0.113.10", service.Spec.LoadBalancerIP)
	}
}
func TestReconcileSkipsLoopbackGatewayStaticIPProjectionOntoService(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	ipAddressType := gatewayv1.IPAddressType

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
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
					Name:      "public-loopback-ip",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Addresses: []gatewayv1.GatewaySpecAddress{
						{Type: &ipAddressType, Value: "127.0.0.1"},
					},
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: &gatewayv1.LocalParametersReference{
							Group: "",
							Kind:  gatewayv1.Kind("ConfigMap"),
							Name:  "public-gateway-infra",
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
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
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("public-loopback-ip")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("service type = %s, want LoadBalancer", service.Spec.Type)
	}
	if len(service.Spec.ExternalIPs) != 0 {
		t.Fatalf("externalIPs = %#v, want empty when static address is loopback", service.Spec.ExternalIPs)
	}
	if service.Spec.LoadBalancerIP != "" {
		t.Fatalf("loadBalancerIP = %q, want empty when static address is loopback", service.Spec.LoadBalancerIP)
	}
	assertServicePort(t, service.Spec.Ports, 8080, corev1.ProtocolTCP, 0)
}
func TestReconcileIgnoresInvalidGatewayInfrastructureParametersRef(t *testing.T) {
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
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "broken-gateway-infra",
					Namespace: "default",
				},
				Data: map[string]string{
					serviceParametersYAMLKey: "type: ExternalName\n",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "broken",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: &gatewayv1.LocalParametersReference{
							Group: "",
							Kind:  gatewayv1.Kind("ConfigMap"),
							Name:  "broken-gateway-infra",
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
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
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("broken")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}

	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("service type = %s, want ClusterIP fallback", service.Spec.Type)
	}
	if service.Spec.Selector != nil {
		t.Fatalf("gateway service selector = %#v, want nil with managed EndpointSlices", service.Spec.Selector)
	}
	if service.Spec.ExternalTrafficPolicy != "" {
		t.Fatalf("externalTrafficPolicy = %q, want empty", service.Spec.ExternalTrafficPolicy)
	}
	if service.Spec.InternalTrafficPolicy != nil {
		t.Fatalf("internalTrafficPolicy = %#v, want nil", service.Spec.InternalTrafficPolicy)
	}
	if service.Spec.SessionAffinity != corev1.ServiceAffinityNone {
		t.Fatalf("sessionAffinity = %s, want None", service.Spec.SessionAffinity)
	}
	if service.Spec.PublishNotReadyAddresses {
		t.Fatalf("publishNotReadyAddresses should default to false")
	}
	if service.Spec.LoadBalancerClass != nil {
		t.Fatalf("loadBalancerClass = %#v, want nil", service.Spec.LoadBalancerClass)
	}
	if service.Spec.AllocateLoadBalancerNodePorts != nil {
		t.Fatalf(
			"allocateLoadBalancerNodePorts = %#v, want nil",
			service.Spec.AllocateLoadBalancerNodePorts,
		)
	}
	if len(service.Spec.LoadBalancerSourceRanges) != 0 {
		t.Fatalf("loadBalancerSourceRanges = %#v, want empty", service.Spec.LoadBalancerSourceRanges)
	}
}
func TestReconcileMergesGatewayClassInfrastructureParametersRef(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	classNamespace := gatewayv1.Namespace("infra-system")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gatewayclass-defaults",
					Namespace: "infra-system",
				},
				Data: map[string]string{
					serviceParametersYAMLKey: `
type: LoadBalancer
externalTrafficPolicy: Local
publishNotReadyAddresses: true
`,
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-overrides",
					Namespace: "default",
				},
				Data: map[string]string{
					serviceParametersYAMLKey: `
loadBalancerClass: internal.example.com/vip
allocateLoadBalancerNodePorts: false
`,
				},
			},
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						ParametersRef: &gatewayv1.LocalParametersReference{
							Group: "",
							Kind:  gatewayv1.Kind("ConfigMap"),
							Name:  "gateway-overrides",
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
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

	if service.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Fatalf("service type = %s, want LoadBalancer", service.Spec.Type)
	}
	if service.Spec.Selector != nil {
		t.Fatalf("gateway service selector = %#v, want nil with managed EndpointSlices", service.Spec.Selector)
	}
	if service.Spec.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
		t.Fatalf("externalTrafficPolicy = %s, want Local", service.Spec.ExternalTrafficPolicy)
	}
	if !service.Spec.PublishNotReadyAddresses {
		t.Fatalf("expected publishNotReadyAddresses=true from GatewayClass defaults")
	}
	if service.Annotations["nantian.dev/gatewayclass-parameters-ref"] != "infra-system/gatewayclass-defaults" {
		t.Fatalf(
			"gatewayclass-parameters-ref annotation = %q",
			service.Annotations["nantian.dev/gatewayclass-parameters-ref"],
		)
	}
	if service.Spec.LoadBalancerClass == nil || *service.Spec.LoadBalancerClass != "internal.example.com/vip" {
		t.Fatalf("loadBalancerClass = %#v", service.Spec.LoadBalancerClass)
	}
	if service.Spec.AllocateLoadBalancerNodePorts == nil || *service.Spec.AllocateLoadBalancerNodePorts {
		t.Fatalf("allocateLoadBalancerNodePorts = %#v, want false", service.Spec.AllocateLoadBalancerNodePorts)
	}
}
func TestReconcileResetsGatewayServiceFieldsWhenParametersRefRemoved(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	internalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
	loadBalancerClass := "internal.example.com/vip"
	allocateNodePorts := false

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
				},
				Spec: corev1.ServiceSpec{
					Type:                          corev1.ServiceTypeLoadBalancer,
					Selector:                      map[string]string{"app": "nantian-dataplane"},
					ExternalTrafficPolicy:         corev1.ServiceExternalTrafficPolicyLocal,
					InternalTrafficPolicy:         &internalTrafficPolicy,
					SessionAffinity:               corev1.ServiceAffinityClientIP,
					PublishNotReadyAddresses:      true,
					LoadBalancerClass:             &loadBalancerClass,
					LoadBalancerSourceRanges:      []string{"198.51.100.0/24"},
					AllocateLoadBalancerNodePorts: &allocateNodePorts,
					ClusterIP:                     "10.0.0.10",
					Ports: []corev1.ServicePort{{
						Name:       "tcp-80",
						Port:       80,
						TargetPort: intstr.FromInt(80),
						Protocol:   corev1.ProtocolTCP,
						NodePort:   32080,
					}},
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

	if service.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("service type = %s, want ClusterIP", service.Spec.Type)
	}
	if service.Spec.Selector != nil {
		t.Fatalf("gateway service selector = %#v, want nil with managed EndpointSlices", service.Spec.Selector)
	}
	if service.Spec.ExternalTrafficPolicy != "" {
		t.Fatalf("externalTrafficPolicy = %q, want empty", service.Spec.ExternalTrafficPolicy)
	}
	if service.Spec.InternalTrafficPolicy != nil {
		t.Fatalf("internalTrafficPolicy = %#v, want nil", service.Spec.InternalTrafficPolicy)
	}
	if service.Spec.SessionAffinity != corev1.ServiceAffinityNone {
		t.Fatalf("sessionAffinity = %s, want None", service.Spec.SessionAffinity)
	}
	if service.Spec.PublishNotReadyAddresses {
		t.Fatalf("publishNotReadyAddresses should reset to false")
	}
	if service.Spec.LoadBalancerClass != nil {
		t.Fatalf("loadBalancerClass = %#v, want nil", service.Spec.LoadBalancerClass)
	}
	if service.Spec.AllocateLoadBalancerNodePorts != nil {
		t.Fatalf(
			"allocateLoadBalancerNodePorts = %#v, want nil",
			service.Spec.AllocateLoadBalancerNodePorts,
		)
	}
	if len(service.Spec.LoadBalancerSourceRanges) != 0 {
		t.Fatalf("loadBalancerSourceRanges = %#v, want empty", service.Spec.LoadBalancerSourceRanges)
	}
	assertServicePort(t, service.Spec.Ports, 80, corev1.ProtocolTCP, 0)
}
