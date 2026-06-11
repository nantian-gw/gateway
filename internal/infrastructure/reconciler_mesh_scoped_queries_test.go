package infrastructure

import (
	"context"
	"fmt"
	"reflect"
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

func TestReconcileMeshServicesScopesServiceAndEndpointLookups(t *testing.T) {
	scheme := newScheme(t)
	serviceKind := gatewayv1.Kind("Service")
	servicePort := gatewayv1.PortNumber(80)

	baseClient := withInfrastructureRouteParentIndexes(
		fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
					Spec: corev1.ServiceSpec{
						Selector: map[string]string{"app": "echo"},
						Ports: []corev1.ServicePort{{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(8080),
							Protocol:   corev1.ProtocolTCP,
						}},
					},
				},
				&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "orphan",
						Namespace: "default",
						Labels: map[string]string{
							managedByLabel:   managedByValue,
							serviceRoleLabel: serviceRoleMeshFrontend,
						},
						Annotations: map[string]string{
							mesh.ManagedServiceAnnotation: "true",
						},
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{{
							Name:       "http",
							Port:       81,
							TargetPort: intstr.FromInt(8081),
							Protocol:   corev1.ProtocolTCP,
						}},
					},
				},
				&gatewayv1.HTTPRoute{
					ObjectMeta: metav1.ObjectMeta{Name: "mesh", Namespace: "default"},
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
				&discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      meshEndpointSliceName("default", "orphan", discoveryv1.AddressTypeIPv4),
						Namespace: "default",
						Labels: map[string]string{
							discoveryv1.LabelManagedBy:   managedByValue,
							discoveryv1.LabelServiceName: "orphan",
							serviceRoleLabel:             meshEndpointSliceRoleValue,
						},
					},
					AddressType: discoveryv1.AddressTypeIPv4,
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
			),
	).Build()

	seenManagedServices := 0
	seenSliceLookups := make(map[string]int)
	reconciler := New(
		rawValidatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func([]client.ListOption) error{
				reflect.TypeOf(&gatewayv1.HTTPRouteList{}):      requireMatchingField(httpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1.GRPCRouteList{}):      requireMatchingField(grpcRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}): requireMatchingField(tcpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}): requireMatchingField(udpRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}): requireMatchingField(tlsRouteServiceParentIndex, serviceParentIndexMarker),
				reflect.TypeOf(&corev1.ServiceList{}): requireAppliedListOptions(func(opts client.ListOptions) error {
					seenManagedServices++
					if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
						return fmt.Errorf("mesh Service lookup must include a scoped label selector")
					}
					if !opts.LabelSelector.Matches(labels.Set{
						managedByLabel:   managedByValue,
						serviceRoleLabel: serviceRoleMeshFrontend,
					}) {
						return fmt.Errorf(
							"selector %q does not match mesh frontend Services",
							opts.LabelSelector.String(),
						)
					}
					if !opts.LabelSelector.Matches(labels.Set{
						managedByLabel:   managedByValue,
						serviceRoleLabel: mesh.ShadowServiceRoleValue,
					}) {
						return fmt.Errorf(
							"selector %q does not match mesh shadow Services",
							opts.LabelSelector.String(),
						)
					}
					if opts.LabelSelector.Matches(labels.Set{
						managedByLabel:   managedByValue,
						serviceRoleLabel: serviceRoleGateway,
					}) {
						return fmt.Errorf("selector %q is broader than mesh Services", opts.LabelSelector.String())
					}
					if opts.LabelSelector.Matches(labels.Set{
						managedByLabel:   managedByValue,
						serviceRoleLabel: serviceRoleShared,
					}) {
						return fmt.Errorf("selector %q is broader than mesh Services", opts.LabelSelector.String())
					}
					return nil
				}),
				reflect.TypeOf(&corev1.EndpointsList{}): func([]client.ListOption) error {
					return fmt.Errorf("mesh reconcile must use Get for Endpoints")
				},
				reflect.TypeOf(&discoveryv1.EndpointSliceList{}): requireMeshEndpointSliceQueries(
					map[string][]string{"default": {"echo", "orphan"}},
					seenSliceLookups,
				),
			},
		},
		"gateway.networking.k8s.io/nantian-gw",
		discardLogger(),
	)

	if err := reconciler.reconcileMeshServices(context.Background()); err != nil {
		t.Fatalf("reconcileMeshServices returned error: %v", err)
	}

	if seenManagedServices != 1 {
		t.Fatalf("managed Service lookup count = %d, want 1", seenManagedServices)
	}
	if seenSliceLookups[serviceKey("default", "echo")] != 1 {
		t.Fatalf(
			"default/echo EndpointSlice lookup count = %d, want 1",
			seenSliceLookups[serviceKey("default", "echo")],
		)
	}
	if seenSliceLookups[serviceKey("default", "orphan")] != 1 {
		t.Fatalf(
			"default/orphan EndpointSlice lookup count = %d, want 1",
			seenSliceLookups[serviceKey("default", "orphan")],
		)
	}

	orphanSlice := &discoveryv1.EndpointSlice{}
	if err := baseClient.Get(
		context.Background(),
		client.ObjectKey{
			Namespace: "default",
			Name:      meshEndpointSliceName("default", "orphan", discoveryv1.AddressTypeIPv4),
		},
		orphanSlice,
	); client.IgnoreNotFound(err) != nil {
		t.Fatalf("Get orphan mesh EndpointSlice returned error: %v", err)
	}
	if orphanSlice.Name != "" {
		t.Fatalf("expected orphan mesh EndpointSlice to be deleted, got %#v", orphanSlice.Labels)
	}
}

