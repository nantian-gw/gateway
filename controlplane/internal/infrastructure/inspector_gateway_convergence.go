package infrastructure

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const gatewayConvergenceOwnerGenerationAnnotation = "nantian.dev/owner-generation"

func summarizeGatewayConvergence(
	gateways []gatewayv1.Gateway,
	expectedServices map[string]infrastructureExpectation,
	serviceIndex map[string]corev1.Service,
	expectedSlices map[string]infrastructureExpectation,
	sliceIndex map[string]discoveryv1.EndpointSlice,
) GatewayConvergenceSummary {
	summary := GatewayConvergenceSummary{
		GatewayCount: len(gateways),
	}

	for _, gateway := range gateways {
		serviceLag, serviceReady := gatewayServiceConvergenceForInspector(
			gateway,
			expectedServices,
			serviceIndex,
		)
		if !serviceReady {
			summary.PendingServiceMetadataCount++
			if serviceLag > summary.MaxServiceMetadataGenerationLag {
				summary.MaxServiceMetadataGenerationLag = serviceLag
			}
			continue
		}

		sliceLag, slicesReady := gatewayFrontendSliceConvergenceForInspector(
			gateway,
			expectedSlices,
			sliceIndex,
		)
		if !slicesReady {
			summary.PendingFrontendEndpointSliceCount++
			if sliceLag > summary.MaxFrontendEndpointSliceGenerationLag {
				summary.MaxFrontendEndpointSliceGenerationLag = sliceLag
			}
			continue
		}

		programmedLag, programmedReady := gatewayProgrammedConvergenceForInspector(gateway)
		if !programmedReady {
			summary.PendingProgrammedObservedGenerationCount++
			if programmedLag > summary.MaxProgrammedObservedGenerationLag {
				summary.MaxProgrammedObservedGenerationLag = programmedLag
			}
			continue
		}

		summary.ReadyCount++
	}

	return summary
}

func gatewayServiceConvergenceForInspector(
	gateway gatewayv1.Gateway,
	expectedServices map[string]infrastructureExpectation,
	serviceIndex map[string]corev1.Service,
) (int64, bool) {
	key := serviceKey(gateway.Namespace, gatewayServiceName(gateway.Name))
	expected, ok := expectedServices[key]
	if !ok || expected.service == nil {
		return 0, true
	}

	current, ok := serviceIndex[key]
	if !ok || !serviceEqual(&current, expected.service) {
		return gatewayOwnedGenerationLag(gateway.Generation, current.Annotations), false
	}

	return 0, true
}

func gatewayFrontendSliceConvergenceForInspector(
	gateway gatewayv1.Gateway,
	expectedSlices map[string]infrastructureExpectation,
	sliceIndex map[string]discoveryv1.EndpointSlice,
) (int64, bool) {
	ready := true
	var maxLag int64
	for key, expected := range expectedSlices {
		if expected.resource.Role != InfrastructureRoleGatewaySlice ||
			expected.resource.OwnerNamespace != gateway.Namespace ||
			expected.resource.OwnerName != gateway.Name ||
			expected.endpointSlice == nil {
			continue
		}

		current, ok := sliceIndex[key]
		if ok && endpointSliceEqual(&current, expected.endpointSlice) {
			continue
		}

		ready = false
		lag := gatewayOwnedGenerationLag(gateway.Generation, current.Annotations)
		if lag > maxLag {
			maxLag = lag
		}
	}

	return maxLag, ready
}

func gatewayProgrammedConvergenceForInspector(gateway gatewayv1.Gateway) (int64, bool) {
	programmed := meta.FindStatusCondition(
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionProgrammed),
	)
	if programmed == nil {
		return gatewayObservedGenerationLag(gateway.Generation, 0), false
	}
	if programmed.Status != metav1.ConditionTrue {
		return gatewayObservedGenerationLag(gateway.Generation, programmed.ObservedGeneration), false
	}
	if programmed.ObservedGeneration < gateway.Generation {
		return gatewayObservedGenerationLag(gateway.Generation, programmed.ObservedGeneration), false
	}

	return 0, true
}

func gatewayOwnedGenerationLag(currentGeneration int64, annotations map[string]string) int64 {
	if currentGeneration <= 0 {
		return 0
	}
	if len(annotations) == 0 {
		return currentGeneration
	}

	raw, ok := annotations[gatewayConvergenceOwnerGenerationAnnotation]
	if !ok {
		return currentGeneration
	}
	observedGeneration, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return currentGeneration
	}
	return gatewayObservedGenerationLag(currentGeneration, observedGeneration)
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
