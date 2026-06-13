package status

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func conditionSpecWithObservedGeneration(spec conditionSpec, generation int64) conditionSpec {
	spec.ObservedGeneration = generation
	return spec
}

func conditionWithObservedGeneration(condition metav1.Condition, generation int64) metav1.Condition {
	condition.ObservedGeneration = generation
	return condition
}

func gatewayEvaluationWithObservedGeneration(eval gatewayEvaluation, generation int64) gatewayEvaluation {
	eval.acceptedCondition = conditionSpecWithObservedGeneration(eval.acceptedCondition, generation)
	eval.programmedCondition = conditionSpecWithObservedGeneration(eval.programmedCondition, generation)
	for i := range eval.extraConditions {
		eval.extraConditions[i] = conditionSpecWithObservedGeneration(eval.extraConditions[i], generation)
	}
	for i := range eval.listeners {
		eval.listeners[i] = listenerEvaluationWithObservedGeneration(eval.listeners[i], generation)
	}
	return eval
}

func listenerEvaluationWithObservedGeneration(eval listenerEvaluation, generation int64) listenerEvaluation {
	eval.acceptedCondition = conditionSpecWithObservedGeneration(eval.acceptedCondition, generation)
	eval.resolvedCondition = conditionSpecWithObservedGeneration(eval.resolvedCondition, generation)
	eval.programmedCondition = conditionSpecWithObservedGeneration(eval.programmedCondition, generation)
	for i := range eval.extraConditions {
		eval.extraConditions[i] = conditionSpecWithObservedGeneration(eval.extraConditions[i], generation)
	}
	return eval
}

func listenerSetEvaluationWithObservedGeneration(eval listenerSetEvaluation, generation int64) listenerSetEvaluation {
	eval.accepted = conditionSpecWithObservedGeneration(eval.accepted, generation)
	eval.programmed = conditionSpecWithObservedGeneration(eval.programmed, generation)
	for i := range eval.listeners {
		for j := range eval.listeners[i].Conditions {
			eval.listeners[i].Conditions[j].ObservedGeneration = generation
		}
	}
	return eval
}

func listenerSetEvaluationObservedGeneration(eval listenerSetEvaluation) int64 {
	generation := eval.accepted.ObservedGeneration
	if eval.programmed.ObservedGeneration > generation {
		generation = eval.programmed.ObservedGeneration
	}
	for _, listener := range eval.listeners {
		for _, condition := range listener.Conditions {
			if condition.ObservedGeneration > generation {
				generation = condition.ObservedGeneration
			}
		}
	}
	return generation
}

func routeParentEvaluationsWithObservedGeneration(
	evals []routeParentEvaluation,
	generation int64,
) []routeParentEvaluation {
	if len(evals) == 0 {
		return nil
	}

	out := make([]routeParentEvaluation, len(evals))
	for i := range evals {
		out[i] = evals[i]
		out[i].acceptedCondition = conditionSpecWithObservedGeneration(out[i].acceptedCondition, generation)
		out[i].resolvedCondition = conditionSpecWithObservedGeneration(out[i].resolvedCondition, generation)
		for j := range out[i].extraConditions {
			out[i].extraConditions[j] = conditionSpecWithObservedGeneration(out[i].extraConditions[j], generation)
		}
	}
	return out
}

func policyAncestorsWithObservedGeneration(
	ancestors []gatewayv1.PolicyAncestorStatus,
	generation int64,
) []gatewayv1.PolicyAncestorStatus {
	if len(ancestors) == 0 {
		return nil
	}

	out := make([]gatewayv1.PolicyAncestorStatus, len(ancestors))
	for i := range ancestors {
		out[i] = ancestors[i]
		if len(ancestors[i].Conditions) == 0 {
			continue
		}
		out[i].Conditions = make([]metav1.Condition, len(ancestors[i].Conditions))
		for j := range ancestors[i].Conditions {
			out[i].Conditions[j] = conditionWithObservedGeneration(ancestors[i].Conditions[j], generation)
		}
	}
	return out
}