func TestLoadMeshServiceParentsStandardModeSkipsExperimentalRouteLists(t *testing.T) {
	scheme := newScheme(t)
	baseClient := withInfrastructureRouteParentIndexes(
		fake.NewClientBuilder().WithScheme(scheme),
	).Build()
	reconciler := NewWithOptions(
		rawValidatingClient{
			Client: baseClient,
			listValidators: map[reflect.Type]func([]client.ListOption) error{
				reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}): func([]client.ListOption) error {
					return fmt.Errorf("standard mode should not list TCPRoutes")
				},
				reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}): func([]client.ListOption) error {
					return fmt.Errorf("standard mode should not list UDPRoutes")
				},
				reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}): func([]client.ListOption) error {
					return fmt.Errorf("standard mode should not list TLSRoutes")
				},
			},
		},
		nil,
		"gateway.networking.k8s.io/nantian-gw",
		Options{EnableExperimentalGateway: false},
		discardLogger(),
	)

	if _, err := reconciler.loadMeshServiceParents(context.Background()); err != nil {
		t.Fatalf("loadMeshServiceParents returned error: %v", err)
	}
}

func TestLoadMeshFrontendNetworkPolicyPortsScopesManagedMeshServices(t *testing.T) {
	scheme := newScheme(t)

	ports, err := loadMeshFrontendNetworkPolicyPorts(
		context.Background(),
		validatingClient{
			Client: newInfrastructureClientBuilder(scheme).
				WithObjects(
					&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "mesh",
							Namespace: "default",
							Labels: map[string]string{
								managedByLabel:   managedByValue,
								serviceRoleLabel: serviceRoleMeshFrontend,
							},
							Annotations: map[string]string{
								mesh.ManagedServiceAnnotation: "true",
							},
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{{
								Name:       "http",
								Port:       80,
								TargetPort: intstr.FromInt(18080),
								Protocol:   corev1.ProtocolTCP,
							}},
						},
					},
					&corev1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "ignored",
							Namespace: "default",
						},
						Spec: corev1.ServiceSpec{
							Ports: []corev1.ServicePort{{
								Name:       "http",
								Port:       81,
								TargetPort: intstr.FromInt(18081),
								Protocol:   corev1.ProtocolTCP,
							}},
						},
					},
				).
				Build(),
			listValidators: map[reflect.Type]func(client.ListOptions) error{
				reflect.TypeOf(&corev1.ServiceList{}): func(opts client.ListOptions) error {
					if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
						return fmt.Errorf("list must include managed mesh service label selector")
					}
					if !opts.LabelSelector.Matches(labels.Set{
						managedByLabel:   managedByValue,
						serviceRoleLabel: serviceRoleMeshFrontend,
					}) {
						return fmt.Errorf(
							"selector %q does not match managed mesh service labels",
							opts.LabelSelector.String(),
						)
					}
					if opts.LabelSelector.Matches(labels.Set{
						managedByLabel:   managedByValue,
						serviceRoleLabel: serviceRoleGateway,
					}) {
						return fmt.Errorf("selector %q is broader than mesh frontend services", opts.LabelSelector.String())
					}
					return nil
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("loadMeshFrontendNetworkPolicyPorts returned error: %v", err)
	}

	if len(ports) != 1 {
		t.Fatalf("expected 1 managed mesh service port, got %#v", ports)
	}
	if ports[0].port != 18080 || ports[0].protocol != corev1.ProtocolTCP {
		t.Fatalf("unexpected managed mesh service port: %#v", ports[0])
	}
}

func requireAppliedListOptions(validator func(client.ListOptions) error) func([]client.ListOption) error {
	return func(opts []client.ListOption) error {
		var listOptions client.ListOptions
		for _, opt := range opts {
			opt.ApplyToList(&listOptions)
		}
		return validator(listOptions)
	}
}

func requireMeshEndpointSliceQueries(
	expectedByNamespace map[string][]string,
	seen map[string]int,
) func([]client.ListOption) error {
	return requireAppliedListOptions(func(opts client.ListOptions) error {
		serviceNames, ok := expectedByNamespace[opts.Namespace]
		if !ok {
			return fmt.Errorf("unexpected EndpointSlice lookup namespace %q", opts.Namespace)
		}
		if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
			return fmt.Errorf("EndpointSlice lookup for namespace %q must include service-name selector", opts.Namespace)
		}

		for _, serviceName := range serviceNames {
			if !opts.LabelSelector.Matches(labels.Set{discoveryv1.LabelServiceName: serviceName}) {
				return fmt.Errorf("selector %q does not match service-name=%s", opts.LabelSelector.String(), serviceName)
			}
			if opts.LabelSelector.Matches(labels.Set{discoveryv1.LabelServiceName: serviceName + "-other"}) {
				return fmt.Errorf("selector %q is broader than service-name=%s", opts.LabelSelector.String(), serviceName)
			}
			seen[serviceKey(opts.Namespace, serviceName)]++
		}

		return nil
	})
}
