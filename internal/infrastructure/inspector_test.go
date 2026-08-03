package infrastructure

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/mesh"
)

func TestInspectReportsDerivedInfrastructureLifecycleAndDrift(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)
	httpProtocol := "http"

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
					Labels: map[string]string{
						gatewayNameLabel:      "public",
						gatewayNamespaceLabel: "default",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{{
						Name:       "tcp-81",
						Port:       81,
						TargetPort: intstr.FromInt(81),
						Protocol:   corev1.ProtocolTCP,
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
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "echo"},
					Ports: []corev1.ServicePort{{
						Name:        "http",
						Port:        80,
						TargetPort:  intstr.FromInt(8080),
						Protocol:    corev1.ProtocolTCP,
						AppProtocol: &httpProtocol,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mesh",
					Namespace: "default",
				},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Kind: &serviceKind,
							Name: "echo",
							Port: &servicePort,
						}},
					},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mesh.ShadowServiceName("default", "stale"),
					Namespace: "default",
					Labels: map[string]string{
						managedByLabel:                     managedByValue,
						mesh.ShadowServiceRoleLabel:        mesh.ShadowServiceRoleValue,
						mesh.OriginalServiceNamespaceLabel: "default",
						mesh.OriginalServiceNameLabel:      "stale",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), discardLogger())
	report, err := reconciler.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	if report.Summary.ResourceCount != 8 {
		t.Fatalf("expected 8 derived resource records, got %+v", report.Summary)
	}
	if report.Summary.MissingCount != 5 || report.Summary.DriftedCount != 2 || report.Summary.OrphanCount != 1 {
		t.Fatalf("unexpected lifecycle/drift summary: %+v", report.Summary)
	}
	expectedWarnings := []string{
		"5 derived infrastructure resources are missing",
		"2 derived infrastructure resources have drifted from desired state",
		"1 managed infrastructure resources are orphaned",
		"1 gateways are waiting for derived Service metadata convergence",
	}
	for _, warning := range expectedWarnings {
		if !containsString(report.Warnings, warning) {
			t.Fatalf("expected warning %q, got %+v", warning, report.Warnings)
		}
	}

	sharedService := mustFindInfrastructureResource(
		t,
		report,
		InfrastructureKindService,
		defaultDataplaneNamespace,
		defaultSharedServiceName,
	)
	if sharedService.State != InfrastructureStateMissing {
		t.Fatalf("expected shared service to be missing, got %+v", sharedService)
	}

	gatewayService := mustFindInfrastructureResource(
		t,
		report,
		InfrastructureKindService,
		"default",
		gatewayServiceName("public"),
	)
	if gatewayService.State != InfrastructureStateDrifted {
		t.Fatalf("expected gateway service to be drifted, got %+v", gatewayService)
	}
	if !containsString(gatewayService.Reasons, "labels differ") || !containsString(gatewayService.Reasons, "ports differ") {
		t.Fatalf("expected drift reasons on gateway service, got %+v", gatewayService)
	}

	sharedSlice := mustFindInfrastructureResource(
		t,
		report,
		InfrastructureKindSlice,
		defaultDataplaneNamespace,
		frontendEndpointSliceName(
			sharedEndpointSliceNamePrefix,
			defaultDataplaneNamespace,
			defaultSharedServiceName,
			discoveryv1.AddressTypeIPv4,
		),
	)
	if sharedSlice.State != InfrastructureStateMissing {
		t.Fatalf("expected shared EndpointSlice to be missing, got %+v", sharedSlice)
	}

	gatewaySlice := mustFindInfrastructureResource(
		t,
		report,
		InfrastructureKindSlice,
		"default",
		frontendEndpointSliceName(
			gatewayEndpointSliceNamePrefix,
			"default",
			gatewayServiceName("public"),
			discoveryv1.AddressTypeIPv4,
		),
	)
	if gatewaySlice.State != InfrastructureStateMissing {
		t.Fatalf("expected gateway EndpointSlice to be missing, got %+v", gatewaySlice)
	}

	meshFrontend := mustFindInfrastructureResource(t, report, InfrastructureKindService, "default", "echo")
	if meshFrontend.State != InfrastructureStateDrifted {
		t.Fatalf("expected mesh frontend service to be drifted, got %+v", meshFrontend)
	}
	if !containsString(meshFrontend.Reasons, "annotations differ") ||
		!containsString(meshFrontend.Reasons, "selector differs") ||
		!containsString(meshFrontend.Reasons, "ports differ") {
		t.Fatalf("expected mesh frontend drift reasons, got %+v", meshFrontend)
	}

	meshShadow := mustFindInfrastructureResource(
		t,
		report,
		InfrastructureKindService,
		"default",
		mesh.ShadowServiceName("default", "echo"),
	)
	if meshShadow.State != InfrastructureStateMissing {
		t.Fatalf("expected mesh shadow service to be missing, got %+v", meshShadow)
	}

	meshSlice := mustFindInfrastructureResource(
		t,
		report,
		InfrastructureKindSlice,
		"default",
		meshEndpointSliceName("default", "echo", discoveryv1.AddressTypeIPv4),
	)
	if meshSlice.State != InfrastructureStateMissing {
		t.Fatalf("expected mesh EndpointSlice to be missing, got %+v", meshSlice)
	}

	orphanShadow := mustFindInfrastructureResource(
		t,
		report,
		InfrastructureKindService,
		"default",
		mesh.ShadowServiceName("default", "stale"),
	)
	if orphanShadow.State != InfrastructureStateOrphan {
		t.Fatalf("expected stale shadow service to be orphaned, got %+v", orphanShadow)
	}
}

