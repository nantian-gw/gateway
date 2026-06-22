package translator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gwapi"
	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
)

func TestBuildLoadsReferencedSecretsAndConfigMapsOnDemand(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate
	portNumber := gatewayv1.PortNumber(8080)

	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					TLS: &gatewayv1.GatewayTLSConfig{
						Frontend: &gatewayv1.FrontendTLSConfig{
							Default: gatewayv1.TLSConfig{
								Validation: &gatewayv1.FrontendTLSValidation{
									CACertificateRefs: []gatewayv1.ObjectReference{{
										Name: "client-ca",
									}},
								},
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "https",
						Protocol: gatewayv1.HTTPSProtocolType,
						Port:     443,
						TLS: &gatewayv1.ListenerTLSConfig{
							Mode: &mode,
							CertificateRefs: []gatewayv1.SecretObjectReference{{
								Name: "example-cert",
							}},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:        "gw",
							SectionName: ptr[gatewayv1.SectionName]("https"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						Filters: []gatewayv1.HTTPRouteFilter{{
							Type: gatewayv1.HTTPRouteFilterExtensionRef,
							ExtensionRef: &gatewayv1.LocalObjectReference{
								Kind: "ConfigMap",
								Name: "route-filter",
							},
						}},
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-1",
					Namespace: "default",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
				}},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "example-cert",
					Namespace: "default",
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readTestTLSAsset(t, "client.crt"),
					"tls.key": readTestTLSAsset(t, "client.key"),
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-ca",
					Namespace: "default",
				},
				Data: map[string]string{
					"ca.crt": "PEM-DATA",
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "route-filter",
					Namespace: "default",
				},
				Data: map[string]string{
					extfilter.ConfigMapDataKey: `
type: RequestHeaderModifier
headerModifier:
  add:
    - name: x-test
      value: enabled
`,
				},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), validatingTranslatorClient{
		Client: baseClient,
		forbiddenLists: map[reflect.Type]string{
			reflect.TypeOf(&corev1.SecretList{}):    "Build should load referenced Secrets on demand",
			reflect.TypeOf(&corev1.ConfigMapList{}): "Build should load referenced ConfigMaps on demand",
			reflect.TypeOf(&corev1.NamespaceList{}): "Build should not list Namespaces for same-namespace attachments",
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}

	listenerTLS := snapshot.Listeners[0].TLS
	if listenerTLS == nil {
		t.Fatal("expected listener TLS config")
	}
	if len(listenerTLS.SecretRefs) != 1 || listenerTLS.SecretRefs[0] != "default/example-cert" {
		t.Fatalf("unexpected secret refs: %#v", listenerTLS.SecretRefs)
	}
	if listenerTLS.FrontendValidation == nil || len(listenerTLS.FrontendValidation.ClientCAPEMs) != 1 {
		t.Fatalf("unexpected frontend validation: %#v", listenerTLS.FrontendValidation)
	}
	if listenerTLS.FrontendValidation.ClientCAPEMs[0] != "PEM-DATA" {
		t.Fatalf("unexpected frontend validation CA: %#v", listenerTLS.FrontendValidation.ClientCAPEMs)
	}

	if got := findSnapshotSecret(t, snapshot, "default", "example-cert").CertPEM; got != string(readTestTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected translated secret material: %q", got)
	}

	if len(snapshot.HTTPRoutes) != 1 || len(snapshot.HTTPRoutes[0].Rules) != 1 || len(snapshot.HTTPRoutes[0].Rules[0].Filters) != 1 {
		t.Fatalf("unexpected translated route filters: %#v", snapshot.HTTPRoutes)
	}
	if snapshot.HTTPRoutes[0].Rules[0].Filters[0].Type != string(gatewayv1.HTTPRouteFilterRequestHeaderModifier) {
		t.Fatalf("expected resolved extension filter, got %#v", snapshot.HTTPRoutes[0].Rules[0].Filters[0])
	}
}

func newTranslatorClientBuilder(scheme *runtime.Scheme) *fake.ClientBuilder {
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, gatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, gatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithIndex(&gatewayv1.ListenerSet{}, listenerSetParentGatewayFieldIndex, listenerSetParentGatewayIndexKeys)
}
func TestBuildLoadsAttachmentNamespacesOnDemand(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	namespaceMode := gatewayv1.NamespacesFromSelector
	portNumber := gatewayv1.PortNumber(8080)

	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "apps",
					Labels: map[string]string{"tenant": "edge"},
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "infra"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
						AllowedRoutes: &gatewayv1.AllowedRoutes{
							Namespaces: &gatewayv1.RouteNamespaces{
								From: &namespaceMode,
								Selector: &metav1.LabelSelector{
									MatchLabels: map[string]string{"tenant": "edge"},
								},
							},
						},
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "apps"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:      "gw",
							Namespace: ptr[gatewayv1.Namespace]("infra"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &portNumber,
								},
							},
						}},
					}},
				},
			},
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "apps"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       8080,
						TargetPort: intstr.FromInt(8080),
						Protocol:   corev1.ProtocolTCP,
					}},
				},
			},
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "echo-1",
					Namespace: "apps",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "echo",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](8080)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.10"},
				}},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), validatingTranslatorClient{
		Client: baseClient,
		forbiddenLists: map[reflect.Type]string{
			reflect.TypeOf(&corev1.NamespaceList{}): "Build should load route Namespaces on demand",
			reflect.TypeOf(&corev1.SecretList{}):    "Build should not list Secrets when no secret refs exist",
			reflect.TypeOf(&corev1.ConfigMapList{}): "Build should not list ConfigMaps when no configmap refs exist",
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(snapshot.Listeners))
	}
	if got := snapshot.Listeners[0].AttachedRoutes; len(got) != 1 || got[0] != "apps/route" {
		t.Fatalf("unexpected attached routes: %#v", got)
	}
}
func TestBuildUsesScopedGatewayListsWhenManagedGatewayClassesExist(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, testGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok || gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, testGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok || gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "other"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "public", Namespace: "default"},
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
				ObjectMeta: metav1.ObjectMeta{Name: "ignored", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "other",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
		).
		Build()

	snapshot, err := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), scopedGatewayQueryValidatingClient{
		Client:         baseClient,
		controllerName: string(controllerName),
		classNames: map[string]struct{}{
			"nantian-gw": {},
		},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if len(snapshot.Listeners) != 1 {
		t.Fatalf("expected 1 listener from managed gateway classes, got %d", len(snapshot.Listeners))
	}
	if snapshot.Listeners[0].Name != "default/public/http" {
		t.Fatalf("unexpected translated listener set: %#v", snapshot.Listeners)
	}
}

func buildSupportScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	must(gatewayv1.Install(scheme), t)
	must(gatewayv1alpha2.Install(scheme), t)
	must(gatewayv1alpha3.Install(scheme), t)
	must(gatewayv1beta1.Install(scheme), t)
	must(backendlb.Install(scheme), t)
	must(mcsv1alpha1.AddToScheme(scheme), t)
	must(corev1.AddToScheme(scheme), t)
	must(discoveryv1.AddToScheme(scheme), t)
	return scheme
}

type validatingTranslatorClient struct {
	client.Client
	forbiddenLists map[reflect.Type]string
}

func (c validatingTranslatorClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if message, ok := c.forbiddenLists[reflect.TypeOf(list)]; ok {
		return fmt.Errorf("unexpected List for %T: %s", list, message)
	}
	return c.Client.List(ctx, list, opts...)
}

type scopedBuildDependencyValidatingTranslatorClient struct {
	client.Client
	expectedPodNamespaces map[string]struct{}
}

func (c scopedBuildDependencyValidatingTranslatorClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}

	switch typed := list.(type) {
	case *corev1.PodList:
		if len(c.expectedPodNamespaces) == 0 {
			return fmt.Errorf("Build should not list Pods when no mesh route namespaces are referenced")
		}
		if listOptions.Namespace == "" {
			return fmt.Errorf("Pod list must be namespace-scoped")
		}
		if _, ok := c.expectedPodNamespaces[listOptions.Namespace]; !ok {
			return fmt.Errorf("unexpected Pod list namespace %q", listOptions.Namespace)
		}
	case *gatewayv1beta1.ReferenceGrantList:
		if listOptions.Namespace == "" {
			return fmt.Errorf("ReferenceGrant list must be namespace-scoped")
		}
	case *backendlb.BackendLBPolicyList:
		if listOptions.Namespace == "" {
			return fmt.Errorf("BackendLBPolicy list must be namespace-scoped")
		}
	case *gatewayv1alpha3.BackendTLSPolicyList:
		if listOptions.Namespace == "" {
			return fmt.Errorf("BackendTLSPolicy typed list must be namespace-scoped")
		}
	case *unstructured.UnstructuredList:
		if typed.GroupVersionKind() == gwapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") &&
			listOptions.Namespace == "" {
			return fmt.Errorf("BackendTLSPolicy list must be namespace-scoped")
		}
	}

	return c.Client.List(ctx, list, opts...)
}

