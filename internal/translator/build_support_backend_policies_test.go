package translator

import (
	"context"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/nantian-gw/gateway/internal/gwapi"
	backendlb "github.com/nantian-gw/gateway/internal/gwexp/backendlb"
	"github.com/nantian-gw/gateway/internal/ir"
)

func TestBuildBackendsForSnapshotListsBackendPoliciesPerReferencedNamespace(t *testing.T) {
	scheme := buildSupportScheme(t)
	sessionType := gatewayv1.CookieBasedSessionPersistence

	baseClient := newTranslatorClientBuilder(scheme).
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
			&backendlb.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-lb", Namespace: "default"},
				Spec: backendlb.BackendLBPolicySpec{
					TargetRefs: []backendlb.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "echo",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{
						Type: &sessionType,
					},
				},
			},
			&backendlb.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "other-lb", Namespace: "other"},
				Spec: backendlb.BackendLBPolicySpec{
					TargetRefs: []backendlb.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "other",
					}},
					SessionPersistence: &gatewayv1.SessionPersistence{
						Type: &sessionType,
					},
				},
			},
		).
		Build()

	current := &ir.Snapshot{
		Backends: []ir.BackendCluster{{
			Name:      "echo:8080",
			Namespace: "default",
			Metadata: map[string]string{
				"service": "echo",
			},
		}},
	}

	backends, err := New(
		"gateway.networking.k8s.io/nantian-gw",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).BuildBackendsForSnapshot(context.Background(), fakeScopedPolicyListValidatingTranslatorClient{
		Client: baseClient,
	}, current, nil, nil)
	if err != nil {
		t.Fatalf("BuildBackendsForSnapshot returned error: %v", err)
	}

	if len(backends) != 1 {
		t.Fatalf("expected 1 backend, got %#v", backends)
	}
	if backends[0].SessionPersistence == nil {
		t.Fatalf("expected backend LB policy to be applied, got %#v", backends[0])
	}
}
func TestBuildBackendsForSnapshotPreservesUntouchedBackends(t *testing.T) {
	scheme := buildSupportScheme(t)

	baseClient := newTranslatorClientBuilder(scheme).
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
				ObjectMeta: metav1.ObjectMeta{Name: "spare", Namespace: "default"},
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
					Namespace: "default",
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

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	translator := New("gateway.networking.k8s.io/nantian-gw", logger)
	current, err := translator.Build(context.Background(), baseClient)
	if err != nil {
		t.Fatalf("initial Build returned error: %v", err)
	}
	if len(current.Backends) != 2 {
		t.Fatalf("expected 2 initial backends, got %#v", current.Backends)
	}

	var echoSlice discoveryv1.EndpointSlice
	if err := baseClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "echo-1"},
		&echoSlice,
	); err != nil {
		t.Fatalf("get echo endpoint slice: %v", err)
	}
	echoSlice.Endpoints = []discoveryv1.Endpoint{{
		Addresses: []string{"10.0.0.11"},
	}}
	if err := baseClient.Update(context.Background(), &echoSlice); err != nil {
		t.Fatalf("update echo endpoint slice: %v", err)
	}
	if err := baseClient.Delete(
		context.Background(),
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "spare", Namespace: "default"}},
	); err != nil {
		t.Fatalf("delete spare service: %v", err)
	}
	if err := baseClient.Delete(
		context.Background(),
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "spare-1", Namespace: "default"}},
	); err != nil {
		t.Fatalf("delete spare endpoint slice: %v", err)
	}

	backends, err := translator.BuildBackendsForSnapshot(
		context.Background(),
		baseClient,
		current,
		[]client.ObjectKey{{Namespace: "default", Name: "echo"}},
		nil,
	)
	if err != nil {
		t.Fatalf("BuildBackendsForSnapshot returned error: %v", err)
	}

	if len(backends) != 2 {
		t.Fatalf("expected untouched backend to be preserved, got %#v", backends)
	}

	backendByName := make(map[string]ir.BackendCluster, len(backends))
	for _, backend := range backends {
		backendByName[backend.Name] = backend
	}
	if got := backendByName["echo:8080"].Endpoints[0].Address; got != "10.0.0.11" {
		t.Fatalf("echo backend endpoint address = %q, want %q", got, "10.0.0.11")
	}
	if _, ok := backendByName["spare:9090"]; !ok {
		t.Fatalf("expected spare backend to be preserved, got %#v", backends)
	}
	if got := backendByName["spare:9090"].Endpoints[0].Address; got != "10.0.0.90" {
		t.Fatalf("spare backend endpoint address = %q, want %q", got, "10.0.0.90")
	}
}
func TestLoadBackendTLSPoliciesForNamespacesScopesAndFilters(t *testing.T) {
	scheme := buildSupportScheme(t)
	caBundle := gatewayv1.WellKnownCACertificatesSystem

	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "echo",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "echo.default.svc.cluster.local",
						WellKnownCACertificates: &caBundle,
					},
				},
			},
			&gatewayv1alpha3.BackendTLSPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "other-tls", Namespace: "other"},
				Spec: gatewayv1.BackendTLSPolicySpec{
					TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
						LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
							Group: "",
							Kind:  "Service",
							Name:  "other",
						},
					}},
					Validation: gatewayv1.BackendTLSPolicyValidation{
						Hostname:                "other.default.svc.cluster.local",
						WellKnownCACertificates: &caBundle,
					},
				},
			},
		).
		Build()

	policies, err := loadBackendTLSPoliciesForNamespaces(
		context.Background(),
		fakeScopedPolicyListValidatingTranslatorClient{Client: baseClient},
		[]string{"default"},
		map[string]client.ObjectKey{
			"default/echo": {Namespace: "default", Name: "echo"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("loadBackendTLSPoliciesForNamespaces returned error: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 scoped BackendTLSPolicy, got %#v", policies)
	}
	if policies[0].Namespace != "default" || policies[0].Name != "echo-tls" {
		t.Fatalf("unexpected scoped BackendTLSPolicy: %#v", policies[0])
	}
}
func TestLoadBackendTLSPoliciesForNamespacesUsesTargetRefFieldIndexes(t *testing.T) {
	scheme := buildSupportScheme(t)
	caBundle := gatewayv1.WellKnownCACertificatesSystem
	echoPolicy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "echo",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "echo.default.svc.cluster.local",
				WellKnownCACertificates: &caBundle,
			},
		},
	}
	sparePolicy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "spare-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "spare",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "spare.default.svc.cluster.local",
				WellKnownCACertificates: &caBundle,
			},
		},
	}
	echoRaw, err := gwapi.EncodeBackendTLSPolicyV1(echoPolicy)
	if err != nil {
		t.Fatalf("encode echo BackendTLSPolicy: %v", err)
	}
	spareRaw, err := gwapi.EncodeBackendTLSPolicyV1(sparePolicy)
	if err != nil {
		t.Fatalf("encode spare BackendTLSPolicy: %v", err)
	}

	baseClient := newTranslatorClientBuilder(scheme).
		WithIndex(gwapi.NewBackendTLSPolicyV1Object(), backendTLSPolicyTargetRefIndex, func(object client.Object) []string {
			return backendTLSPolicyTargetRefIndexKeys(object)
		}).
		WithObjects(
			echoRaw,
			spareRaw,
		).
		Build()

	policies, err := loadBackendTLSPoliciesForNamespaces(
		context.Background(),
		fakeIndexedPolicyListValidatingTranslatorClient{
			Client: baseClient,
			expectedBackendTLSTargets: map[string]struct{}{
				backendPolicyTargetRefIndexValue("", "Service", "echo"): {},
			},
		},
		[]string{"default"},
		map[string]client.ObjectKey{
			"default/echo": {Namespace: "default", Name: "echo"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("loadBackendTLSPoliciesForNamespaces returned error: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 indexed BackendTLSPolicy, got %#v", policies)
	}
	if policies[0].Namespace != "default" || policies[0].Name != "echo-tls" {
		t.Fatalf("unexpected indexed BackendTLSPolicy: %#v", policies[0])
	}
}
func TestLoadBackendTLSPoliciesForNamespacesFallsBackWhenFieldSelectorUnsupported(t *testing.T) {
	scheme := buildSupportScheme(t)

	caBundle := gatewayv1.WellKnownCACertificatesSystem
	echoPolicy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "echo",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "echo.default.svc.cluster.local",
				WellKnownCACertificates: &caBundle,
			},
		},
	}
	sparePolicy := &gatewayv1alpha3.BackendTLSPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "spare-tls", Namespace: "default"},
		Spec: gatewayv1.BackendTLSPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReferenceWithSectionName{{
				LocalPolicyTargetReference: gatewayv1.LocalPolicyTargetReference{
					Group: "",
					Kind:  "Service",
					Name:  "spare",
				},
			}},
			Validation: gatewayv1.BackendTLSPolicyValidation{
				Hostname:                "spare.default.svc.cluster.local",
				WellKnownCACertificates: &caBundle,
			},
		},
	}
	echoRaw, err := gwapi.EncodeBackendTLSPolicyV1(echoPolicy)
	if err != nil {
		t.Fatalf("encode echo BackendTLSPolicy: %v", err)
	}
	spareRaw, err := gwapi.EncodeBackendTLSPolicyV1(sparePolicy)
	if err != nil {
		t.Fatalf("encode spare BackendTLSPolicy: %v", err)
	}

	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(
			echoRaw,
			spareRaw,
		).
		Build()

	policies, err := loadBackendTLSPoliciesForNamespaces(
		context.Background(),
		fieldSelectorRejectingTranslatorClient{Client: baseClient},
		[]string{"default"},
		map[string]client.ObjectKey{
			"default/echo": {Namespace: "default", Name: "echo"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("loadBackendTLSPoliciesForNamespaces returned error: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 BackendTLSPolicy after fallback, got %#v", policies)
	}
	if policies[0].Namespace != "default" || policies[0].Name != "echo-tls" {
		t.Fatalf("unexpected BackendTLSPolicy after fallback: %#v", policies[0])
	}
}
func TestLoadBackendLBPoliciesForNamespacesUsesTargetRefFieldIndexes(t *testing.T) {
	scheme := buildSupportScheme(t)

	baseClient := newTranslatorClientBuilder(scheme).
		WithIndex(&backendlb.BackendLBPolicy{}, backendLBPolicyTargetRefIndex, func(object client.Object) []string {
			policy, ok := object.(*backendlb.BackendLBPolicy)
			if !ok {
				return nil
			}
			return testBackendLBPolicyTargetRefIndexKeys(policy)
		}).
		WithObjects(
			&backendlb.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "echo-lb", Namespace: "default"},
				Spec: backendlb.BackendLBPolicySpec{
					TargetRefs: []backendlb.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "echo",
					}},
				},
			},
			&backendlb.BackendLBPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "spare-lb", Namespace: "default"},
				Spec: backendlb.BackendLBPolicySpec{
					TargetRefs: []backendlb.LocalPolicyTargetReference{{
						Group: "",
						Kind:  "Service",
						Name:  "spare",
					}},
				},
			},
		).
		Build()

	policies, err := loadBackendLBPoliciesForNamespaces(
		context.Background(),
		fakeIndexedPolicyListValidatingTranslatorClient{
			Client: baseClient,
			expectedBackendLBTargets: map[string]struct{}{
				backendPolicyTargetRefIndexValue("", "Service", "echo"): {},
			},
		},
		[]string{"default"},
		map[string]client.ObjectKey{
			"default/echo": {Namespace: "default", Name: "echo"},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("loadBackendLBPoliciesForNamespaces returned error: %v", err)
	}
	if len(policies) != 1 {
		t.Fatalf("expected 1 indexed BackendLBPolicy, got %#v", policies)
	}
	if policies[0].Namespace != "default" || policies[0].Name != "echo-lb" {
		t.Fatalf("unexpected indexed BackendLBPolicy: %#v", policies[0])
	}
}
