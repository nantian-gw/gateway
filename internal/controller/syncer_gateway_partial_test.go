package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backendlb "github.com/nantian-gw/gateway/internal/gatewayexp/backendlb"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator"
)

func TestReconcileGatewayListenerScopedRequestRefreshesOnlyListenersAndSecrets(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

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
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "example-cert", Namespace: "default"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readGatewayListenerTLSAsset(t, "client.crt"),
					"tls.key": readGatewayListenerTLSAsset(t, "client.key"),
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

	current := store.Current()
	if current == nil || len(current.Listeners) != 1 || len(current.Secrets) != 1 {
		t.Fatalf("expected initial listener and secret, got %#v", current)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected initial attached route, got %#v", current.Listeners[0].AttachedRoutes)
	}
	if current.Secrets[0].CertPEM != string(readGatewayListenerTLSAsset(t, "client.crt")) {
		t.Fatalf("unexpected initial secret material: %#v", current.Secrets)
	}

	var secret corev1.Secret
	if err := validatingClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "example-cert"},
		&secret,
	); err != nil {
		t.Fatalf("get secret: %v", err)
	}
	secret.Data["tls.crt"] = readGatewayListenerTLSAsset(t, "server-san.crt")
	secret.Data["tls.key"] = readGatewayListenerTLSAsset(t, "server-san.key")
	if err := validatingClient.Update(context.Background(), &secret); err != nil {
		t.Fatalf("update secret: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayList{}):                 "gateway-listener rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):               "gateway-listener rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):               "gateway-listener rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):          "gateway-listener rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):          "gateway-listener rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):          "gateway-listener rebuild should not list TLSRoutes",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}):     "gateway-listener rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.NamespaceList{}):                  "gateway-listener rebuild should not list Namespaces",
		reflect.TypeOf(&corev1.ServiceList{}):                    "gateway-listener rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):         "gateway-listener rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):         "gateway-listener rebuild should not list EndpointSlices",
		reflect.TypeOf(&corev1.SecretList{}):                     "gateway-listener rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):                  "gateway-listener rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                        "gateway-listener rebuild should not list Pods",
		reflect.TypeOf(&gatewayv1alpha3.BackendTLSPolicyList{}):  "gateway-listener rebuild should not list BackendTLSPolicies",
		reflect.TypeOf(&backendlb.BackendLBPolicyList{}): "gateway-listener rebuild should not list BackendLBPolicies",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}): requireGatewayClassControllerList(
			"gateway-listener rebuild should list GatewayClasses with controller-name selector",
			string(controllerName),
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "default",
			Name:      "gw",
		}),
	); err != nil {
		t.Fatalf("gateway-listener-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil || len(current.Listeners) != 1 || len(current.Secrets) != 1 {
		t.Fatalf("expected listener and secret after partial rebuild, got %#v", current)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected gateway-listener rebuild to preserve attached routes, got %#v", current.Listeners[0].AttachedRoutes)
	}
	if current.Secrets[0].CertPEM != string(readGatewayListenerTLSAsset(t, "server-san.crt")) ||
		current.Secrets[0].KeyPEM != string(readGatewayListenerTLSAsset(t, "server-san.key")) {
		t.Fatalf("expected gateway-listener rebuild to refresh secret material, got %#v", current.Secrets)
	}
}

func TestReconcileGatewayListenerScopedRequestDropsGatewayOutsideManagedClass(t *testing.T) {
	scheme := newPartialRebuildTestScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	mode := gatewayv1.TLSModeTerminate

	baseClient := newControllerClientBuilder(scheme).
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
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default"},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
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
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "example-cert", Namespace: "default"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": readGatewayListenerTLSAsset(t, "client.crt"),
					"tls.key": readGatewayListenerTLSAsset(t, "client.key"),
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

	current := store.Current()
	if current == nil || len(current.Listeners) != 1 || len(current.Secrets) != 1 {
		t.Fatalf("expected initial listener and secret, got %#v", current)
	}

	var gateway gatewayv1.Gateway
	if err := validatingClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "gw"},
		&gateway,
	); err != nil {
		t.Fatalf("get gateway: %v", err)
	}
	gateway.Spec.GatewayClassName = "other"
	if err := validatingClient.Update(context.Background(), &gateway); err != nil {
		t.Fatalf("update gateway: %v", err)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayList{}):                 "gateway-listener rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):               "gateway-listener rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):               "gateway-listener rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):          "gateway-listener rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):          "gateway-listener rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):          "gateway-listener rebuild should not list TLSRoutes",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}):     "gateway-listener rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.NamespaceList{}):                  "gateway-listener rebuild should not list Namespaces",
		reflect.TypeOf(&corev1.ServiceList{}):                    "gateway-listener rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):         "gateway-listener rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):         "gateway-listener rebuild should not list EndpointSlices",
		reflect.TypeOf(&corev1.SecretList{}):                     "gateway-listener rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):                  "gateway-listener rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                        "gateway-listener rebuild should not list Pods",
		reflect.TypeOf(&gatewayv1alpha3.BackendTLSPolicyList{}):  "gateway-listener rebuild should not list BackendTLSPolicies",
		reflect.TypeOf(&backendlb.BackendLBPolicyList{}): "gateway-listener rebuild should not list BackendLBPolicies",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}): requireGatewayClassControllerList(
			"gateway-listener rebuild should list GatewayClasses with controller-name selector",
			string(controllerName),
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "default",
			Name:      "gw",
		}),
	); err != nil {
		t.Fatalf("gateway-listener-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil {
		t.Fatal("expected current snapshot after scoped rebuild")
	}
	if len(current.Listeners) != 0 {
		t.Fatalf("expected scoped rebuild to drop unmanaged gateway listeners, got %#v", current.Listeners)
	}
	if len(current.Secrets) != 0 {
		t.Fatalf("expected scoped rebuild to drop unmanaged gateway secrets, got %#v", current.Secrets)
	}
}

