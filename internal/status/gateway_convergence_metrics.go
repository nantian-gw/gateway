package status

import (
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/infrastructure"
)

const (
	gatewayConvergenceOwnerGenerationAnnotation = "nantian.dev/owner-generation"

	gatewayConvergenceStageServiceMetadata              = "service_metadata"
	gatewayConvergenceStageFrontendEndpointSlice        = "frontend_endpointslice"
	gatewayConvergenceStageProgrammedObservedGeneration = "programmed_observed_generation"
	gatewayProgrammedPendingReasonMissingCondition      = "MissingCondition"
	gatewayProgrammedPendingReasonOther                 = "Other"
)

type gatewayConvergenceStage string

const (
	gatewayConvergenceStageManaged                 gatewayConvergenceStage = "managed"
	gatewayConvergenceStageTranslated              gatewayConvergenceStage = "translated"
	gatewayConvergenceStageInfrastructureConverged gatewayConvergenceStage = "infrastructure_converged"
	gatewayConvergenceStageProgrammed              gatewayConvergenceStage = "programmed"
)

var (
	gatewayConvergenceGenerationLag = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nantian_gateway_controlplane_gateway_convergence_generation_lag",
			Help:    "Observed Gateway generation lag before status reconciliation, partitioned by convergence stage.",
			Buckets: []float64{1, 2, 3, 5, 8, 13},
		},
		[]string{"stage"},
	)
	gatewayProgrammedPendingTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nantian_gateway_controlplane_gateway_programmed_pending_total",
			Help: "Total number of Gateway status reconciles that observed a non-True Programmed condition before updating status, partitioned by the current reason.",
		},
		[]string{"reason"},
	)
	gatewayConvergenceStageTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_gateway_convergence_stage_total",
			Help: "Deprecated compatibility alias for nantian_gateway_controlplane_gateway_convergence_stage_current.",
		},
		[]string{"stage"},
	)
	gatewayConvergenceStageCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "nantian_gateway_controlplane_gateway_convergence_stage_current",
			Help: "Current number of managed Gateways that have reached each convergence stage in the latest status evaluation snapshot.",
		},
		[]string{"stage"},
	)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		gatewayConvergenceGenerationLag,
		gatewayProgrammedPendingTotal,
		gatewayConvergenceStageTotal,
		gatewayConvergenceStageCurrent,
	)
}

var gatewayConvergenceStageTracker = struct {
	mu      sync.Mutex
	records map[string]gatewayConvergenceProgress
}{
	records: make(map[string]gatewayConvergenceProgress),
}

type gatewayConvergenceObservation struct {
	serviceMetadataGenerationLag         int64
	frontendEndpointSliceGenerationLag   int64
	frontendEndpointSliceRefreshRequired bool
	programmedObservedGenerationLag      int64
	programmedPendingReason              string
}

type gatewayConvergenceProgress struct {
	translated     bool
	infraConverged bool
	programmed     bool
}

func observeGatewayConvergenceMetrics(observation gatewayConvergenceObservation) {
	if observation.serviceMetadataGenerationLag > 0 {
		gatewayConvergenceGenerationLag.WithLabelValues(
			gatewayConvergenceStageServiceMetadata,
		).Observe(float64(observation.serviceMetadataGenerationLag))
	}
	if observation.frontendEndpointSliceGenerationLag > 0 {
		gatewayConvergenceGenerationLag.WithLabelValues(
			gatewayConvergenceStageFrontendEndpointSlice,
		).Observe(float64(observation.frontendEndpointSliceGenerationLag))
	}
	if observation.programmedObservedGenerationLag > 0 {
		gatewayConvergenceGenerationLag.WithLabelValues(
			gatewayConvergenceStageProgrammedObservedGeneration,
		).Observe(float64(observation.programmedObservedGenerationLag))
	}
	if observation.programmedPendingReason != "" {
		gatewayProgrammedPendingTotal.WithLabelValues(observation.programmedPendingReason).Inc()
	}
}

