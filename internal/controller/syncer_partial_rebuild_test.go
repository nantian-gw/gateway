package controller

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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator"
)

func TestReconcileAttachmentScopedRequestRebuildsOnlyAttachments(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	namespaceMode := gatewayv1.NamespacesFromSelector

	baseClient := newControllerClientBuilder(scheme).
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
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
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
							Name:        "gw",
							Namespace:   ptr[gatewayv1.Namespace]("default"),
							SectionName: ptr[gatewayv1.SectionName]("http"),
						}},
					},
				},
			},
		).
		Build()

	validatingClient := &partialRebuildValidatingClient{Client: baseClient}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		validatingClient,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), snapshotReconcileRequest); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}
	if current := store.Current(); current == nil || len(current.Listeners) != 1 || len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected initial attached route, got %#v", current)
	}

	var namespace corev1.Namespace
	if err := validatingClient.Get(context.Background(), client.ObjectKey{Name: "apps"}, &namespace); err != nil {
		t.Fatalf("get namespace: %v", err)
	}
	namespace.Labels = map[string]string{"tenant": "other"}
	if err := validatingClient.Update(context.Background(), &namespace); err != nil {
		t.Fatalf("update namespace: %v", err)
	}
	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):       "attachment-only rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):       "attachment-only rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):  "attachment-only rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):  "attachment-only rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):  "attachment-only rebuild should not list TLSRoutes",
		reflect.TypeOf(&corev1.ServiceList{}):            "attachment-only rebuild should not list Services",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}): "attachment-only rebuild should not list EndpointSlices",
		reflect.TypeOf(&corev1.PodList{}):                "attachment-only rebuild should not list Pods",
	}

	if _, err := syncer.Reconcile(context.Background(), snapshotAttachmentsReconcileRequest("apps")); err != nil {
		t.Fatalf("attachment-scoped Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.Listeners) != 1 {
		t.Fatalf("expected listener after attachment rebuild, got %#v", current)
	}
	if len(current.Listeners[0].AttachedRoutes) != 0 {
		t.Fatalf("expected attachment-only rebuild to detach route, got %#v", current.Listeners[0].AttachedRoutes)
	}
}

func TestBuildSnapshotRebuildsGatewayListenersBeforeListenerSetAttachments(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	allNamespaces := gatewayv1.NamespacesFromAll

	baseClient := newControllerClientBuilder(scheme).
		WithObjects(
			&corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
					Labels: map[string]string{
						"kubernetes.io/metadata.name": "default",
					},
				},
			},
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
					AllowedListeners: &gatewayv1.AllowedListeners{
						Namespaces: &gatewayv1.ListenerNamespaces{
							From: &allNamespaces,
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "gateway-listener",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
		).
		Build()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	snapshotStore := ir.NewSnapshotStore(logger)
	xlator := translator.New(string(controllerName), logger)
	current, err := xlator.Build(context.Background(), baseClient)
	if err != nil {
		t.Fatalf("initial Build returned error: %v", err)
	}
	current.HTTPRoutes = []ir.HTTPRoute{{
		Name:      "ls-route",
		Namespace: "default",
		ParentRefs: []ir.ParentRef{{
			Group:     gatewayv1.GroupName,
			Kind:      "ListenerSet",
			Namespace: "default",
			Name:      "ls",
		}},
	}}
	if !snapshotStore.Publish(current) {
		t.Fatal("expected seed snapshot publish")
	}

	listenerHostname := gatewayv1.Hostname("listener-set.example.com")
	if err := baseClient.Create(context.Background(), &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default"},
		Spec: gatewayv1.ListenerSetSpec{
			ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gatewayv1.ListenerEntry{{
				Name:     "ls-listener",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
				Hostname: &listenerHostname,
				AllowedRoutes: &gatewayv1.AllowedRoutes{
					Namespaces: &gatewayv1.RouteNamespaces{
						From: &allNamespaces,
					},
				},
			}},
		},
	}); err != nil {
		t.Fatalf("create listenerset: %v", err)
	}

	syncer := NewSyncer(
		baseClient,
		xlator,
		snapshotStore,
		testMetrics(),
		0,
		logger,
	)

	next, err := syncer.buildSnapshot(
		context.Background(),
		snapshotBuildScopeGatewayListeners|snapshotBuildScopeAttachments,
		[]string{"default"},
		nil,
		[]client.ObjectKey{{Namespace: "default", Name: "gw"}},
		nil,
		nil,
		snapshotRouteObjectKeys{},
	)
	if err != nil {
		t.Fatalf("buildSnapshot returned error: %v", err)
	}

	for _, listener := range next.Listeners {
		if listener.Name != "default/gw/default/ls/ls-listener" {
			continue
		}
		if len(listener.AttachedRoutes) != 1 || listener.AttachedRoutes[0] != "default/ls-route" {
			t.Fatalf("expected ListenerSet listener attachments to be refreshed, got %#v", listener.AttachedRoutes)
		}
		return
	}
	t.Fatalf("expected ListenerSet listener in rebuilt snapshot, got %#v", next.Listeners)
}

