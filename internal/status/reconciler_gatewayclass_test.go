package status

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayfeatures "sigs.k8s.io/gateway-api/pkg/features"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
)

func TestReconcileGatewayClassObjectSetsAcceptedConditionDetails(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 7},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.ReconcileGatewayClassObject(context.Background(), "nantian-gw"); err != nil {
		t.Fatalf("ReconcileGatewayClassObject returned error: %v", err)
	}

	var gatewayClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "nantian-gw"}, &gatewayClass); err != nil {
		t.Fatalf("Get GatewayClass returned error: %v", err)
	}
	assertCondition(t, gatewayClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayClassReasonAccepted), 7)

	for _, condition := range gatewayClass.Status.Conditions {
		if condition.Type != string(gatewayv1.GatewayClassConditionStatusAccepted) {
			continue
		}
		if condition.Message != "GatewayClass is accepted by nantian-gw" {
			t.Fatalf("accepted message = %q", condition.Message)
		}
		return
	}

	t.Fatalf("accepted condition not found in %#v", gatewayClass.Status.Conditions)
}

func TestReconcileGatewayClassObjectPublishesSupportedVersionAndFeatures(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 3},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gatewayAPICRD("gatewayclasses.gateway.networking.k8s.io", "v1.5.1"),
			gatewayAPICRD("backendlbpolicies.gateway.networking.k8s.io", "v1.5.1"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.ReconcileGatewayClassObject(context.Background(), "nantian-gw"); err != nil {
		t.Fatalf("ReconcileGatewayClassObject returned error: %v", err)
	}

	var gatewayClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "nantian-gw"}, &gatewayClass); err != nil {
		t.Fatalf("Get GatewayClass returned error: %v", err)
	}

	assertCondition(t, gatewayClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayClassReasonAccepted), 3)
	assertCondition(t, gatewayClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusSupportedVersion), metav1.ConditionTrue, string(gatewayv1.GatewayClassReasonSupportedVersion), 3)
	assertConditionMessage(
		t,
		gatewayClass.Status.Conditions,
		string(gatewayv1.GatewayClassConditionStatusSupportedVersion),
		"Gateway API CRD bundle versions are supported: v1.5.1",
	)
	wantFeatures := gatewayapi.SupportedFeaturesForOptions(gatewayapi.FeatureOptions{EnableExperimentalGateway: true})
	if !reflect.DeepEqual(gatewayClass.Status.SupportedFeatures, wantFeatures) {
		t.Fatalf("supported features = %#v, want %#v", gatewayClass.Status.SupportedFeatures, wantFeatures)
	}
}

func TestReconcileGatewayClassObjectFiltersExperimentalGatewayFeaturesWhenDisabled(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 3},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gatewayAPICRD("gatewayclasses.gateway.networking.k8s.io", "v1.5.1"),
		).
		Build()

	reconciler := NewWithAddressesAndReaderOptions(
		k8sClient,
		k8sClient,
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
		Options{EnableExperimentalGateway: false},
	)
	if err := reconciler.ReconcileGatewayClassObject(context.Background(), "nantian-gw"); err != nil {
		t.Fatalf("ReconcileGatewayClassObject returned error: %v", err)
	}

	var gatewayClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "nantian-gw"}, &gatewayClass); err != nil {
		t.Fatalf("Get GatewayClass returned error: %v", err)
	}

	want := gatewayapi.SupportedFeaturesForOptions(gatewayapi.FeatureOptions{EnableExperimentalGateway: false})
	if !reflect.DeepEqual(gatewayClass.Status.SupportedFeatures, want) {
		t.Fatalf("supported features with experimental Gateway disabled = %#v, want %#v", gatewayClass.Status.SupportedFeatures, want)
	}

	names := supportedFeatureStatusNameSet(gatewayClass.Status.SupportedFeatures)
	for _, name := range []gatewayv1.FeatureName{
		gatewayv1.FeatureName(gatewayapi.SupportedTCPRoute),
		gatewayv1.FeatureName(gatewayfeatures.SupportListenerSet),
		gatewayv1.FeatureName(gatewayfeatures.SupportUDPRoute),
		gatewayv1.FeatureName(gatewayfeatures.SupportTLSRoute),
		gatewayv1.FeatureName(gatewayfeatures.SupportTLSRouteModeTerminate),
		gatewayv1.FeatureName(gatewayfeatures.SupportTLSRouteModeMixed),
	} {
		if names[name] {
			t.Fatalf("feature %s should not be advertised when experimental Gateway support is disabled: %#v", name, gatewayClass.Status.SupportedFeatures)
		}
	}
}

func TestReconcileGatewayClassObjectRejectsMissingBundleVersionAnnotations(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 4},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gatewayAPICRD("gatewayclasses.gateway.networking.k8s.io", ""),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.ReconcileGatewayClassObject(context.Background(), "nantian-gw"); err != nil {
		t.Fatalf("ReconcileGatewayClassObject returned error: %v", err)
	}

	var gatewayClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "nantian-gw"}, &gatewayClass); err != nil {
		t.Fatalf("Get GatewayClass returned error: %v", err)
	}

	assertCondition(t, gatewayClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayClassReasonAccepted), 4)
	assertCondition(t, gatewayClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusSupportedVersion), metav1.ConditionFalse, string(gatewayv1.GatewayClassReasonUnsupportedVersion), 4)
	assertConditionMessage(
		t,
		gatewayClass.Status.Conditions,
		string(gatewayv1.GatewayClassConditionStatusSupportedVersion),
		"Gateway API CRDs are missing the gateway.networking.k8s.io/bundle-version annotation: gatewayclasses.gateway.networking.k8s.io",
	)
}