func TestInspectScopesObservedServiceAndEndpointSliceQueries(t *testing.T) {
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

	reconciler := New(
		validatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func(client.ListOptions) error{
				reflect.TypeOf(&corev1.ServiceList{}): func(opts client.ListOptions) error {
					if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
						return fmt.Errorf("service lookup must include managed infrastructure selector")
					}
					for _, role := range []string{
						serviceRoleShared,
						serviceRoleGateway,
						serviceRoleMeshFrontend,
						mesh.ShadowServiceRoleValue,
					} {
						if !opts.LabelSelector.Matches(labels.Set{
							managedByLabel:   managedByValue,
							serviceRoleLabel: role,
						}) {
							return fmt.Errorf("selector %q does not match service role %s", opts.LabelSelector.String(), role)
						}
					}
					if opts.LabelSelector.Matches(labels.Set{
						managedByLabel:   managedByValue,
						serviceRoleLabel: "other",
					}) {
						return fmt.Errorf("selector %q is broader than infrastructure service roles", opts.LabelSelector.String())
					}
					return nil
				},
				reflect.TypeOf(&discoveryv1.EndpointSliceList{}): func(opts client.ListOptions) error {
					if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
						return fmt.Errorf("EndpointSlice lookup must include managed infrastructure selector")
					}
					for _, role := range []string{
						sharedEndpointSliceRoleValue,
						gatewayEndpointSliceRoleValue,
						meshEndpointSliceRoleValue,
					} {
						if !opts.LabelSelector.Matches(labels.Set{
							discoveryv1.LabelManagedBy: managedByValue,
							serviceRoleLabel:           role,
						}) {
							return fmt.Errorf("selector %q does not match EndpointSlice role %s", opts.LabelSelector.String(), role)
						}
					}
					if opts.LabelSelector.Matches(labels.Set{
						discoveryv1.LabelManagedBy: managedByValue,
						serviceRoleLabel:           "other",
					}) {
						return fmt.Errorf("selector %q is broader than infrastructure EndpointSlice roles", opts.LabelSelector.String())
					}
					return nil
				},
			},
		},
		string(controllerName),
		discardLogger(),
	)

	if _, err := reconciler.Inspect(context.Background()); err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
}

func TestInspectCachesGatewayClassParametersAcrossGateways(t *testing.T) {
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

	if _, err := reconciler.Inspect(context.Background()); err != nil {
		t.Fatalf("Inspect returned error: %v", err)
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

func TestInspectUsesServiceParentRouteIndexes(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	baseClient := withInfrastructureGatewayIndexes(withInfrastructureRouteParentIndexes(
		fake.NewClientBuilder().
			WithScheme(scheme).
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
						Name:      "echo",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						}},
					},
				},
				&gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mesh",
						Namespace: "default",
					},
					Spec: gatewayv1.HTTPRouteSpec{
						CommonRouteSpec: gatewayv1.CommonRouteSpec{
							ParentRefs: []gatewayv1.ParentReference{{
								Kind: &serviceKind,
								Name: "echo",
								Port: &servicePort,
							}},
						},
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
			),
	)).Build()

	reconciler := New(
		rawValidatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func([]client.ListOption) error{
				reflect.TypeOf(&gatewayv1.HTTPRouteList{}):      requireMatchingField(httpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1.GRPCRouteList{}):      requireMatchingField(grpcRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}): requireMatchingField(tcpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}): requireMatchingField(udpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}): requireMatchingField(tlsRouteServiceParentIndex, serviceParentIndexMarker),
			},
		},
		string(controllerName),
		discardLogger(),
	)

	if _, err := reconciler.Inspect(context.Background()); err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
}

func mustFindInfrastructureResource(
	t *testing.T,
	report InfrastructureReport,
	kind string,
	namespace string,
	name string,
) InfrastructureResource {
	t.Helper()

	for _, item := range report.Resources {
		if item.Kind == kind && item.Namespace == namespace && item.Name == name {
			return item
		}
	}

	t.Fatalf("resource %s %s/%s not found in %+v", kind, namespace, name, report.Resources)
	return InfrastructureResource{}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == needle {
			return true
		}
	}
	return false
}