func TestReconcileGatewayListenerScopedRequestPreservesRoutesAfterRouteScopedRecovery(t *testing.T) {
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
					Hostnames: []gatewayv1.Hostname{"example.com"},
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
	store.Publish(&ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
	})
	syncer := NewSyncer(
		validatingClient,
		translator.New(string(controllerName), logger),
		store,
		testMetrics(),
		0,
		logger,
	)
	syncer.SetSettleDelay(0)

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotScopedObjectReconcileRequest("snapshot-routes-http", types.NamespacedName{
			Namespace: "default",
			Name:      "route",
		}),
	); err != nil {
		t.Fatalf("initial route-scoped Reconcile returned error: %v", err)
	}

	current := store.Current()
	if current == nil || len(current.HTTPRoutes) != 1 {
		t.Fatalf("expected initial route-only snapshot, got %#v", current)
	}
	if len(current.Listeners) != 1 {
		t.Fatalf("expected route-scoped rebuild to recover missing listener, got %#v", current.Listeners)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected route-scoped rebuild to attach route during listener recovery, got %#v", current.Listeners[0].AttachedRoutes)
	}
	if got := current.Listeners[0].AttachedRoutes[0]; got != "default/route" {
		t.Fatalf("attached route = %q, want default/route", got)
	}

	validatingClient.forbiddenLists = map[reflect.Type]string{
		reflect.TypeOf(&gatewayv1.GatewayList{}):                 "gateway-listener rebuild should not list Gateways",
		reflect.TypeOf(&gatewayv1.HTTPRouteList{}):               "gateway-listener rebuild should not list HTTPRoutes",
		reflect.TypeOf(&gatewayv1.GRPCRouteList{}):               "gateway-listener rebuild should not list GRPCRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TCPRouteList{}):          "gateway-listener rebuild should not list TCPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.UDPRouteList{}):          "gateway-listener rebuild should not list UDPRoutes",
		reflect.TypeOf(&gatewayv1alpha2.TLSRouteList{}):          "gateway-listener rebuild should not list TLSRoutes",
		reflect.TypeOf(&gatewayv1beta1.ReferenceGrantList{}):     "gateway-listener rebuild should not list ReferenceGrants",
		reflect.TypeOf(&corev1.ServiceList{}):                    "gateway-listener rebuild should not list Services",
		reflect.TypeOf(&mcsv1alpha1.ServiceImportList{}):         "gateway-listener rebuild should not list ServiceImports",
		reflect.TypeOf(&discoveryv1.EndpointSliceList{}):         "gateway-listener rebuild should not list EndpointSlices",
		reflect.TypeOf(&corev1.SecretList{}):                     "gateway-listener rebuild should not list Secrets",
		reflect.TypeOf(&corev1.ConfigMapList{}):                  "gateway-listener rebuild should not list ConfigMaps",
		reflect.TypeOf(&corev1.PodList{}):                        "gateway-listener rebuild should not list Pods",
		reflect.TypeOf(&gatewayv1alpha3.BackendTLSPolicyList{}):  "gateway-listener rebuild should not list BackendTLSPolicies",
		reflect.TypeOf(&backendlb.BackendLBPolicyList{}): "gateway-listener rebuild should not list BackendLBPolicies",
	}
	validatingClient.listValidators = map[reflect.Type]func(client.ListOptions) error{
		reflect.TypeOf(&gatewayv1.GatewayClassList{}): requireGatewayClassControllerList(
			"gateway-listener rebuild should list GatewayClasses with controller-name selector",
			string(controllerName),
		),
	}

	if _, err := syncer.Reconcile(
		context.Background(),
		snapshotGatewayListenersReconcileRequestForKey(client.ObjectKey{
			Namespace: "default",
			Name:      "gw",
		}),
	); err != nil {
		t.Fatalf("gateway-listener-scoped Reconcile returned error: %v", err)
	}

	current = store.Current()
	if current == nil || len(current.Listeners) != 1 {
		t.Fatalf("expected listener after gateway listener rebuild, got %#v", current)
	}
	if len(current.Listeners[0].AttachedRoutes) != 1 {
		t.Fatalf("expected gateway-listener rebuild to preserve attached route, got %#v", current.Listeners[0].AttachedRoutes)
	}
	if got := current.Listeners[0].AttachedRoutes[0]; got != "default/route" {
		t.Fatalf("attached route = %q, want default/route", got)
	}
}

func readGatewayListenerTLSAsset(t *testing.T, name string) []byte {
	t.Helper()

	for _, dir := range []string{"tls", "backendtls"} {
		path := filepath.Join("..", "..", "test", "testdata", dir, name)
		raw, err := os.ReadFile(path)
		if err == nil {
			return raw
		}
	}
	t.Fatalf("read test tls asset %q: file not found in test/testdata/{tls,backendtls}", name)
	return nil
}

func requireGatewayClassControllerList(message, controllerName string) func(client.ListOptions) error {
	return func(opts client.ListOptions) error {
		if opts.FieldSelector == nil || opts.FieldSelector.Empty() {
			return errors.New(message)
		}
		if !opts.FieldSelector.Matches(fields.Set{
			"nantian.dev/infrastructure.gatewayclass.controller-name": controllerName,
		}) {
			return fmt.Errorf("unexpected GatewayClass field selector %q", opts.FieldSelector.String())
		}
		return nil
	}
}