type fakeScopedPolicyListValidatingTranslatorClient struct {
	client.Client
}

func (c fakeScopedPolicyListValidatingTranslatorClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	namespace := listNamespace(opts)
	switch typed := list.(type) {
	case *backendlb.BackendLBPolicyList:
		if namespace == "" {
			return fmt.Errorf("BackendLBPolicy list must be namespace-scoped")
		}
	case *gatewayv1alpha3.BackendTLSPolicyList:
		if namespace == "" {
			return fmt.Errorf("BackendTLSPolicy typed list must be namespace-scoped")
		}
	case *unstructured.UnstructuredList:
		if typed.GroupVersionKind() == gwapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") &&
			namespace == "" {
			return fmt.Errorf("BackendTLSPolicy list must be namespace-scoped")
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func listNamespace(opts []client.ListOption) string {
	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	return listOptions.Namespace
}

type fakeScopedReferenceGrantValidatingTranslatorClient struct {
	client.Client
}

func (c fakeScopedReferenceGrantValidatingTranslatorClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if _, ok := list.(*gatewayv1beta1.ReferenceGrantList); ok && listNamespace(opts) == "" {
		return fmt.Errorf("ReferenceGrant list must be namespace-scoped")
	}
	return c.Client.List(ctx, list, opts...)
}

type fakeIndexedPolicyListValidatingTranslatorClient struct {
	client.Client
	expectedBackendTLSTargets map[string]struct{}
	expectedBackendLBTargets  map[string]struct{}
}

func (c fakeIndexedPolicyListValidatingTranslatorClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	switch typed := list.(type) {
	case *backendlb.BackendLBPolicyList:
		if err := requireMatchingAnyField(opts, backendLBPolicyTargetRefIndex, c.expectedBackendLBTargets); err != nil {
			return err
		}
	case *gatewayv1alpha3.BackendTLSPolicyList:
		if err := requireMatchingAnyField(opts, backendTLSPolicyTargetRefIndex, c.expectedBackendTLSTargets); err != nil {
			return err
		}
	case *unstructured.UnstructuredList:
		if typed.GroupVersionKind() == gwapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") {
			if err := requireMatchingAnyField(opts, backendTLSPolicyTargetRefIndex, c.expectedBackendTLSTargets); err != nil {
				return err
			}
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func requireMatchingAnyField(
	opts []client.ListOption,
	field string,
	allowed map[string]struct{},
) error {
	if len(allowed) == 0 {
		return nil
	}

	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	if listOptions.FieldSelector == nil || listOptions.FieldSelector.Empty() {
		return fmt.Errorf("list must include %s field selector", field)
	}
	for value := range allowed {
		if listOptions.FieldSelector.Matches(fields.Set{field: value}) {
			return nil
		}
	}
	return fmt.Errorf("field selector %q does not match any expected %s value", listOptions.FieldSelector.String(), field)
}

func testBackendLBPolicyTargetRefIndexKeys(policy *backendlb.BackendLBPolicy) []string {
	if policy == nil {
		return nil
	}
	out := make([]string, 0, len(policy.Spec.TargetRefs))
	seen := make(map[string]struct{}, len(policy.Spec.TargetRefs))
	for _, targetRef := range policy.Spec.TargetRefs {
		value := backendPolicyTargetRefIndexValue(
			string(targetRef.Group),
			string(targetRef.Kind),
			string(targetRef.Name),
		)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type fieldSelectorRejectingTranslatorClient struct {
	client.Client
}

func (c fieldSelectorRejectingTranslatorClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	typed, ok := list.(*unstructured.UnstructuredList)
	if !ok {
		return c.Client.List(ctx, list, opts...)
	}
	if typed.GroupVersionKind() != gwapi.BackendTLSPolicyV1GVK.GroupVersion().WithKind("BackendTLSPolicyList") {
		return c.Client.List(ctx, list, opts...)
	}

	listOptions := &client.ListOptions{}
	for _, opt := range opts {
		opt.ApplyToList(listOptions)
	}
	if listOptions.FieldSelector != nil && !listOptions.FieldSelector.Empty() {
		return fmt.Errorf("field label not supported: %s", backendTLSPolicyTargetRefIndex)
	}

	return c.Client.List(ctx, list, opts...)
}
