package status

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestGatewayConvergenceObservationTracksServiceMetadataLag(t *testing.T) {
	gateway := gatewayWithGenerationForConvergenceTest()
	service := gatewayInfrastructureServiceForGateway(*gateway)
	service.Annotations[gatewayConvergenceOwnerGenerationAnnotation] = "2"

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways: []gatewayv1.Gateway{*gateway},
		services: []corev1.Service{*service},
	}
	state.index()

	observation := gatewayConvergenceObservationForCurrentState(state, *gateway)
	if observation.serviceMetadataGenerationLag != 1 {
		t.Fatalf("serviceMetadataGenerationLag = %d, want 1", observation.serviceMetadataGenerationLag)
	}
	if observation.frontendEndpointSliceGenerationLag != 0 {
		t.Fatalf("frontendEndpointSliceGenerationLag = %d, want 0", observation.frontendEndpointSliceGenerationLag)
	}
	if observation.programmedObservedGenerationLag != 0 {
		t.Fatalf("programmedObservedGenerationLag = %d, want 0", observation.programmedObservedGenerationLag)
	}
}

func TestGatewayConvergenceObservationTracksFrontendEndpointSliceLag(t *testing.T) {
	gateway := gatewayWithGenerationForConvergenceTest()
	service := gatewayInfrastructureServiceForGateway(*gateway)
	endpointSlice := gatewayInfrastructureEndpointSlice("default", "gw", "shared-frontend-endpoints")
	endpointSlice.Annotations[gatewayConvergenceOwnerGenerationAnnotation] = "2"

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways:       []gatewayv1.Gateway{*gateway},
		services:       []corev1.Service{*service},
		endpointSlices: []discoveryv1.EndpointSlice{*endpointSlice},
	}
	state.index()

	observation := gatewayConvergenceObservationForCurrentState(state, *gateway)
	if observation.serviceMetadataGenerationLag != 0 {
		t.Fatalf("serviceMetadataGenerationLag = %d, want 0", observation.serviceMetadataGenerationLag)
	}
	if observation.frontendEndpointSliceGenerationLag != 1 {
		t.Fatalf("frontendEndpointSliceGenerationLag = %d, want 1", observation.frontendEndpointSliceGenerationLag)
	}
	if observation.programmedObservedGenerationLag != 0 {
		t.Fatalf("programmedObservedGenerationLag = %d, want 0", observation.programmedObservedGenerationLag)
	}
}

func TestGatewayConvergenceObservationTracksMissingFrontendEndpointSliceLag(t *testing.T) {
	gateway := gatewayWithGenerationForConvergenceTest()
	service := gatewayInfrastructureServiceForGateway(*gateway)

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways: []gatewayv1.Gateway{*gateway},
		services: []corev1.Service{*service},
	}
	state.index()

	observation := gatewayConvergenceObservationForCurrentState(state, *gateway)
	if observation.serviceMetadataGenerationLag != 0 {
		t.Fatalf("serviceMetadataGenerationLag = %d, want 0", observation.serviceMetadataGenerationLag)
	}
	if observation.frontendEndpointSliceGenerationLag != 3 {
		t.Fatalf("frontendEndpointSliceGenerationLag = %d, want 3", observation.frontendEndpointSliceGenerationLag)
	}
	if observation.programmedObservedGenerationLag != 0 {
		t.Fatalf("programmedObservedGenerationLag = %d, want 0", observation.programmedObservedGenerationLag)
	}
}

func TestGatewayRequiresInfrastructureRefreshIgnoresMissingFrontendEndpointSliceLag(t *testing.T) {
	eval := gatewayEvaluation{
		convergence: gatewayConvergenceObservation{
			frontendEndpointSliceGenerationLag: 3,
		},
	}

	if gatewayRequiresInfrastructureRefresh(eval) {
		t.Fatal("missing frontend EndpointSlice lag should not force object-reader infrastructure refresh")
	}
}

func TestGatewayConvergenceObservationTracksProgrammedObservedGenerationLag(t *testing.T) {
	gateway := gatewayWithGenerationForConvergenceTest()
	setCondition(&gateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonProgrammed),
		Message:            "Gateway is programmed",
		ObservedGeneration: 2,
	})

	service := gatewayInfrastructureServiceForGateway(*gateway)
	endpointSlice := gatewayInfrastructureEndpointSlice("default", "gw", "gateway-frontend-endpoints")
	endpointSlice.Annotations = map[string]string{}
	for key, value := range service.Annotations {
		endpointSlice.Annotations[key] = value
	}

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways:       []gatewayv1.Gateway{*gateway},
		services:       []corev1.Service{*service},
		endpointSlices: []discoveryv1.EndpointSlice{*endpointSlice},
	}
	state.index()

	observation := gatewayConvergenceObservationForCurrentState(state, *gateway)
	if observation.programmedObservedGenerationLag != 1 {
		t.Fatalf("programmedObservedGenerationLag = %d, want 1", observation.programmedObservedGenerationLag)
	}
	if observation.programmedPendingReason != "" {
		t.Fatalf("programmedPendingReason = %q, want empty", observation.programmedPendingReason)
	}
}