func TestReconcileBackendScopedRequestRebuildsOnlyBackends(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	servicePort := gatewayv1.PortNumber(8080)

	baseClient := newControllerClientBuilder(scheme).
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
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&gatewayv1.HTTPRoute{
				ObjectMeta: metav1.ObjectMeta{Name: "route", Namespace: "default"},
				Spec: gatewayv1.HTTPRouteSpec{
					CommonRouteSpec: gatewayv1.CommonRouteSpec{
						ParentRefs: []gatewayv1.ParentReference{{
							Name:        "gw",
							SectionName: ptr[gatewayv1.SectionName]("http"),
						}},
					},
					Rules: []gatewayv1.HTTPRouteRule{{
						BackendRefs: []gatewayv1.HTTPBackendRef{{
							BackendRef: gatewayv1.BackendRef{
								BackendObjectReference: gatewayv1.BackendObjectReference{
									Name: "echo",
									Port: &servicePort,
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
		).
		Build()

	validatingClient := &partialRebuildValidatingClient{Client: baseClient}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		validatingClient,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), snapshotReconcileRequest); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}
	if current := store.Current(); current == nil || len(current.Backends) != 1 || current.Backends[0].Endpoints[0].Address != "10.0.0.10" {
		t.Fatalf("expected initial backend endpoint, got %#v", current)
	}

	var slice discoveryv1.EndpointSlice
	if err := validatingClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "echo-1"}, &slice); err != nil {
		t.Fatalf("get endpoint slice: %v", err)
	}
	slice.Endpoints = []discoveryv1.Endpoint{{
		Addresses: []string{"10.0.0.11"},
	}}
	if err := validatingClient.Update(context.Background(), &slice); err != nil {
		t.Fatalf("update endpoint slice: %v", err)
	}
	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "backend-only rebuild should not list GatewayClasses",
		reflect.TypeOf(&gatewayv1.GatewayList{}):             "backend-only rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "backend-only rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "backend-only rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "backend-only rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "backend-only rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "backend-only rebuild should not list TLSRoutes",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "backend-only rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.ServiceList{}):                "backend-only rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):     "backend-only rebuild should not list ServiceImports",
		reflect.TypeOf(&corev1.SecretList{}):                 "backend-only rebuild should not list Secrets",
		reflect.TypeOf(&corev1.PodList{}):                    "backend-only rebuild should not list Pods",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}): requireEndpointSliceList(
			"default",
			discoveryv1.LabelServiceName,
			"echo",
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotBackendsReconcileRequestForService(client.ObjectKey{
			Namespace: "default",
			Name:      "echo",
		}),
	); err != nil {
		t.Fatalf("backend-scoped Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.Backends) != 1 {
		t.Fatalf("expected backend after rebuild, got %#v", current)
	}
	if current.Backends[0].Endpoints[0].Address != "10.0.0.11" {
		t.Fatalf("expected backend-only rebuild to refresh endpoints, got %#v", current.Backends[0].Endpoints)
	}
	if len(current.Listeners) != 1 || len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected backend-only rebuild to preserve listeners and attachments, got %#v", current.Listeners)
	}
}
func TestReconcileBackendNamespaceScopedRequestRebuildsOnlyNamespaceBackends(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)

	baseClient := newControllerClientBuilder(scheme).
		WithObjects(
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
			&corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "spare", Namespace: "other"},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{{
						Name:       "http",
						Port:       9090,
						TargetPort: intstr.FromInt(9090),
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
			&discoveryv1.EndpointSlice{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "spare-1",
					Namespace: "other",
					Labels: map[string]string{
						discoveryv1.LabelServiceName: "spare",
					},
				},
				Ports: []discoveryv1.EndpointPort{{Port: ptr[int32](9090)}},
				Endpoints: []discoveryv1.Endpoint{{
					Addresses: []string{"10.0.0.90"},
				}},
			},
		).
		Build()

	validatingClient := &partialRebuildValidatingClient{Client: baseClient}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		validatingClient,
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(context.Background(), snapshotReconcileRequest); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}
	if current := store.Current(); current == nil || len(current.Backends) != 2 {
		t.Fatalf("expected initial backends, got %#v", current)
	}

	var echoSlice discoveryv1.EndpointSlice
	if err := validatingClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "echo-1"},
		&echoSlice,
	); err != nil {
		t.Fatalf("get echo endpoint slice: %v", err)
	}
	echoSlice.Endpoints = []discoveryv1.Endpoint{{
		Addresses: []string{"10.0.0.11"},
	}}
	if err := validatingClient.Update(context.Background(), &echoSlice); err != nil {
		t.Fatalf("update echo endpoint slice: %v", err)
	}
	if err := validatingClient.Delete(
		context.Background(),
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "spare", Namespace: "other"}},
	); err != nil {
		t.Fatalf("delete spare service: %v", err)
	}
	if err := validatingClient.Delete(
		context.Background(),
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "spare-1", Namespace: "other"}},
	); err != nil {
		t.Fatalf("delete spare endpoint slice: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}):        "namespace backend rebuild should not list GatewayClasses",
		reflect.TypeOf(&gatewayv1.GatewayList{}):             "namespace backend rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):           "namespace backend rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):           "namespace backend rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):      "namespace backend rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):      "namespace backend rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):      "namespace backend rebuild should not list TLSRoutes",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}): "namespace backend rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.SecretList{}):                 "namespace backend rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):              "namespace backend rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                    "namespace backend rebuild should not list Pods",
	}

	if _, err := syncer.Reconcile(context.Background(), snapshotBackendsReconcileRequestForNamespace("default")); err != nil {
		t.Fatalf("namespace backend Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.Backends) != 2 {
		t.Fatalf("expected namespace backend rebuild to preserve untouched namespaces, got %#v", current)
	}

	backendByName := make(map[string]ir.BackendCluster, len(current.Backends))
	for _, backend := range current.Backends {
		backendByName[backend.Namespace+"/"+backend.Name] = backend
	}
	if got := backendByName["default/echo:8080"].Endpoints[0].Address; got != "10.0.0.11" {
		t.Fatalf("echo backend endpoint address = %q, want %q", got, "10.0.0.11")
	}
	if got := backendByName["other/spare:9090"].Endpoints[0].Address; got != "10.0.0.90" {
		t.Fatalf("spare backend endpoint address = %q, want %q", got, "10.0.0.90")
	}
}