func syncGatewayConvergenceStageMetrics(evals map[types.NamespacedName]gatewayEvaluation) {
	gatewayConvergenceStageTracker.mu.Lock()
	defer gatewayConvergenceStageTracker.mu.Unlock()

	next := make(map[string]gatewayConvergenceProgress, len(evals))
	for key, eval := range evals {
		next[namespacedName(key.Namespace, key.Name)] = gatewayConvergenceProgressForEvaluation(eval)
	}
	gatewayConvergenceStageTracker.records = next
	setGatewayConvergenceStageTotalsLocked()
}

func updateGatewayConvergenceStageMetric(key types.NamespacedName, eval gatewayEvaluation) {
	gatewayConvergenceStageTracker.mu.Lock()
	defer gatewayConvergenceStageTracker.mu.Unlock()

	gatewayConvergenceStageTracker.records[namespacedName(key.Namespace, key.Name)] = gatewayConvergenceProgressForEvaluation(eval)
	setGatewayConvergenceStageTotalsLocked()
}

func deleteGatewayConvergenceStageMetric(key types.NamespacedName) {
	gatewayConvergenceStageTracker.mu.Lock()
	defer gatewayConvergenceStageTracker.mu.Unlock()

	delete(gatewayConvergenceStageTracker.records, namespacedName(key.Namespace, key.Name))
	setGatewayConvergenceStageTotalsLocked()
}

func gatewayConvergenceProgressForEvaluation(eval gatewayEvaluation) gatewayConvergenceProgress {
	progress := gatewayConvergenceProgress{}
	if !eval.translationReady {
		return progress
	}

	progress.translated = true
	if !eval.infraConverged {
		return progress
	}

	progress.infraConverged = true
	progress.programmed = eval.programmedCondition.Status == metav1.ConditionTrue
	return progress
}

func setGatewayConvergenceStageTotalsLocked() {
	managed := len(gatewayConvergenceStageTracker.records)
	translated := 0
	infraConverged := 0
	programmed := 0

	for _, record := range gatewayConvergenceStageTracker.records {
		if record.translated {
			translated++
		}
		if record.infraConverged {
			infraConverged++
		}
		if record.programmed {
			programmed++
		}
	}

	setGatewayConvergenceStageValue(gatewayConvergenceStageManaged, managed)
	setGatewayConvergenceStageValue(gatewayConvergenceStageTranslated, translated)
	setGatewayConvergenceStageValue(gatewayConvergenceStageInfrastructureConverged, infraConverged)
	setGatewayConvergenceStageValue(gatewayConvergenceStageProgrammed, programmed)
}

func setGatewayConvergenceStageValue(stage gatewayConvergenceStage, value int) {
	label := string(stage)
	gatewayConvergenceStageTotal.WithLabelValues(label).Set(float64(value))
	gatewayConvergenceStageCurrent.WithLabelValues(label).Set(float64(value))
}

func gatewayConvergenceObservationForCurrentState(
	state *clusterState,
	gateway gatewayv1.Gateway,
) gatewayConvergenceObservation {
	observation := gatewayConvergenceObservation{}
	convergence := gatewayInfrastructureConvergenceState(state, gateway)

	switch {
	case !convergence.serviceReady:
		observation.serviceMetadataGenerationLag = gatewayServiceMetadataGenerationLag(state, gateway)
		return observation
	case !convergence.frontendEndpointSliceReady:
		observation.frontendEndpointSliceGenerationLag,
			observation.frontendEndpointSliceRefreshRequired = gatewayFrontendEndpointSliceGenerationLag(state, gateway)
		return observation
	}

	programmed := meta.FindStatusCondition(
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
	)
	if programmed == nil {
		observation.programmedObservedGenerationLag = maxGatewayGenerationLag(gateway.Generation, 0, false)
		observation.programmedPendingReason = gatewayProgrammedPendingReasonMissingCondition
		return observation
	}

	observation.programmedObservedGenerationLag = gatewayObservedGenerationLag(
		gateway.Generation,
		programmed.ObservedGeneration,
	)
	if programmed.Status != metav1.ConditionTrue {
		observation.programmedPendingReason = normalizeGatewayProgrammedPendingReason(programmed.Reason)
	}

	return observation
}