func TestGatewayConvergenceObservationTracksProgrammedPendingReason(t *testing.T) {
	gateway := gatewayWithGenerationForConvergenceTest()
	setCondition(&gateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionFalse,
		Reason:             string(gatewayv1.GatewayReasonPending),
		Message:            "Waiting for derived Gateway frontend EndpointSlices to converge",
		ObservedGeneration: 3,
	})

	service := gatewayInfrastructureServiceForGateway(*gateway)
	endpointSlice := gatewayInfrastructureEndpointSlice("default", "gw", "gateway-frontend-endpoints")
	endpointSlice.Annotations = map[string]string{}
	for key, value := range service.Annotations {
		endpointSlice.Annotations[key] = value
	}

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways:       []gatewayv1.Gateway{*gateway},
		services:       []corev1.Service{*service},
		endpointSlices: []discoveryv1.EndpointSlice{*endpointSlice},
	}
	state.index()

	observation := gatewayConvergenceObservationForCurrentState(state, *gateway)
	if observation.programmedObservedGenerationLag != 0 {
		t.Fatalf("programmedObservedGenerationLag = %d, want 0", observation.programmedObservedGenerationLag)
	}
	if observation.programmedPendingReason != string(gatewayv1.GatewayReasonPending) {
		t.Fatalf("programmedPendingReason = %q, want %q", observation.programmedPendingReason, gatewayv1.GatewayReasonPending)
	}
}

func TestGatewayConvergenceObservationNormalizesUnknownProgrammedPendingReason(t *testing.T) {
	gateway := gatewayWithGenerationForConvergenceTest()
	setCondition(&gateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionFalse,
		Reason:             "tenant-specific-reason-123",
		Message:            "Waiting for an external status writer",
		ObservedGeneration: 3,
	})

	service := gatewayInfrastructureServiceForGateway(*gateway)
	endpointSlice := gatewayInfrastructureEndpointSlice("default", "gw", "gateway-frontend-endpoints")
	endpointSlice.Annotations = cloneStringMap(service.Annotations)

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways:       []gatewayv1.Gateway{*gateway},
		services:       []corev1.Service{*service},
		endpointSlices: []discoveryv1.EndpointSlice{*endpointSlice},
	}
	state.index()

	observation := gatewayConvergenceObservationForCurrentState(state, *gateway)
	if observation.programmedPendingReason != gatewayProgrammedPendingReasonOther {
		t.Fatalf("programmedPendingReason = %q, want %q", observation.programmedPendingReason, gatewayProgrammedPendingReasonOther)
	}
}

func TestSyncGatewayConvergenceStageMetricsTracksCurrentStageTotals(t *testing.T) {
	resetGatewayConvergenceMetricsForTest()

	invalidGateway := gatewayWithNameAndGenerationForConvergenceTest("gw-invalid", 1)
	invalidGateway.Spec.Listeners[0].Protocol = gatewayv1.ProtocolType("SMTP")

	translatedGateway := gatewayWithNameAndGenerationForConvergenceTest("gw-translated", 3)

	infraGateway := gatewayWithNameAndGenerationForConvergenceTest("gw-infra", 4)
	infraService := gatewayInfrastructureServiceForGateway(*infraGateway)
	infraSlice := gatewayInfrastructureEndpointSlice(
		infraGateway.Namespace,
		infraGateway.Name,
		"gateway-frontend-endpoints",
	)
	infraSlice.Annotations = cloneStringMap(infraService.Annotations)

	readyGateway := gatewayWithNameAndGenerationForConvergenceTest("gw-ready", 5)
	setCondition(&readyGateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonProgrammed),
		Message:            "Gateway is programmed",
		ObservedGeneration: readyGateway.Generation,
	})
	readyService := gatewayInfrastructureServiceForGateway(*readyGateway)
	readySlice := gatewayInfrastructureEndpointSlice(
		readyGateway.Namespace,
		readyGateway.Name,
		"gateway-frontend-endpoints",
	)
	readySlice.Annotations = cloneStringMap(readyService.Annotations)

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways: []gatewayv1.Gateway{
			*invalidGateway,
			*translatedGateway,
			*infraGateway,
			*readyGateway,
		},
		services: []corev1.Service{
			*infraService,
			*readyService,
		},
		endpointSlices: []discoveryv1.EndpointSlice{
			*infraSlice,
			*readySlice,
		},
	}
	state.index()

	syncGatewayConvergenceStageMetrics(evaluateGateways(state, nil))

	if got := testutil.ToFloat64(
		gatewayConvergenceStageTotal.WithLabelValues(
			string(gatewayConvergenceStageManaged),
		),
	); got != 4 {
		t.Fatalf("stage_total{stage=%q} = %v, want 4", gatewayConvergenceStageManaged, got)
	}
	if got := testutil.ToFloat64(
		gatewayConvergenceStageTotal.WithLabelValues(
			string(gatewayConvergenceStageTranslated),
		),
	); got != 3 {
		t.Fatalf("stage_total{stage=%q} = %v, want 3", gatewayConvergenceStageTranslated, got)
	}
	if got := testutil.ToFloat64(
		gatewayConvergenceStageTotal.WithLabelValues(
			string(gatewayConvergenceStageInfrastructureConverged),
		),
	); got != 2 {
		t.Fatalf(
			"stage_total{stage=%q} = %v, want 2",
			gatewayConvergenceStageInfrastructureConverged,
			got,
		)
	}
	if got := testutil.ToFloat64(
		gatewayConvergenceStageTotal.WithLabelValues(
			string(gatewayConvergenceStageProgrammed),
		),
	); got != 2 {
		t.Fatalf("stage_total{stage=%q} = %v, want 2", gatewayConvergenceStageProgrammed, got)
	}
}

