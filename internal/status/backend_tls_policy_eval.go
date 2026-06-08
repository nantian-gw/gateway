package status

import (
	"crypto/x509"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/backendtls"
)

func evaluateBackendTLSPolicies(
	state *clusterState,
	routeState routeState,
) map[client.ObjectKey]backendTLSPolicyEvaluation {
	out := make(map[client.ObjectKey]backendTLSPolicyEvaluation, len(state.backendTLSPolicies))
	backendAncestors := collectBackendTLSPolicyAncestors(state, routeState)

	specEvals := make(map[client.ObjectKey]backendTLSPolicySpecEvaluation, len(state.backendTLSPolicies))
	policyByKey := make(map[client.ObjectKey]gatewayv1alpha3.BackendTLSPolicy, len(state.backendTLSPolicies))
	winners := make(map[string]client.ObjectKey)

	for _, policy := range state.backendTLSPolicies {
		key := client.ObjectKeyFromObject(&policy)
		policyByKey[key] = policy
		eval := evaluateBackendTLSPolicySpec(state, policy)
		specEvals[key] = eval
		if !eval.valid {
			continue
		}
		for _, claimKey := range eval.claimKeys {
			currentWinner, exists := winners[claimKey]
			if !exists || backendtls.PolicyPrecedes(policy, policyByKey[currentWinner]) {
				winners[claimKey] = key
			}
		}
	}

	for _, policy := range state.backendTLSPolicies {
		key := client.ObjectKeyFromObject(&policy)
		eval := specEvals[key]
		switch {
		case !eval.valid:
			ancestors := policyGatewayAncestors(eval.targetBackendKeys, backendAncestors)
			if len(ancestors) == 0 {
				ancestors = fallbackPolicyAncestors(eval.fallbackAncestors)
			}
			out[key] = backendTLSPolicyEvaluation{
				ancestors: policyStatusesForAncestors(
					ancestors,
					state.controllerName,
					eval.acceptedCondition,
					eval.resolvedCondition,
				),
			}
		case policyConflicted(eval.claimKeys, key, winners):
			ancestors := policyGatewayAncestors(eval.targetBackendKeys, backendAncestors)
			if len(ancestors) == 0 {
				ancestors = fallbackPolicyAncestors(eval.fallbackAncestors)
			}
			out[key] = backendTLSPolicyEvaluation{
				ancestors: policyStatusesForAncestors(
					ancestors,
					state.controllerName,
					acceptedPolicyCondition(
						eval.generation,
						metav1.ConditionFalse,
						string(gatewayv1.PolicyReasonConflicted),
						"BackendTLSPolicy conflicts with another policy targeting the same backend",
					),
					eval.resolvedCondition,
				),
			}
		default:
			ancestors := policyGatewayAncestors(eval.targetBackendKeys, backendAncestors)
			if len(ancestors) == 0 {
				ancestors = fallbackPolicyAncestors(eval.fallbackAncestors)
			}
			out[key] = backendTLSPolicyEvaluation{
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

func evaluateBackendTLSPolicySpec(
	state *clusterState,
	policy gatewayv1alpha3.BackendTLSPolicy,
) backendTLSPolicySpecEvaluation {
	eval := backendTLSPolicySpecEvaluation{
		generation:        policy.Generation,
		acceptedCondition: acceptedPolicyCondition(policy.Generation, metav1.ConditionTrue, string(gatewayv1.PolicyReasonAccepted), "BackendTLSPolicy is accepted by nantian-gw"),
		resolvedCondition: resolvedPolicyCondition(policy.Generation, metav1.ConditionTrue, backendTLSPolicyReasonResolvedRefs, "BackendTLSPolicy references are resolved"),
	}

	if len(policy.Spec.TargetRefs) == 0 {
		eval.acceptedCondition = invalidAcceptedPolicyCondition(policy.Generation, "BackendTLSPolicy must target at least one backend")
		return eval
	}

	validationAccepted, validationResolved, validationValid := backendTLSPolicyValidationSupported(state, policy)
	eval.acceptedCondition = validationAccepted
	eval.resolvedCondition = validationResolved
	if !validationValid {
		eval.fallbackAncestors = rawPolicyFallbackAncestors(policy)
	} else {
		eval.acceptedCondition = validationAccepted
		eval.resolvedCondition = validationResolved
	}

	keys, claimKeys, ancestors, accepted, resolved, ok := backendTLSPolicyTargets(
		state,
		policy,
	)
	eval.fallbackAncestors = ancestors
	if ok {
		eval.claimKeys = claimKeys
		eval.targetBackendKeys = keys
	}

	if !validationValid {
		if !ok {
			eval.acceptedCondition = preferFailedCondition(eval.acceptedCondition, accepted)
			eval.resolvedCondition = preferFailedCondition(eval.resolvedCondition, resolved)
		}
		return eval
	}

	if !ok {
		eval.acceptedCondition = accepted
		eval.resolvedCondition = resolved
		return eval
	}

	eval.acceptedCondition = preferFailedCondition(eval.acceptedCondition, accepted)
	eval.resolvedCondition = preferFailedCondition(eval.resolvedCondition, resolved)
	eval.valid = true
	eval.claimKeys = claimKeys
	eval.targetBackendKeys = keys
	return eval
}

func preferFailedCondition(current, next conditionSpec) conditionSpec {
	if current.Status == metav1.ConditionFalse {
		return current
	}
	return next
}

func backendTLSPolicyValidationSupported(
	state *clusterState,
	policy gatewayv1alpha3.BackendTLSPolicy,
) (conditionSpec, conditionSpec, bool) {
	resolvedRefs := resolvedPolicyCondition(
		policy.Generation,
		metav1.ConditionTrue,
		backendTLSPolicyReasonResolvedRefs,
		"BackendTLSPolicy references are resolved",
	)
	validation := policy.Spec.Validation
	if validation.Hostname == "" {
		return invalidAcceptedPolicyCondition(policy.Generation, "BackendTLSPolicy validation.hostname is required"), resolvedRefs, false
	}

	hasCustomCAs := len(validation.CACertificateRefs) > 0
	hasWellKnown := validation.WellKnownCACertificates != nil
	switch {
	case hasCustomCAs && hasWellKnown:
		return invalidAcceptedPolicyCondition(policy.Generation, "BackendTLSPolicy must not specify both caCertificateRefs and wellKnownCACertificates"), resolvedRefs, false
	case !hasCustomCAs && !hasWellKnown:
		return invalidAcceptedPolicyCondition(policy.Generation, "BackendTLSPolicy must specify either caCertificateRefs or wellKnownCACertificates"), resolvedRefs, false
	case hasCustomCAs:
		validCARefs := 0
		resolvedReason := ""
		resolvedMessage := ""
		for _, ref := range validation.CACertificateRefs {
			if string(ref.Group) != "" {
				if resolvedMessage == "" {
					resolvedReason = backendTLSPolicyReasonInvalidKind
					resolvedMessage = fmt.Sprintf(
						"BackendTLSPolicy CA ref %q uses unsupported group %q",
						ref.Name,
						ref.Group,
					)
				}
				continue
			}

			kind := string(ref.Kind)
			if kind == "" {
				kind = "ConfigMap"
			}
			if kind != "ConfigMap" {
				if resolvedMessage == "" {
					resolvedReason = backendTLSPolicyReasonInvalidKind
					resolvedMessage = fmt.Sprintf(
						"BackendTLSPolicy CA ref %q uses unsupported kind %q",
						ref.Name,
						kind,
					)
				}
				continue
			}

			configMap, ok := state.configMapByKey[namespacedName(policy.Namespace, string(ref.Name))]
			caPEM := []byte(configMap.Data["ca.crt"])
			if !ok || len(caPEM) == 0 {
				if resolvedMessage == "" {
					resolvedReason = backendTLSPolicyReasonInvalidCACertRef
					resolvedMessage = fmt.Sprintf(
						"BackendTLSPolicy CA ref ConfigMap %s/%s was not found or does not contain ca.crt",
						policy.Namespace,
						ref.Name,
					)
				}
				continue
			}

			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPEM) {
				if resolvedMessage == "" {
					resolvedReason = backendTLSPolicyReasonInvalidCACertRef
					resolvedMessage = fmt.Sprintf(
						"BackendTLSPolicy CA ref ConfigMap %s/%s does not contain a valid PEM certificate bundle",
						policy.Namespace,
						ref.Name,
					)
				}
				continue
			}

			validCARefs++
		}

		if validCARefs == 0 {
			if resolvedMessage == "" {
				resolvedReason = backendTLSPolicyReasonInvalidCACertRef
				resolvedMessage = "BackendTLSPolicy does not contain any valid CA certificate references"
			}
			return noValidCACertificateAcceptedCondition(policy.Generation),
				resolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, resolvedReason, resolvedMessage),
				false
		}
		if resolvedMessage != "" {
			resolvedRefs = resolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, resolvedReason, resolvedMessage)
		}
	default:
		if *validation.WellKnownCACertificates != gatewayv1.WellKnownCACertificatesSystem {
			return invalidAcceptedPolicyCondition(policy.Generation, "BackendTLSPolicy wellKnownCACertificates value is not supported"), resolvedRefs, false
		}
	}
	if _, err := backendtls.ParseSubjectAltNames(validation.SubjectAltNames); err != nil {
		return invalidAcceptedPolicyCondition(policy.Generation, err.Error()), resolvedRefs, false
	}

	if _, err := backendtls.ParseOptions(policy.Spec.Options); err != nil {
		return invalidAcceptedPolicyCondition(policy.Generation, err.Error()), resolvedRefs, false
	}

	return acceptedPolicyCondition(policy.Generation, metav1.ConditionTrue, string(gatewayv1.PolicyReasonAccepted), "BackendTLSPolicy is accepted by nantian-gw"), resolvedRefs, true
}

func backendTLSPolicyTargets(
	state *clusterState,
	policy gatewayv1alpha3.BackendTLSPolicy,
) ([]string, []string, []gatewayv1.ParentReference, conditionSpec, conditionSpec, bool) {
	claimKeys := make([]string, 0)
	keys := make([]string, 0)
	ancestors := make([]gatewayv1.ParentReference, 0, len(policy.Spec.TargetRefs))
	firstResolvedReason := ""
	firstResolvedMessage := ""
	firstAcceptedReason := ""
	firstAcceptedMessage := ""

	for _, targetRef := range policy.Spec.TargetRefs {
		ancestor := policyTargetAncestor(policy.Namespace, targetRef)
		ancestors = append(ancestors, ancestor)

		group := string(targetRef.Group)
		kind := string(targetRef.Kind)
		switch {
		case group == "" && kind == "Service":
			service, ok := state.serviceByKey[namespacedName(policy.Namespace, string(targetRef.Name))]
			if !ok {
				if firstAcceptedMessage == "" {
					firstAcceptedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstAcceptedMessage = "BackendTLSPolicy target Service was not found"
				}
				if firstResolvedMessage == "" {
					firstResolvedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstResolvedMessage = "BackendTLSPolicy target Service was not found"
				}
				continue
			}
			targetKeys, ok := backendTLSPolicyServiceKeys(policy.Namespace, service.Spec.Ports, targetRef.SectionName, string(targetRef.Name))
			if !ok {
				if firstAcceptedMessage == "" {
					firstAcceptedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstAcceptedMessage = "BackendTLSPolicy target Service port was not found"
				}
				if firstResolvedMessage == "" {
					firstResolvedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstResolvedMessage = "BackendTLSPolicy target Service port was not found"
				}
				continue
			}
			keys = append(keys, targetKeys...)
			claimKeys = append(claimKeys, backendTLSPolicyClaimKey(policy.Namespace, targetRef))
		case group == mcsv1alpha1.GroupName && kind == "ServiceImport":
			serviceImport, ok := state.serviceImportByKey[namespacedName(policy.Namespace, string(targetRef.Name))]
			if !ok {
				if firstAcceptedMessage == "" {
					firstAcceptedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstAcceptedMessage = "BackendTLSPolicy target ServiceImport was not found"
				}
				if firstResolvedMessage == "" {
					firstResolvedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstResolvedMessage = "BackendTLSPolicy target ServiceImport was not found"
				}
				continue
			}
			targetKeys, ok := backendTLSPolicyServiceImportKeys(policy.Namespace, serviceImport.Spec.Ports, targetRef.SectionName, string(targetRef.Name))
			if !ok {
				if firstAcceptedMessage == "" {
					firstAcceptedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstAcceptedMessage = "BackendTLSPolicy target ServiceImport port was not found"
				}
				if firstResolvedMessage == "" {
					firstResolvedReason = string(gatewayv1.PolicyReasonTargetNotFound)
					firstResolvedMessage = "BackendTLSPolicy target ServiceImport port was not found"
				}
				continue
			}
			keys = append(keys, targetKeys...)
			claimKeys = append(claimKeys, backendTLSPolicyClaimKey(policy.Namespace, targetRef))
		default:
			if firstAcceptedMessage == "" {
				firstAcceptedReason = string(gatewayv1.PolicyReasonInvalid)
				firstAcceptedMessage = "BackendTLSPolicy targetRef kind is not supported"
			}
			if firstResolvedMessage == "" {
				firstResolvedReason = backendTLSPolicyReasonInvalidKind
				firstResolvedMessage = "BackendTLSPolicy targetRef kind is not supported"
			}
			continue
		}
	}

	sort.Strings(keys)
	keys = compactPolicyBackendKeys(keys)
	sort.Strings(claimKeys)
	claimKeys = compactPolicyBackendKeys(claimKeys)
	if len(keys) == 0 {
		message := firstAcceptedMessage
		reason := firstAcceptedReason
		resolvedReason := firstResolvedReason
		resolvedMessage := firstResolvedMessage
		if message == "" {
			message = "BackendTLSPolicy target did not resolve to any backend ports"
			reason = string(gatewayv1.PolicyReasonTargetNotFound)
			resolvedReason = string(gatewayv1.PolicyReasonTargetNotFound)
			resolvedMessage = message
		}
		return nil, nil, ancestors,
			acceptedPolicyCondition(policy.Generation, metav1.ConditionFalse, reason, message),
			resolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, resolvedReason, resolvedMessage),
			false
	}

	accepted := acceptedPolicyCondition(policy.Generation, metav1.ConditionTrue, string(gatewayv1.PolicyReasonAccepted), "BackendTLSPolicy is accepted by nantian-gw")
	resolved := resolvedPolicyCondition(policy.Generation, metav1.ConditionTrue, backendTLSPolicyReasonResolvedRefs, "BackendTLSPolicy references are resolved")
	if firstResolvedMessage != "" {
		resolved = resolvedPolicyCondition(policy.Generation, metav1.ConditionFalse, firstResolvedReason, firstResolvedMessage)
	}
	return keys, claimKeys, ancestors,
		accepted,
		resolved,
		true
}