func normalizeGatewayProgrammedPendingReason(reason string) string {
	switch reason {
	case gatewayProgrammedPendingReasonMissingCondition,
		string(gatewayv1.GatewayReasonAccepted),
		string(gatewayv1.GatewayReasonAddressNotAssigned),
		string(gatewayv1.GatewayReasonAddressNotUsable),
		string(gatewayv1.GatewayReasonConfigurationChanged),
		string(gatewayv1.GatewayReasonInvalid),
		string(gatewayv1.GatewayReasonInvalidClientCertificateRef),
		string(gatewayv1.GatewayReasonInvalidParameters),
		string(gatewayv1.GatewayReasonListenersNotResolved),
		string(gatewayv1.GatewayReasonListenersNotValid),
		//nolint:staticcheck // SA1019: deprecated but still valid for backward-compatible reason normalization
		string(gatewayv1.GatewayReasonListenersNotReady),
		string(gatewayv1.GatewayReasonNoResources),
		//nolint:staticcheck // SA1019: deprecated but still valid for backward-compatible reason normalization
		string(gatewayv1.GatewayReasonNotReconciled),
		string(gatewayv1.GatewayReasonPending),
		string(gatewayv1.GatewayReasonProgrammed),
		//nolint:staticcheck // SA1019: deprecated but still valid for backward-compatible reason normalization
		string(gatewayv1.GatewayReasonReady),
		string(gatewayv1.GatewayReasonRefNotPermitted),
		string(gatewayv1.GatewayReasonResolvedRefs),
		//nolint:staticcheck // SA1019: deprecated but still valid for backward-compatible reason normalization
		string(gatewayv1.GatewayReasonScheduled),
		string(gatewayv1.GatewayReasonUnsupportedAddress):
		return reason
	default:
		return gatewayProgrammedPendingReasonOther
	}
}

func gatewayServiceMetadataGenerationLag(state *clusterState, gateway gatewayv1.Gateway) int64 {
	serviceKey := namespacedName(gateway.Namespace, infrastructure.GatewayServiceName(gateway.Name))
	service, ok := state.serviceByKey[serviceKey]
	if !ok {
		return maxGatewayGenerationLag(gateway.Generation, 0, false)
	}

	currentGeneration, ok := parseGatewayOwnedGeneration(service.Annotations)
	return maxGatewayGenerationLag(
		gateway.Generation,
		currentGeneration,
		ok && infrastructureServiceMetadataReady(state, gateway, service),
	)
}

func gatewayFrontendEndpointSliceGenerationLag(state *clusterState, gateway gatewayv1.Gateway) (int64, bool) {
	serviceKey := namespacedName(gateway.Namespace, infrastructure.GatewayServiceName(gateway.Name))
	service, ok := state.serviceByKey[serviceKey]
	if !ok {
		return maxGatewayGenerationLag(gateway.Generation, 0, false), false
	}

	endpointSlices := state.endpointSlicesByService[serviceKey]
	if len(endpointSlices) == 0 {
		return maxGatewayGenerationLag(gateway.Generation, 0, false), false
	}

	var maxObserved int64
	foundGeneration := false
	for _, endpointSlice := range endpointSlices {
		generation, ok := parseGatewayOwnedGeneration(endpointSlice.Annotations)
		if !ok {
			continue
		}
		if !foundGeneration || generation > maxObserved {
			maxObserved = generation
			foundGeneration = true
		}
	}

	lag := maxGatewayGenerationLag(
		gateway.Generation,
		maxObserved,
		hasManagedGatewayFrontendEndpointSlice(endpointSlices, service),
	)
	return lag, lag > 0
}

func parseGatewayOwnedGeneration(annotations map[string]string) (int64, bool) {
	if len(annotations) == 0 {
		return 0, false
	}

	raw, ok := annotations[gatewayConvergenceOwnerGenerationAnnotation]
	if !ok {
		return 0, false
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}

func maxGatewayGenerationLag(currentGeneration, observedGeneration int64, ready bool) int64 {
	if currentGeneration <= 0 {
		return 0
	}
	if ready {
		return 0
	}

	lag := currentGeneration - observedGeneration
	if lag <= 0 {
		return 1
	}
	return lag
}

func gatewayObservedGenerationLag(currentGeneration, observedGeneration int64) int64 {
	if currentGeneration <= 0 {
		return 0
	}
	if observedGeneration <= 0 {
		return currentGeneration
	}

	lag := currentGeneration - observedGeneration
	if lag < 0 {
		return 0
	}
	return lag
}