type partialRebuildValidatingClient struct {
	client.Client
	forbiddenLists map[reflect.Type]string
	listValidators map[reflect.Type]func(client.ListOptions) error
}

func (c *partialRebuildValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if message := c.forbiddenLists[reflect.TypeOf(list)]; message != "" {
		return fmt.Errorf("%s", message)
	}
	if validator := c.listValidators[reflect.TypeOf(list)]; validator != nil {
		var listOptions client.ListOptions
		for _, opt := range opts {
			opt.ApplyToList(&listOptions)
		}
		if err := validator(listOptions); err != nil {
			return fmt.Errorf("%w", err)
		}
	}
	return c.Client.List(ctx, list, opts...)
}

func requireEndpointSliceList(namespace, labelKey, serviceName string) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		if opts.Namespace != namespace {
			return fmt.Errorf("endpoint slice list namespace = %q, want %q", opts.Namespace, namespace)
		}
		if opts.LabelSelector == nil || opts.LabelSelector.Empty() {
			return fmt.Errorf("endpoint slice list must include a service label selector")
		}
		if !opts.LabelSelector.Matches(labels.Set{labelKey: serviceName}) {
			return fmt.Errorf("endpoint slice list selector = %q does not match %s=%q", opts.LabelSelector.String(), labelKey, serviceName)
		}
		if opts.LabelSelector.Matches(labels.Set{labelKey: serviceName + "-other"}) {
			return fmt.Errorf("endpoint slice list selector = %q is broader than %s=%q", opts.LabelSelector.String(), labelKey, serviceName)
		}
		return nil
	}
}

func requireNamespaceScopedList(namespace, resource string) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		if opts.Namespace != namespace {
			return fmt.Errorf("%s list namespace = %q, want %q", resource, opts.Namespace, namespace)
		}
		return nil
	}
}

func newPartialRebuildTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha3.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, backendlbv1alpha2.Install)
	mustAddToScheme(t, scheme, mcsv1alpha1.AddToScheme)
	return scheme
}
