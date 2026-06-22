package gwapi

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// EffectiveListeners returns the subset of Gateway listeners that should remain
// visible to snapshot translation and watch/index rebuilds.
func EffectiveListeners(gateway gatewayv1.Gateway) []gatewayv1.Listener {
	return filterListeners(gateway, snapshotListenerBlocked)
}

// InfrastructureListeners returns the subset of Gateway listeners that are
// ready to be exposed through derived Service and NetworkPolicy resources.
func InfrastructureListeners(gateway gatewayv1.Gateway) []gatewayv1.Listener {
	return filterListeners(gateway, infrastructureListenerBlocked)
}

func filterListeners(
	gateway gatewayv1.Gateway,
	blocked func(gatewayv1.ListenerStatus, int64) bool,
) []gatewayv1.Listener {
	if len(gateway.Spec.Listeners) == 0 {
		return nil
	}

	statusByName := make(map[gatewayv1.SectionName]gatewayv1.ListenerStatus, len(gateway.Status.Listeners))
	for _, status := range gateway.Status.Listeners {
		statusByName[status.Name] = status
	}

	out := make([]gatewayv1.Listener, 0, len(gateway.Spec.Listeners))
	for _, listener := range gateway.Spec.Listeners {
		status, ok := statusByName[listener.Name]
		if ok && blocked(status, gateway.Generation) {
			continue
		}
		out = append(out, listener)
	}

	return out
}

func snapshotListenerBlocked(status gatewayv1.ListenerStatus, generation int64) bool {
	if conditionFalseForGenerationWithReason(
		status.Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		generation,
		string(gatewayv1.ListenerReasonNoValidCACertificate),
	) {
		return false
	}
	return conditionExplicitlyFalseForGeneration(
		status.Conditions,
		string(gatewayv1.ListenerConditionAccepted),
		generation,
	)
}

func infrastructureListenerBlocked(status gatewayv1.ListenerStatus, generation int64) bool {
	return snapshotListenerBlocked(status, generation) ||
		conditionExplicitlyFalseForGeneration(
			status.Conditions,
			string(gatewayv1.ListenerConditionProgrammed),
			generation,
		)
}

func conditionExplicitlyFalseForGeneration(
	conditions []metav1.Condition,
	conditionType string,
	generation int64,
) bool {
	condition := meta.FindStatusCondition(conditions, conditionType)
	if condition == nil || condition.Status != metav1.ConditionFalse {
		return false
	}
	if generation == 0 || condition.ObservedGeneration == 0 {
		return true
	}
	return condition.ObservedGeneration == generation
}

func conditionFalseForGenerationWithReason(
	conditions []metav1.Condition,
	conditionType string,
	generation int64,
	reason string,
) bool {
	condition := meta.FindStatusCondition(conditions, conditionType)
	if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != reason {
		return false
	}
	if generation == 0 || condition.ObservedGeneration == 0 {
		return true
	}
	return condition.ObservedGeneration == generation
}
