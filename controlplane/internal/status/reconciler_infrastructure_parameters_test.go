package status

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/controlplane/internal/managedresources"
)

func TestReconcileRejectsGatewayWithInvalidInfrastructureParametersRef(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
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
					"service.yaml": "type: ExternalName\n",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
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
			gatewayInfrastructureService("default", "gw"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}

	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalidParameters),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalid),
		1,
	)
	message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted))
	if !strings.Contains(message, "Gateway.spec.infrastructure.parametersRef default/broken-gateway-infra is invalid") {
		t.Fatalf("unexpected accepted message: %q", message)
	}
}

func TestReconcileRejectsGatewayWithUnsupportedInfrastructureParameterField(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
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
					"service.yaml": "type: LoadBalancer\nunsupportedField: true\n",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
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
			gatewayInfrastructureService("default", "gw"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}

	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalidParameters),
		1,
	)
	message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted))
	if !strings.Contains(message, "Gateway.spec.infrastructure.parametersRef default/broken-gateway-infra is invalid") {
		t.Fatalf("unexpected accepted message: %q", message)
	}
	if !strings.Contains(message, "unsupportedField") {
		t.Fatalf("expected unsupported field to be surfaced, got %q", message)
	}
}

func TestReconcileRejectsGatewayWhenGatewayClassParametersRefIsInvalid(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	classNamespace := gatewayv1.Namespace("infra-config")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: string(classNamespace)}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
					ParametersRef: &gatewayv1.ParametersReference{
						Group:     "",
						Kind:      "ConfigMap",
						Name:      "broken-class-infra",
						Namespace: &classNamespace,
					},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "broken-class-infra",
					Namespace: string(classNamespace),
				},
				Data: map[string]string{
					"service.yaml": "sessionAffinity: BrokenAffinity\n",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			gatewayInfrastructureService("default", "gw"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}

	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalidParameters),
		1,
	)
	message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted))
	if !strings.Contains(message, "GatewayClass.spec.parametersRef infra-config/broken-class-infra is invalid") {
		t.Fatalf("unexpected accepted message: %q", message)
	}
}

func TestReconcileAggregatesGatewayAndGatewayClassInfrastructureParameterErrors(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	classNamespace := gatewayv1.Namespace("infra-config")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: string(classNamespace)}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
					ParametersRef: &gatewayv1.ParametersReference{
						Group:     "",
						Kind:      "ConfigMap",
						Name:      "broken-class-infra",
						Namespace: &classNamespace,
					},
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "broken-class-infra",
					Namespace: string(classNamespace),
				},
				Data: map[string]string{
					"service.yaml": "sessionAffinity: BrokenAffinity\n",
				},
			},
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "broken-gateway-infra",
					Namespace: "default",
				},
				Data: map[string]string{
					"service.yaml": "type: ExternalName\n",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
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
			gatewayInfrastructureService("default", "gw"),
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}

	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalidParameters),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalid),
		1,
	)

	message := conditionMessage(t, gateway.Status.Conditions, string(gatewayv1.GatewayConditionAccepted))
	if !strings.Contains(message, "Gateway.spec.infrastructure.parametersRef default/broken-gateway-infra is invalid") {
		t.Fatalf("gateway parametersRef error missing from accepted message: %q", message)
	}
	if !strings.Contains(message, "GatewayClass.spec.parametersRef infra-config/broken-class-infra is invalid") {
		t.Fatalf("gatewayClass parametersRef error missing from accepted message: %q", message)
	}
}

func TestReconcileEmitsGatewayInfrastructureParameterEventsOnlyOnChange(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")
	gatewayObj := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
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
	}
	gatewayService := gatewayInfrastructureServiceForGateway(*gatewayObj)
	gatewayEndpointSlice := gatewayInfrastructureEndpointSliceForService(
		gatewayService,
		managedresources.EndpointSliceRoleGatewayFrontend,
	)

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(
			&gatewayv1.GatewayClass{},
			&gatewayv1.Gateway{},
		).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw", Generation: 1},
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
					"service.yaml": "type: ExternalName\n",
				},
			},
			gatewayObj,
			gatewayService,
			gatewayEndpointSlice,
		).
		Build()

	recorder := record.NewFakeRecorder(10)
	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	reconciler.SetEventRecorder(recorder)

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile returned error: %v", err)
	}

	warningEvent := nextRecordedEvent(t, recorder)
	if !strings.Contains(warningEvent, "Warning "+gatewayInfrastructureParametersInvalidEventReason) {
		t.Fatalf("unexpected warning event: %q", warningEvent)
	}
	if !strings.Contains(warningEvent, "Gateway.spec.infrastructure.parametersRef default/broken-gateway-infra is invalid") {
		t.Fatalf("unexpected warning event message: %q", warningEvent)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}
	assertNoRecordedEvent(t, recorder)

	var configMap corev1.ConfigMap
	if err := k8sClient.Get(
		context.Background(),
		client.ObjectKey{Namespace: "default", Name: "broken-gateway-infra"},
		&configMap,
	); err != nil {
		t.Fatalf("Get ConfigMap returned error: %v", err)
	}
	configMap.Data["service.yaml"] = "type: LoadBalancer\n"
	if err := k8sClient.Update(context.Background(), &configMap); err != nil {
		t.Fatalf("Update ConfigMap returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile returned error: %v", err)
	}

	resolvedEvent := nextRecordedEvent(t, recorder)
	if !strings.Contains(resolvedEvent, "Normal "+gatewayInfrastructureParametersResolvedEventReason) {
		t.Fatalf("unexpected resolved event: %q", resolvedEvent)
	}
	if !strings.Contains(resolvedEvent, "Gateway infrastructure parameters are resolved") {
		t.Fatalf("unexpected resolved event message: %q", resolvedEvent)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionAccepted),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonAccepted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonProgrammed),
		1,
	)
}

func nextRecordedEvent(t *testing.T, recorder *record.FakeRecorder) string {
	t.Helper()
	select {
	case event := <-recorder.Events:
		return event
	case <-time.After(time.Second):
		t.Fatal("expected Kubernetes event to be recorded")
		return ""
	}
}

func assertNoRecordedEvent(t *testing.T, recorder *record.FakeRecorder) {
	t.Helper()

	select {
	case event := <-recorder.Events:
		t.Fatalf("unexpected event: %q", event)
	case <-time.After(100 * time.Millisecond):
	}
}
