package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestReconcileCreatesGatewayInfrastructureServiceAndUpdatesSharedPorts(t *testing.T) {
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-with-infrastructure-metadata",
					Namespace: "gateway-conformance-infra",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Infrastructure: &gatewayv1.GatewayInfrastructure{
						Labels: map[gatewayv1.LabelKey]gatewayv1.LabelValue{
							"example.com/team": "edge",
						},
						Annotations: map[gatewayv1.AnnotationKey]gatewayv1.AnnotationValue{
							"example.com/trace": "enabled",
						},
					},
					Listeners: []gatewayv1.Listener{
						{
							Name:     "legacy-http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     80,
						},
						{
							Name:     "http",
							Protocol: gatewayv1.HTTPProtocolType,
							Port:     8080,
						},
						{
							Name:     "dns",
							Protocol: gatewayv1.UDPProtocolType,
							Port:     5300,
						},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      defaultSharedServiceName,
					Namespace: defaultDataplaneNamespace,
				},
				Spec: corev1.ServiceSpec{
					Type:     corev1.ServiceTypeNodePort,
					Selector: map[string]string{"app": "nantian-gw-dataplane"},
					Ports: []corev1.ServicePort{
						{
							Name:       "tcp-80",
							Port:       80,
							TargetPort: intstr.FromInt(80),
							Protocol:   corev1.ProtocolTCP,
							NodePort:   30080,
						},
						{
							Name:       defaultAdminPortName,
							Port:       defaultAdminPort,
							TargetPort: intstr.FromInt(defaultAdminPort),
							Protocol:   corev1.ProtocolTCP,
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

	if shared.Spec.Type != corev1.ServiceTypeNodePort {
		t.Fatalf("shared service type = %s, want NodePort", shared.Spec.Type)
	}
	if shared.Spec.Selector == nil || shared.Spec.Selector["app"] != "nantian-gw-dataplane" {
		t.Fatalf("shared service selector = %#v, want map[app:nantian-gw-dataplane]", shared.Spec.Selector)
	}
	assertServicePort(t, shared.Spec.Ports, 80, corev1.ProtocolTCP, 0)
	assertServicePort(t, shared.Spec.Ports, 8080, corev1.ProtocolTCP, 0)
	assertServicePort(t, shared.Spec.Ports, 5300, corev1.ProtocolUDP, 0)
	assertMissingServicePort(t, shared.Spec.Ports, defaultAdminPort, corev1.ProtocolTCP)

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
	assertNetworkPolicyPort(t, dataplanePolicy.Spec.Ingress, 8080, corev1.ProtocolTCP)
	assertNetworkPolicyPort(t, dataplanePolicy.Spec.Ingress, 5300, corev1.ProtocolUDP)
	assertAdminNetworkPolicyRule(t, dataplanePolicy.Spec.Ingress, defaultDataplaneNamespace)

	gatewayService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{
			Namespace: "gateway-conformance-infra",
			Name:      gatewayServiceName("gateway-with-infrastructure-metadata"),
		},
	)
	if err != nil {
		t.Fatalf("Get gateway Service returned error: %v", err)
	}

	if gatewayService.Labels[gatewayNameLabel] != "gateway-with-infrastructure-metadata" {
		t.Fatalf("gateway-name label = %q", gatewayService.Labels[gatewayNameLabel])
	}
	if gatewayService.Labels["example.com/team"] != "edge" {
		t.Fatalf("expected propagated label, got %#v", gatewayService.Labels)
	}
	if gatewayService.Annotations["example.com/trace"] != "enabled" {
		t.Fatalf("expected propagated annotation, got %#v", gatewayService.Annotations)
	}
	if gatewayService.Spec.Selector != nil {
		t.Fatalf("gateway service selector = %#v, want nil with managed EndpointSlices", gatewayService.Spec.Selector)
	}
	assertServicePort(t, gatewayService.Spec.Ports, 8080, corev1.ProtocolTCP, 0)
	assertServicePort(t, gatewayService.Spec.Ports, 5300, corev1.ProtocolUDP, 0)
}

func TestReconcileListsDataplanePodsOncePerRun(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	baseClient := newInfrastructureClientBuilder(scheme).
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

	podLists := 0
	reconciler := New(
		validatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func(client.ListOptions) error{
				reflect.TypeOf(&corev1.PodList{}): func(opts client.ListOptions) error {
					podLists++
					if opts.Namespace != defaultDataplaneNamespace {
						return fmt.Errorf("pod lookup namespace = %q, want %q", opts.Namespace, defaultDataplaneNamespace)
					}
					if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
						return fmt.Errorf("pod lookup must include dataplane selector")
					}
					if !opts.LabelSelector.Matches(labels.Set(defaultDataplaneSelector)) {
						return fmt.Errorf("selector %q does not match dataplane selector", opts.LabelSelector.String())
					}
					return nil
				},
			},
		},
		string(controllerName),
		discardLogger(),
	)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if podLists != 1 {
		t.Fatalf("dataplane Pod lookup count = %d, want 1", podLists)
	}
}

func TestInfrastructureReconcileCreatesSpan(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	reconciler := New(newInfrastructureClientBuilder(newScheme(t)).Build(), "gateway.networking.k8s.io/nantian-gw", discardLogger())
	_ = reconciler.Reconcile(context.Background())

	if !slices.Contains(spanNames(exporter.GetSpans()), "controlplane.infrastructure.reconcile") {
		t.Fatalf("expected infrastructure reconcile span")
	}
}