func TestReconcileGatewayClassObjectRejectsOlderGatewayAPIBundleVersions(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 5},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			gatewayAPICRD("gatewayclasses.gateway.networking.k8s.io", "v1.2.1"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.ReconcileGatewayClassObject(context.Background(), "nantian-gw"); err != nil {
		t.Fatalf("ReconcileGatewayClassObject returned error: %v", err)
	}

	var gatewayClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "nantian-gw"}, &gatewayClass); err != nil {
		t.Fatalf("Get GatewayClass returned error: %v", err)
	}

	assertCondition(
		t,
		gatewayClass.Status.Conditions,
		string(gatewayv1.GatewayClassConditionStatusSupportedVersion),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayClassReasonUnsupportedVersion),
		5,
	)
	assertConditionMessage(
		t,
		gatewayClass.Status.Conditions,
		string(gatewayv1.GatewayClassConditionStatusSupportedVersion),
		"Unsupported Gateway API CRD bundle versions detected: gatewayclasses.gateway.networking.k8s.io=v1.2.1 (supported range: v1.5.1 - v1.5.1)",
	)
}

func TestReconcileGatewayClassObjectSkipsUnmanagedGatewayClass(t *testing.T) {
	scheme := newScheme(t)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, "gateway.networking.k8s.io/nantian-gw", "127.0.0.1", discardLogger())
	if err := reconciler.ReconcileGatewayClassObject(context.Background(), "foreign"); err != nil {
		t.Fatalf("ReconcileGatewayClassObject returned error: %v", err)
	}

	var gatewayClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "foreign"}, &gatewayClass); err != nil {
		t.Fatalf("Get GatewayClass returned error: %v", err)
	}
	if len(gatewayClass.Status.Conditions) != 0 {
		t.Fatalf("expected unmanaged GatewayClass status to stay empty, got %#v", gatewayClass.Status.Conditions)
	}
}

func TestReconcileSkipsUnmanagedGatewayClassDuringFullReconcile(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 2},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "foreign", Generation: 4},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var managed gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "nantian-gw"}, &managed); err != nil {
		t.Fatalf("Get managed GatewayClass returned error: %v", err)
	}
	assertCondition(t, managed.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayClassReasonAccepted), 2)

	var foreign gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "foreign"}, &foreign); err != nil {
		t.Fatalf("Get foreign GatewayClass returned error: %v", err)
	}
	if len(foreign.Status.Conditions) != 0 {
		t.Fatalf("expected unmanaged GatewayClass status to stay empty, got %#v", foreign.Status.Conditions)
	}
}

func TestReconcileManagesMultipleGatewayClassesForSameController(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Generation: 3},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "internal", Generation: 5},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-edge", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "edge",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			gatewayInfrastructureService("default", "gw-edge"),
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw-internal", Namespace: "default", Generation: 2},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "internal",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
				},
			},
			gatewayInfrastructureService("default", "gw-internal"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var edgeClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "edge"}, &edgeClass); err != nil {
		t.Fatalf("Get edge GatewayClass returned error: %v", err)
	}
	assertCondition(t, edgeClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayClassReasonAccepted), 3)

	var internalClass gatewayv1.GatewayClass
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Name: "internal"}, &internalClass); err != nil {
		t.Fatalf("Get internal GatewayClass returned error: %v", err)
	}
	assertCondition(t, internalClass.Status.Conditions, string(gatewayv1.GatewayClassConditionStatusAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayClassReasonAccepted), 5)

	var edgeGateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw-edge"}, &edgeGateway); err != nil {
		t.Fatalf("Get edge Gateway returned error: %v", err)
	}
	assertCondition(t, edgeGateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 1)

	var internalGateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw-internal"}, &internalGateway); err != nil {
		t.Fatalf("Get internal Gateway returned error: %v", err)
	}
	assertCondition(t, internalGateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted), metav1.ConditionTrue, string(gatewayv1.GatewayReasonAccepted), 2)
}

func TestReconcileLoadsGatewayAPICRDsOncePerFullReconcile(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}).
		WithObjects(
			gatewayAPICRD("gatewayclasses.gateway.networking.k8s.io", "v1.5.1"),
			gatewayAPICRD("gateways.gateway.networking.k8s.io", "v1.5.1"),
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "edge", Generation: 3},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "internal", Generation: 5},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
		).
		Build()

	crdLists := 0
	reconciler := NewWithAddressesAndReader(
		k8sClient,
		validatingListReader{
			Reader: k8sClient,
			listValidators: map[reflect.Type]func(client.ListOptions) error{
				reflect.TypeOf(&apiextensionsv1.CustomResourceDefinitionList{}): func(client.ListOptions) error { //nolint:unparam // signature constrained by listValidators map type
					crdLists++
					return nil
				},
			},
		},
		string(controllerName),
		[]string{"127.0.0.1"},
		discardLogger(),
	)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}
	if crdLists != 1 {
		t.Fatalf("gateway API CRD list count = %d, want 1", crdLists)
	}
}

func supportedFeatureStatusNameSet(items []gatewayv1.SupportedFeature) map[gatewayv1.FeatureName]bool {
	out := make(map[gatewayv1.FeatureName]bool, len(items))
	for _, item := range items {
		out[item.Name] = true
	}
	return out
}