func TestGatewayConvergenceStageCurrentMetricUsesGaugeName(t *testing.T) {
	resetGatewayConvergenceMetricsForTest()

	readyGateway := gatewayWithNameAndGenerationForConvergenceTest("gw-ready", 5)
	setCondition(&readyGateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonProgrammed),
		Message:            "Gateway is programmed",
		ObservedGeneration: readyGateway.Generation,
	})
	readyService := gatewayInfrastructureServiceForGateway(*readyGateway)
	readySlice := gatewayInfrastructureEndpointSlice(
		readyGateway.Namespace,
		readyGateway.Name,
		"gateway-frontend-endpoints",
	)
	readySlice.Annotations = cloneStringMap(readyService.Annotations)

	state := &clusterState{
		controllerName: string(gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")),
		gatewayClasses: []gatewayv1.GatewayClass{{
			ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
			Spec: gatewayv1.GatewayClassSpec{
				ControllerName: "gateway.networking.k8s.io/nantian-gw",
			},
		}},
		gateways:       []gatewayv1.Gateway{*readyGateway},
		services:       []corev1.Service{*readyService},
		endpointSlices: []discoveryv1.EndpointSlice{*readySlice},
	}
	state.index()

	syncGatewayConvergenceStageMetrics(evaluateGateways(state, nil))

	expected := strings.NewReader(`# HELP nantian_gateway_controlplane_gateway_convergence_stage_current Current number of managed Gateways that have reached each convergence stage in the latest status evaluation snapshot.
# TYPE nantian_gateway_controlplane_gateway_convergence_stage_current gauge
nantian_gateway_controlplane_gateway_convergence_stage_current{stage="infrastructure_converged"} 1
nantian_gateway_controlplane_gateway_convergence_stage_current{stage="managed"} 1
nantian_gateway_controlplane_gateway_convergence_stage_current{stage="programmed"} 1
nantian_gateway_controlplane_gateway_convergence_stage_current{stage="translated"} 1
`)
	if err := testutil.GatherAndCompare(
		ctrlmetrics.Registry,
		expected,
		"nantian_gateway_controlplane_gateway_convergence_stage_current",
	); err != nil {
		t.Fatal(err)
	}
}

func gatewayWithGenerationForConvergenceTest() *gatewayv1.Gateway {
	return gatewayWithNameAndGenerationForConvergenceTest("gw", 3)
}

func gatewayWithNameAndGenerationForConvergenceTest(name string, generation int64) *gatewayv1.Gateway {
	gateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: generation,
			UID:        types.UID(name + "-uid"),
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
			Listeners: []gatewayv1.Listener{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}
	return gateway
}

func resetGatewayConvergenceMetricsForTest() {
	gatewayConvergenceGenerationLag.Reset()
	gatewayProgrammedPendingTotal.Reset()
	gatewayConvergenceStageTotal.Reset()
	gatewayConvergenceStageCurrent.Reset()

	gatewayConvergenceStageTracker.mu.Lock()
	defer gatewayConvergenceStageTracker.mu.Unlock()
	gatewayConvergenceStageTracker.records = make(map[string]gatewayConvergenceProgress)
}

func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}

	out := make(map[string]string, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}
