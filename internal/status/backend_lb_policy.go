package status

import (
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/loadbalancing"
)

const (
	backendLBPolicyConditionResolvedRefs = "ResolvedRefs"
	backendLBPolicyReasonResolvedRefs    = "ResolvedRefs"
	backendLBPolicyReasonInvalidKind     = "InvalidKind"
)

type backendLBPolicyEvaluation struct {
	ancestors []backend.PolicyAncestorStatus
}

type backendLBPolicySpecEvaluation struct {
	generation        int64
	valid             bool
	claimsTargets     bool
	targetBackendKeys []string
	fallbackAncestors []gatewayv1.ParentReference
	acceptedCondition conditionSpec
	resolvedCondition conditionSpec
}

func evaluateBackendLBPolicies(
	state *clusterState,
	routeState routeState,
) map[client.ObjectKey]backendLBPolicyEvaluation {
	out := make(map[client.ObjectKey]backendLBPolicyEvaluation, len(state.backendLBPolicies))
	backendAncestors := collectBackendTLSPolicyAncestors(state, routeState)

	specEvals := make(map[client.ObjectKey]backendLBPolicySpecEvaluation, len(state.backendLBPolicies))
	policyByKey := make(map[client.ObjectKey]backend.BackendLBPolicy, len(state.backendLBPolicies))
	winners := make(map[string]client.ObjectKey)

	for _, policy := range state.backendLBPolicies {
		key := client.ObjectKeyFromObject(&policy)
		policyByKey[key] = policy
		eval := evaluateBackendLBPolicySpec(state, policy)
		specEvals[key] = eval
		if !eval.valid || !eval.claimsTargets {
			continue
		}
		for _, backendKey := range eval.targetBackendKeys {
			currentWinner, exists := winners[backendKey]
			if !exists || loadbalancing.PolicyPrecedes(policy, policyByKey[currentWinner]) {
				winners[backendKey] = key
			}
		}
	}

	for _, policy := range state.backendLBPolicies {
		key := client.ObjectKeyFromObject(&policy)
		eval := specEvals[key]
		switch {
		case !eval.valid:
			out[key] = backendLBPolicyEvaluation{
				ancestors: policyStatusesForAncestors(
					fallbackPolicyAncestors(eval.fallbackAncestors),
					state.controllerName,
					eval.acceptedCondition,
					eval.resolvedCondition,
				),
			}
		case eval.claimsTargets && policyConflicted(eval.targetBackendKeys, key, winners):
			ancestors := policyGatewayAncestors(eval.targetBackendKeys, backendAncestors)
			if len(ancestors) == 0 {
				ancestors = fallbackPolicyAncestors(eval.fallbackAncestors)
			}
			out[key] = backendLBPolicyEvaluation{
				ancestors: policyStatusesForAncestors(
					ancestors,
					state.controllerName,
					acceptedPolicyCondition(
						eval.generation,
						metav1.ConditionFalse,
						string(backend.PolicyReasonConflicted),
						"BackendLBPolicy conflicts with another policy targeting the same backend",
					),
					backendLBResolvedPolicyCondition(
						eval.generation,
						metav1.ConditionTrue,
						backendLBPolicyReasonResolvedRefs,
						"BackendLBPolicy references are resolved",
					),
				),
			}
		default:
			ancestors := policyGatewayAncestors(eval.targetBackendKeys, backendAncestors)
			if len(ancestors) == 0 {
				ancestors = fallbackPolicyAncestors(eval.fallbackAncestors)
			}
			out[key] = backendLBPolicyEvaluation{
				ancestors: policyStatusesForAncestors(
					ancestors,
					state.controllerName,
					eval.acceptedCondition,
					eval.resolvedCondition,
				),
			}
		}
	}

	return out
}

