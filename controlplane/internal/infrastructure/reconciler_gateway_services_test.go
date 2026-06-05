package infrastructure

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestReconcileSharedServiceUpdateAvoidsRedundantReread(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "aether-gateway",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}
	existingSharedService := desiredSharedService(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            defaultSharedServiceName,
				Namespace:       defaultDataplaneNamespace,
				ResourceVersion: "1",
				UID:             "shared-service-uid",
			},
		},
		[]gatewayv1.Gateway{gateway},
		DefaultOptions(),
	)

	baseClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gateway,
			existingSharedService,
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aether-gateway-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "aether-gateway-dataplane"},
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

	sharedServiceKey := client.ObjectKey{
		Namespace: defaultDataplaneNamespace,
		Name:      defaultSharedServiceName,
	}
	sharedServiceGets := 0
	reconciler := New(
		countingGetClient{
			Client: baseClient,
			onGet: func(key client.ObjectKey, obj client.Object) {
				if _, ok := obj.(*corev1.Service); ok && key == sharedServiceKey {
					sharedServiceGets++
				}
			},
		},
		string(controllerName),
		discardLogger(),
	)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if sharedServiceGets != 1 {
		t.Fatalf("shared Service get count = %d, want 1", sharedServiceGets)
	}
}
func TestReconcileGatewayServiceUpdateAvoidsRedundantReread(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "public",
			Namespace: "default",
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "aether-gateway",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}
	gatewayServiceKey := gatewayServiceObjectKey(gateway)
	existingGatewayService := desiredGatewayService(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:            gatewayServiceKey.Name,
				Namespace:       gatewayServiceKey.Namespace,
				ResourceVersion: "1",
				UID:             "gateway-service-uid",
			},
			Spec: corev1.ServiceSpec{
				ClusterIP: "10.96.0.15",
			},
		},
		gateway,
		gatewayServiceParameters{},
		"",
	)

	baseClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gateway,
			existingGatewayService,
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "aether-gateway-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "aether-gateway-dataplane"},
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

	gatewayServiceGets := 0
	reconciler := New(
		countingGetClient{
			Client: baseClient,
			onGet: func(key client.ObjectKey, obj client.Object) {
				if _, ok := obj.(*corev1.Service); ok && key == gatewayServiceKey {
					gatewayServiceGets++
				}
			},
		},
		string(controllerName),
		discardLogger(),
	)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if gatewayServiceGets != 1 {
		t.Fatalf("gateway Service get count = %d, want 1", gatewayServiceGets)
	}
}
func TestReconcileSkipsInvalidGatewayListenersInDerivedResources(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "edge",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
						},
						{
							Name:     "broken-https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
						},
					},
				},
				Status: gatewayv1.GatewayStatus{
					Listeners: []gatewayv1.ListenerStatus{
						{
							Name: "http",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayv1.ListenerConditionAccepted),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionResolvedRefs),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionProgrammed),
									Status:             metav1.ConditionTrue,
									ObservedGeneration: 1,
								},
							},
						},
						{
							Name: "broken-https",
							Conditions: []metav1.Condition{
								{
									Type:               string(gatewayv1.ListenerConditionAccepted),
									Status:             metav1.ConditionFalse,
									Reason:             string(gatewayv1.ListenerReasonInvalid),
									ObservedGeneration: 1,
								},
								{
									Type:               string(gatewayv1.ListenerConditionProgrammed),
									Status:             metav1.ConditionFalse,
									Reason:             string(gatewayv1.ListenerReasonInvalid),
									ObservedGeneration: 1,
								},
							},
						},
					},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	shared, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: defaultDataplaneNamespace, Name: defaultSharedServiceName},
	)
	if err != nil {
		t.Fatalf("Get shared Service returned error: %v", err)
	}
	assertServicePort(t, shared.Spec.Ports, 80, corev1.ProtocolTCP, 30080)
	assertMissingServicePort(t, shared.Spec.Ports, 443, corev1.ProtocolTCP)

	gatewayService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("edge")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}
	assertServicePort(t, gatewayService.Spec.Ports, 80, corev1.ProtocolTCP, 0)
	assertMissingServicePort(t, gatewayService.Spec.Ports, 443, corev1.ProtocolTCP)

	dataplanePolicy := &networkingv1.NetworkPolicy{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: defaultDataplaneNamespace,
			Name:      defaultDataplaneNetworkPolicyName,
		},
		dataplanePolicy,
	); err != nil {
		t.Fatalf("Get dataplane NetworkPolicy returned error: %v", err)
	}
	assertNetworkPolicyPort(t, dataplanePolicy.Spec.Ingress, 80, corev1.ProtocolTCP)
	assertMissingNetworkPolicyPort(t, dataplanePolicy.Spec.Ingress, 443, corev1.ProtocolTCP)
}
func TestReconcileSkipsGatewayServiceUntilListenerRefsRecover(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")
	mode := gatewayv1.TLSModeTerminate

	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cert-edge",
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
				Conditions: []metav1.Condition{
					{
						Type:               string(gatewayv1.ListenerConditionAccepted),
						Status:             metav1.ConditionTrue,
						ObservedGeneration: 1,
					},
					{
						Type:               string(gatewayv1.ListenerConditionResolvedRefs),
						Status:             metav1.ConditionFalse,
						Reason:             string(gatewayv1.ListenerReasonRefNotPermitted),
						ObservedGeneration: 1,
					},
					{
						Type:               string(gatewayv1.ListenerConditionProgrammed),
						Status:             metav1.ConditionFalse,
						Reason:             string(gatewayv1.ListenerReasonPending),
						ObservedGeneration: 1,
					},
				},
			}},
		},
	}

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gateway,
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	shared := &corev1.Service{}
	err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: defaultDataplaneNamespace, Name: defaultSharedServiceName},
		shared,
	)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected shared Service to be absent while all listener refs are unresolved, got err=%v service=%#v", err, shared.Spec.Ports)
	}

	gatewayService := &corev1.Service{}
	err = k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("cert-edge")},
		gatewayService,
	)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected gateway Service to be absent while listener refs are unresolved, got err=%v service=%#v", err, gatewayService.Spec.Ports)
	}

	gateway.Status.Listeners[0].Conditions[1].Status = metav1.ConditionTrue
	gateway.Status.Listeners[0].Conditions[1].Reason = string(gatewayv1.ListenerReasonResolvedRefs)
	gateway.Status.Listeners[0].Conditions[2].Status = metav1.ConditionTrue
	gateway.Status.Listeners[0].Conditions[2].Reason = string(gatewayv1.ListenerReasonProgrammed)
	if err := k8sClient.Update(context.Background(), gateway); err != nil {
		t.Fatalf("Update Gateway returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("recovery Reconcile returned error: %v", err)
	}

	shared, err = mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: defaultDataplaneNamespace, Name: defaultSharedServiceName},
	)
	if err != nil {
		t.Fatalf("Get shared Service after recovery returned error: %v", err)
	}
	assertServicePort(t, shared.Spec.Ports, 443, corev1.ProtocolTCP, 30443)

	gatewayService, err = mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("cert-edge")},
	)
	if err != nil {
		t.Fatalf("Get gateway Service after recovery returned error: %v", err)
	}
	assertServicePort(t, gatewayService.Spec.Ports, 443, corev1.ProtocolTCP, 0)
}
func TestReconcileUpdatesGatewayInfrastructureServiceForListenerAndMetadataChanges(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
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
					GatewayClassName: "aether-gateway",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						Labels: map[gatewayv1.LabelKey]gatewayv1.LabelValue{
							"example.com/team": "platform",
						},
						Annotations: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"example.com/trace": "v2",
						},
					},
					Listeners: []gatewayv1.Listener{
						{
							Name:     "https",
							Protocol: gatewayv1.HTTPSProtocolType,
							Port:     443,
						},
						{
							Name:     "dns",
							Protocol: gatewayv1.UDPProtocolType,
							Port:     5353,
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gatewayServiceName("public"),
					Namespace: "default",
					Labels: map[string]string{
						managedByLabel:        managedByValue,
						serviceRoleLabel:      serviceRoleGateway,
						gatewayNameLabel:      "public",
						gatewayNamespaceLabel: "default",
						"example.com/team":    "old-team",
						"example.com/old":     "stale",
					},
					Annotations: map[string]string{
						"example.com/trace": "v1",
						"example.com/old":   "stale",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{
							Name:       "tcp-80",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
						},
						{
							Name:       "udp-5300",
							Port:       5300,
							TargetPort: intstr.FromInt(5300),
							Protocol:   corev1.ProtocolUDP,
						},
					},
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

	if service.Labels["example.com/team"] != "platform" {
		t.Fatalf("expected updated label, got %#v", service.Labels)
	}
	if _, ok := service.Labels["example.com/old"]; ok {
		t.Fatalf("expected stale label to be removed, got %#v", service.Labels)
	}
	if service.Annotations["example.com/trace"] != "v2" {
		t.Fatalf("expected updated annotation, got %#v", service.Annotations)
	}
	if _, ok := service.Annotations["example.com/old"]; ok {
		t.Fatalf("expected stale annotation to be removed, got %#v", service.Annotations)
	}

	assertServicePort(t, service.Spec.Ports, 443, corev1.ProtocolTCP, 0)
	assertServicePort(t, service.Spec.Ports, 5353, corev1.ProtocolUDP, 0)
	assertMissingServicePort(t, service.Spec.Ports, 80, corev1.ProtocolTCP)
	assertMissingServicePort(t, service.Spec.Ports, 5300, corev1.ProtocolUDP)
}
func TestReconcilePropagatesGatewayInfrastructureMetadataToEndpointSlices(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway"},
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
					GatewayClassName: "aether-gateway",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						Labels: map[gatewayv1.LabelKey]gatewayv1.LabelValue{
							"example.com/team": "platform",
						},
						Annotations: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"example.com/trace": "enabled",
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
					Name:      "aether-gateway-dataplane-0",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "aether-gateway-dataplane"},
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

	sliceName := frontendEndpointSliceName(
		gatewayEndpointSliceNamePrefix,
		"default",
		gatewayServiceName("public"),
		discoveryv1.AddressTypeIPv4,
	)
	endpointSlice := &discoveryv1.EndpointSlice{}
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: sliceName},
		endpointSlice,
	); err != nil {
		t.Fatalf("Get gateway EndpointSlice returned error: %v", err)
	}

	if endpointSlice.Labels[gatewayNameLabel] != "public" {
		t.Fatalf("expected gateway name label on EndpointSlice, got %#v", endpointSlice.Labels)
	}
	if endpointSlice.Labels["example.com/team"] != "platform" {
		t.Fatalf("expected propagated infrastructure label, got %#v", endpointSlice.Labels)
	}
	if endpointSlice.Labels[serviceRoleLabel] != gatewayEndpointSliceRoleValue {
		t.Fatalf("expected gateway endpoint slice role label, got %#v", endpointSlice.Labels)
	}
	if endpointSlice.Annotations["example.com/trace"] != "enabled" {
		t.Fatalf("expected propagated infrastructure annotation, got %#v", endpointSlice.Annotations)
	}
}