func TestInfrastructureReconcileSpanRecordsGatewayServiceResult(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	reconciler := New(newInfrastructureClientBuilder(newScheme(t)).Build(), "gateway.networking.k8s.io/nantian-gw", discardLogger())
	_ = reconciler.Reconcile(context.Background())

	span, ok := spanByName(exporter.GetSpans(), "controlplane.infrastructure.reconcile")
	if !ok {
		t.Fatal("expected infrastructure reconcile span")
	}
	if !spanHasAttr(span, "infrastructure.gateway_services_failed") {
		t.Fatal("expected infrastructure.gateway_services_failed attribute")
	}
	if got := spanBoolAttr(span, "infrastructure.gateway_services_failed"); got {
		t.Fatal("expected empty infrastructure reconcile to complete without gateway service failure")
	}
}

func TestInfrastructureReconcileSpanRecordsGatewayServiceFailure(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	baseClient := newInfrastructureClientBuilder(scheme).
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
		).
		Build()

	wantErr := errors.New("gateway services list failed")
	reconciler := New(
		validatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func(client.ListOptions) error{
				reflect.TypeOf(&corev1.ServiceList{}): func(opts client.ListOptions) error {
					wantLabels := map[string]string{
						managedByLabel:   managedByValue,
						serviceRoleLabel: serviceRoleGateway,
					}
					if opts.LabelSelector == nil || !opts.LabelSelector.Matches(labels.Set(wantLabels)) {
						return nil
					}
					return wantErr
				},
			},
		},
		string(controllerName),
		discardLogger(),
	)

	err := reconciler.Reconcile(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Reconcile error = %v, want %v", err, wantErr)
	}

	span, ok := spanByName(exporter.GetSpans(), "controlplane.infrastructure.reconcile")
	if !ok {
		t.Fatal("expected infrastructure reconcile span")
	}
	if !spanHasAttr(span, "infrastructure.gateway_services_failed") {
		t.Fatal("expected infrastructure.gateway_services_failed attribute")
	}
	if got := spanBoolAttr(span, "infrastructure.gateway_services_failed"); !got {
		t.Fatal("expected infrastructure reconcile to record gateway service failure")
	}
}

func TestSharedNodePortForStaysInsideDefaultNodePortRange(t *testing.T) {
	tests := []struct {
		name     string
		port     int32
		protocol corev1.Protocol
		want     int32
	}{
		{name: "privileged tcp port", port: 80, protocol: corev1.ProtocolTCP, want: 30080},
		{name: "common high tcp port", port: 8080, protocol: corev1.ProtocolTCP, want: 32080},
		{name: "kind bridged tls backend port", port: 8443, protocol: corev1.ProtocolTCP, want: 32443},
		{name: "conformance mixed tls port wraps into range", port: 8883, protocol: corev1.ProtocolTCP, want: 31883},
		{name: "udp port", port: 5300, protocol: corev1.ProtocolUDP, want: 31300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sharedNodePortFor(tt.port, tt.protocol, DefaultOptions())
			if got != tt.want {
				t.Fatalf("sharedNodePortFor(%d, %s) = %d, want %d", tt.port, tt.protocol, got, tt.want)
			}
			if got < 30000 || got > 32767 {
				t.Fatalf("sharedNodePortFor(%d, %s) = %d, outside default NodePort range", tt.port, tt.protocol, got)
			}
		})
	}
}

func spanNames(spans tracetest.SpanStubs) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name)
	}
	return names
}

func spanByName(spans tracetest.SpanStubs, name string) (tracetest.SpanStub, bool) {
	for _, span := range spans {
		if span.Name == name {
			return span, true
		}
	}
	return tracetest.SpanStub{}, false
}

func spanBoolAttr(span tracetest.SpanStub, key string) bool {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsBool()
		}
	}
	return false
}

func spanHasAttr(span tracetest.SpanStub, key string) bool {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return true
		}
	}
	return false
}

func TestReconcileTwoGatewaysSamePortNoConflict(t *testing.T) {
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-a",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{Name: "http-a", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
					},
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-b",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{Name: "http-b", Protocol: gatewayv1.HTTPProtocolType, Port: 80},
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
	assertServicePort(t, shared.Spec.Ports, 80, corev1.ProtocolTCP, 0)

	gatewayAService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("gateway-a")},
	)
	if err != nil {
		t.Fatalf("Get gateway-a Service returned error: %v", err)
	}
	assertServicePort(t, gatewayAService.Spec.Ports, 80, corev1.ProtocolTCP, 0)

	gatewayBService, err := mustGetService(
		context.Background(),
		k8sClient,
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("gateway-b")},
	)
	if err != nil {
		t.Fatalf("Get gateway-b Service returned error: %v", err)
	}
	assertServicePort(t, gatewayBService.Spec.Ports, 80, corev1.ProtocolTCP, 0)
}

func TestReconcileSkipsGatewayServiceForUnsupportedProtocol(t *testing.T) {
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
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-unsupported-proto",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{
						{
							Name:     "custom",
							Protocol: gatewayv1.ProtocolType("CUSTOM"),
							Port:     9999,
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

	err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: gatewayServiceName("gateway-unsupported-proto")},
		&corev1.Service{},
	)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected gateway Service to not exist for unsupported protocol, got err=%v", err)
	}
}