func evaluateBackendLBPolicySpec(
	state *clusterState,
	policy backend.BackendLBPolicy,
) backendLBPolicySpecEvaluation {
	eval := backendLBPolicySpecEvaluation{
		generation:        policy.Generation,
		claimsTargets:     policy.Spec.SessionPersistence != nil || policy.Spec.LoadBalancing != nil,
		acceptedCondition: acceptedPolicyCondition(policy.Generation, metav1.ConditionTrue, string(backend.PolicyReasonAccepted), "BackendLBPolicy is accepted by nantian-gw"),
		resolvedCondition: backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionTrue, backendLBPolicyReasonResolvedRefs, "BackendLBPolicy references are resolved"),
	}

	if len(policy.Spec.TargetRefs) == 0 {
		eval.acceptedCondition = invalidAcceptedPolicyCondition(policy.Generation, "BackendLBPolicy must target at least one backend")
		return eval
	}
	if !eval.claimsTargets {
		message := "BackendLBPolicy must configure at least one supported backend policy feature"
		eval.acceptedCondition = invalidAcceptedPolicyCondition(policy.Generation, message)
		eval.resolvedCondition = backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonInvalid), message)
		return eval
	}
	if err := loadbalancing.ValidateLoadBalancing(policy.Spec.LoadBalancing); err != nil {
		eval.acceptedCondition = invalidAcceptedPolicyCondition(policy.Generation, err.Error())
		eval.resolvedCondition = backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonInvalid), err.Error())
		return eval
	}
	if err := loadbalancing.ValidateSessionPersistence(policy.Spec.SessionPersistence); err != nil {
		eval.acceptedCondition = invalidAcceptedPolicyCondition(policy.Generation, err.Error())
		eval.resolvedCondition = backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonInvalid), err.Error())
		return eval
	}

	keys, ancestors, accepted, resolved, ok := backendLBPolicyTargets(state, policy)
	eval.fallbackAncestors = ancestors
	if !ok {
		eval.acceptedCondition = accepted
		eval.resolvedCondition = resolved
		return eval
	}

	eval.valid = true
	eval.targetBackendKeys = keys
	return eval
}

func backendLBPolicyTargets(
	state *clusterState,
	policy backend.BackendLBPolicy,
) ([]string, []gatewayv1.ParentReference, conditionSpec, conditionSpec, bool) {
	keys := make([]string, 0)
	ancestors := make([]gatewayv1.ParentReference, 0, len(policy.Spec.TargetRefs))

	for _, targetRef := range policy.Spec.TargetRefs {
		ancestor := backendLBPolicyTargetAncestor(policy.Namespace, targetRef)
		ancestors = append(ancestors, ancestor)

		group := string(targetRef.Group)
		kind := string(targetRef.Kind)
		switch {
		case group == "" && kind == "Service":
			service, ok := state.serviceByKey[namespacedName(policy.Namespace, string(targetRef.Name))]
			if !ok {
				message := "BackendLBPolicy target Service was not found"
				return nil, ancestors,
					acceptedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					false
			}
			targetKeys, ok := backendTLSPolicyServiceKeys(policy.Namespace, service.Spec.Ports, nil, string(targetRef.Name))
			if !ok {
				message := "BackendLBPolicy target did not resolve to any backend ports"
				return nil, ancestors,
					acceptedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					false
			}
			keys = append(keys, targetKeys...)
		case group == mcsv1alpha1.GroupName && kind == "ServiceImport":
			serviceImport, ok := state.serviceImportByKey[namespacedName(policy.Namespace, string(targetRef.Name))]
			if !ok {
				message := "BackendLBPolicy target ServiceImport was not found"
				return nil, ancestors,
					acceptedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					false
			}
			targetKeys, ok := backendTLSPolicyServiceImportKeys(policy.Namespace, serviceImport.Spec.Ports, nil, string(targetRef.Name))
			if !ok {
				message := "BackendLBPolicy target did not resolve to any backend ports"
				return nil, ancestors,
					acceptedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
					false
			}
			keys = append(keys, targetKeys...)
		default:
			message := "BackendLBPolicy targetRef kind is not supported"
			return nil, ancestors,
				acceptedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonInvalid), message),
				backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, backendLBPolicyReasonInvalidKind, message),
				false
		}
	}

	sort.Strings(keys)
	keys = compactPolicyBackendKeys(keys)
	if len(keys) == 0 {
		message := "BackendLBPolicy target did not resolve to any backend ports"
		return nil, ancestors,
			acceptedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
			backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, string(backend.PolicyReasonTargetNotFound), message),
			false
	}

	return keys, ancestors,
		acceptedPolicyCondition(policy.Generation, metav1.ConditionTrue, string(backend.PolicyReasonAccepted), "BackendLBPolicy is accepted by nantian-gw"),
		backendLBResolvedPolicyCondition(policy.Generation, metav1.ConditionTrue, backendLBPolicyReasonResolvedRefs, "BackendLBPolicy references are resolved"),
		true
}

func backendLBPolicyTargetAncestor(
	namespace string,
	targetRef backend.LocalPolicyTargetReference,
) gatewayv1.ParentReference {
	return gatewayv1.ParentReference{
		Group:     groupPtr(string(targetRef.Group)),
		Kind:      kindPtr(string(targetRef.Kind)),
		Name:      targetRef.Name,
		Namespace: gatewayNamespacePtr(namespace),
	}
}

func backendLBResolvedPolicyCondition(
	generation int64,
	status metav1.ConditionStatus,
	reason string,
	message string,
) conditionSpec {
	return conditionSpec{
		Type:               backendLBPolicyConditionResolvedRefs,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}
}
