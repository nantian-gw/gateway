package status

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

type listenerSetEvaluation struct {
	accepted   conditionSpec
	programmed conditionSpec
	listeners  []gatewayv1.ListenerEntryStatus
}

func evaluateListenerSets(
	state *clusterState,
	lses []gatewayv1.ListenerSet,
	managedGateways map[string]gatewayv1.Gateway,
	attachments map[listenerKey]routeAttachmentSet,
) map[string]listenerSetEvaluation {
	out := make(map[string]listenerSetEvaluation, len(lses))
	gwToLSes := groupListenerSetsByGateway(lses)

	for gwKey, gwLSes := range gwToLSes {
		gw, ok := managedGateways[gwKey]
		if !ok {
			continue
		}
		sortListenerSetsByPrecedence(gwLSes)

		conflictSet := make([]gatewayv1.Listener, len(gw.Spec.Listeners))
		copy(conflictSet, gw.Spec.Listeners)

		for _, ls := range gwLSes {
			eval := evaluateOneListenerSet(ls, gw, conflictSet, state, attachments)
			key := ls.Namespace + "/" + ls.Name
			out[key] = eval

			if eval.accepted.Status == metav1.ConditionTrue {
				for _, entry := range ls.Spec.Listeners {
					if evaluateListenerSetEntry(entry, ls, conflictSet, state).valid {
						conflictSet = append(conflictSet, listenerEntryToInternalListener(entry, ls))
					}
				}
			}
		}
	}
	return out
}

func groupListenerSetsByGateway(lses []gatewayv1.ListenerSet) map[string][]gatewayv1.ListenerSet {
	out := make(map[string][]gatewayv1.ListenerSet)
	for _, ls := range lses {
		gwKey := listenerSetParentGatewayKey(ls)
		if gwKey == "" {
			continue
		}
		out[gwKey] = append(out[gwKey], ls)
	}
	return out
}

func listenerSetParentGatewayKey(ls gatewayv1.ListenerSet) string {
	if string(ls.Spec.ParentRef.Name) == "" {
		return ""
	}
	return namespacedName(listenerSetParentGatewayNamespace(ls), string(ls.Spec.ParentRef.Name))
}

func listenerSetParentGatewayNamespace(ls gatewayv1.ListenerSet) string {
	return namespaceOrDefault(ls.Spec.ParentRef.Namespace, ls.Namespace)
}

func evaluateOneListenerSet(
	ls gatewayv1.ListenerSet,
	gw gatewayv1.Gateway,
	conflictListeners []gatewayv1.Listener,
	state *clusterState,
	attachments map[listenerKey]routeAttachmentSet,
) listenerSetEvaluation {
	allowed := gatewayAllowsListenerSet(gw, ls, state.namespaceByName)
	if !allowed {
		return disallowedListenerSetEvaluation(ls)
	}

	listenerStatuses := make([]gatewayv1.ListenerEntryStatus, 0, len(ls.Spec.Listeners))
	hasValidListener := len(ls.Spec.Listeners) == 0

	for _, entry := range ls.Spec.Listeners {
		entryEval := evaluateListenerSetEntry(entry, ls, conflictListeners, state)
		if entryEval.valid {
			hasValidListener = true
		}
		entryEval.status.AttachedRoutes = listenerSetEntryAttachedRoutes(gw, ls, entry, attachments)
		listenerStatuses = append(listenerStatuses, entryEval.status)
	}

	accepted := conditionSpec{Type: string(gatewayv1.ListenerSetConditionAccepted), ObservedGeneration: ls.Generation}
	programmed := conditionSpec{Type: string(gatewayv1.ListenerSetConditionProgrammed), ObservedGeneration: ls.Generation}

	if hasValidListener {
		accepted.Status = metav1.ConditionTrue
		accepted.Reason = string(gatewayv1.ListenerSetReasonAccepted)
		accepted.Message = "ListenerSet is accepted"
		programmed.Status = metav1.ConditionTrue
		programmed.Reason = string(gatewayv1.ListenerSetReasonProgrammed)
		programmed.Message = "ListenerSet listeners are programmed"
	} else {
		accepted.Status = metav1.ConditionFalse
		accepted.Reason = string(gatewayv1.ListenerSetReasonListenersNotValid)
		accepted.Message = "ListenerSet has no valid listeners"
		programmed.Status = metav1.ConditionFalse
		programmed.Reason = string(gatewayv1.ListenerSetReasonListenersNotValid)
		programmed.Message = "ListenerSet has no valid listeners"
	}

	return listenerSetEvaluation{accepted: accepted, programmed: programmed, listeners: listenerStatuses}
}

func disallowedListenerSetEvaluation(ls gatewayv1.ListenerSet) listenerSetEvaluation {
	return listenerSetEvaluation{
		accepted: conditionSpec{
			Type: string(gatewayv1.ListenerSetConditionAccepted), Status: metav1.ConditionFalse,
			Reason: string(gatewayv1.ListenerSetReasonNotAllowed), Message: "ListenerSet is not allowed by the Gateway's allowedListeners policy",
			ObservedGeneration: ls.Generation,
		},
		programmed: conditionSpec{
			Type: string(gatewayv1.ListenerSetConditionProgrammed), Status: metav1.ConditionFalse,
			Reason: string(gatewayv1.ListenerSetReasonNotAllowed), Message: "ListenerSet is not allowed by the Gateway's allowedListeners policy",
			ObservedGeneration: ls.Generation,
		},
		listeners: buildDisallowedListenerSetListenerStatuses(ls),
	}
}

type listenerSetEntryEval struct {
	valid  bool
	status gatewayv1.ListenerEntryStatus
}

func evaluateListenerSetEntry(
	entry gatewayv1.ListenerEntry,
	ls gatewayv1.ListenerSet,
	conflictListeners []gatewayv1.Listener,
	state *clusterState,
) listenerSetEntryEval {
	entryListener := listenerEntryToInternalListener(entry, ls)
	policy := buildListenerPolicy(entryListener)

	resolvedRefsCond := metav1.Condition{Type: string(gatewayv1.ListenerConditionResolvedRefs), Status: metav1.ConditionTrue, Reason: string(gatewayv1.ListenerReasonResolvedRefs), Message: "Listener references are resolved", ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now()}
	acceptedCond := metav1.Condition{Type: string(gatewayv1.ListenerConditionAccepted), Status: metav1.ConditionTrue, Reason: string(gatewayv1.ListenerReasonAccepted), Message: "Listener is accepted", ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now()}
	programmedCond := metav1.Condition{Type: string(gatewayv1.ListenerConditionProgrammed), Status: metav1.ConditionTrue, Reason: string(gatewayv1.ListenerReasonProgrammed), Message: "Listener is programmed", ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now()}
	var extraConditions []metav1.Condition
	entryValid := true

	if policy.invalidKindRefs {
		resolvedRefsCond.Status = metav1.ConditionFalse
		resolvedRefsCond.Reason = string(gatewayv1.ListenerReasonInvalidRouteKinds)
		resolvedRefsCond.Message = "Listener contains unsupported route kinds"
		entryValid = false
		// When all user-specified kinds are invalid, fall back to protocol defaults
		// for SupportedKinds. This matches the upstream ListenerSet conformance test
		// expectation: SupportedKinds should report what the listener actually supports.
		policy.supportedKinds = supportedKindsForRoutes(defaultListenerKinds(entry.Protocol))
	}
	if entryValid {
		if reason, message, ok := evaluateListenerSpec(entryListener); !ok {
			acceptedCond.Status = metav1.ConditionFalse
			acceptedCond.Reason = reason
			acceptedCond.Message = message
			entryValid = false
		}
	}
	if entryValid && entryListener.TLS != nil && len(entryListener.TLS.CertificateRefs) > 0 {
		reason, message, ok := evaluateListenerSetTLSRefs(entryListener, ls, state)
		if !ok {
			resolvedRefsCond.Status = metav1.ConditionFalse
			resolvedRefsCond.Reason = reason
			resolvedRefsCond.Message = message
			acceptedCond.Status = metav1.ConditionFalse
			acceptedCond.Reason = reason
			acceptedCond.Message = message
			entryValid = false
		}
	}
	if entryValid {
		if reason, message, ok := evaluateListenerConflict(conflictListeners, entryListener); !ok {
			acceptedCond.Status = metav1.ConditionFalse
			acceptedCond.Reason = reason
			acceptedCond.Message = message
			programmedCond.Status = metav1.ConditionFalse
			programmedCond.Reason = reason
			programmedCond.Message = message
			extraConditions = append(extraConditions, metav1.Condition{Type: string(gatewayv1.ListenerConditionConflicted), Status: metav1.ConditionTrue, Reason: reason, Message: message, ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now()})
			entryValid = false
		}
	}
	if !entryValid {
		programmedCond.Status = metav1.ConditionFalse
		if acceptedCond.Status != metav1.ConditionTrue {
			programmedCond.Reason = acceptedCond.Reason
			programmedCond.Message = acceptedCond.Message
		} else {
			programmedCond.Reason = resolvedRefsCond.Reason
			programmedCond.Message = resolvedRefsCond.Message
		}
	}

	conditions := []metav1.Condition{resolvedRefsCond, acceptedCond, programmedCond}
	conditions = append(conditions, extraConditions...)

	return listenerSetEntryEval{
		valid:  entryValid,
		status: gatewayv1.ListenerEntryStatus{Name: entry.Name, SupportedKinds: policy.supportedKinds, Conditions: conditions},
	}
}

func listenerEntryToInternalListener(entry gatewayv1.ListenerEntry, ls gatewayv1.ListenerSet) gatewayv1.Listener {
	return gatewayv1.Listener{
		Name:          gatewayv1.SectionName(ls.Namespace + "/" + ls.Name + "/" + string(entry.Name)),
		Hostname:      entry.Hostname,
		Port:          entry.Port,
		Protocol:      entry.Protocol,
		AllowedRoutes: entry.AllowedRoutes,
		TLS:           entry.TLS,
	}
}

func listenerSetEntryAttachedRoutes(
	gateway gatewayv1.Gateway,
	ls gatewayv1.ListenerSet,
	entry gatewayv1.ListenerEntry,
	attachments map[listenerKey]routeAttachmentSet,
) int32 {
	if len(attachments) == 0 {
		return 0
	}

	listener := listenerEntryToInternalListener(entry, ls)
	key := listenerKey{
		gatewayNamespace: gateway.Namespace,
		gatewayName:      gateway.Name,
		listenerName:     listener.Name,
	}
	return int32(len(attachments[key])) //nolint:gosec // G115: conversion is safe — len is non-negative
}

func gatewayAllowsListenerSet(gw gatewayv1.Gateway, ls gatewayv1.ListenerSet, namespaces map[string]corev1.Namespace) bool {
	if gw.Spec.AllowedListeners == nil || gw.Spec.AllowedListeners.Namespaces == nil {
		return false
	}
	ns := gw.Spec.AllowedListeners.Namespaces
	if ns.From == nil {
		return false
	}
	switch *ns.From {
	case gatewayv1.NamespacesFromAll:
		return true
	case gatewayv1.NamespacesFromSame:
		return ls.Namespace == gw.Namespace
	case gatewayv1.NamespacesFromSelector:
		if ns.Selector == nil {
			return true
		}
		selector, err := metav1.LabelSelectorAsSelector(ns.Selector)
		if err != nil {
			return false
		}
		nsObj, ok := namespaces[ls.Namespace]
		if !ok {
			return false
		}
		return selector.Matches(labels.Set(nsObj.Labels))
	default:
		return false
	}
}

func evaluateGatewayListenerSetListeners(
	state *clusterState,
	gateway gatewayv1.Gateway,
	listenerSets []gatewayv1.ListenerSet,
) []listenerEvaluation {
	if len(listenerSets) == 0 {
		return nil
	}

	gwKey := gateway.Namespace + "/" + gateway.Name
	gwLSes := groupListenerSetsByGateway(listenerSets)[gwKey]
	if len(gwLSes) == 0 {
		return nil
	}

	sortListenerSetsByPrecedence(gwLSes)

	allListeners := make([]gatewayv1.Listener, 0, len(gateway.Spec.Listeners))
	allListeners = append(allListeners, gateway.Spec.Listeners...)

	var out []listenerEvaluation
	for _, ls := range gwLSes {
		if !gatewayAllowsListenerSet(gateway, ls, state.namespaceByName) {
			continue
		}

		acceptedCond := meta.FindStatusCondition(ls.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted))
		if acceptedCond == nil || acceptedCond.Status != metav1.ConditionTrue {
			continue
		}

		for _, entry := range ls.Spec.Listeners {
			entryEval := evaluateListenerSetEntry(entry, ls, allListeners, state)
			if !entryEval.valid {
				continue
			}

			listener := listenerEntryToInternalListener(entry, ls)
			allListeners = append(allListeners, listener)

			eval := evaluateGatewayListener(state, gateway, allListeners, listener, 0)
			out = append(out, eval)
		}
	}
	return out
}

func countAttachedListenerSets(state *clusterState, gateway gatewayv1.Gateway) int32 {
	gwKey := gateway.Namespace + "/" + gateway.Name
	gwLSes := groupListenerSetsByGateway(state.listenerSets)[gwKey]
	if len(gwLSes) == 0 {
		return 0
	}
	sortListenerSetsByPrecedence(gwLSes)

	conflictSet := make([]gatewayv1.Listener, len(gateway.Spec.Listeners))
	copy(conflictSet, gateway.Spec.Listeners)
	var count int32
	for _, ls := range gwLSes {
		if !gatewayAllowsListenerSet(gateway, ls, state.namespaceByName) {
			continue
		}
		lsAccepted := false
		for _, entry := range ls.Spec.Listeners {
			if evaluateListenerSetEntry(entry, ls, conflictSet, state).valid {
				count++
				lsAccepted = true
				break
			}
		}
		if lsAccepted {
			for _, entry := range ls.Spec.Listeners {
				if evaluateListenerSetEntry(entry, ls, conflictSet, state).valid {
					conflictSet = append(conflictSet, listenerEntryToInternalListener(entry, ls))
				}
			}
		}
	}
	return count
}

func sortListenerSetsByPrecedence(listenerSets []gatewayv1.ListenerSet) {
	sort.SliceStable(listenerSets, func(i, j int) bool {
		left := listenerSets[i]
		right := listenerSets[j]
		if !left.CreationTimestamp.Time.Equal(right.CreationTimestamp.Time) {
			return left.CreationTimestamp.Time.Before(right.CreationTimestamp.Time)
		}
		return left.Namespace+"/"+left.Name < right.Namespace+"/"+right.Name
	})
}

func buildDisallowedListenerSetListenerStatuses(ls gatewayv1.ListenerSet) []gatewayv1.ListenerEntryStatus {
	statuses := make([]gatewayv1.ListenerEntryStatus, 0, len(ls.Spec.Listeners))
	for _, l := range ls.Spec.Listeners {
		statuses = append(statuses, gatewayv1.ListenerEntryStatus{
			Name: l.Name,
			Conditions: []metav1.Condition{
				{Type: string(gatewayv1.ListenerConditionResolvedRefs), Status: metav1.ConditionFalse, ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now(), Reason: string(gatewayv1.ListenerReasonInvalid), Message: "Listener from disallowed ListenerSet"},
				{Type: string(gatewayv1.ListenerConditionAccepted), Status: metav1.ConditionFalse, ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now(), Reason: string(gatewayv1.ListenerReasonInvalid), Message: "Listener from disallowed ListenerSet"},
				{Type: string(gatewayv1.ListenerConditionProgrammed), Status: metav1.ConditionFalse, ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now(), Reason: string(gatewayv1.ListenerReasonInvalid), Message: "Listener from disallowed ListenerSet"},
			},
		})
	}
	return statuses
}

func evaluateListenerSetTLSRefs(listener gatewayv1.Listener, ls gatewayv1.ListenerSet, state *clusterState) (reason string, message string, ok bool) {
	for _, certRef := range listener.TLS.CertificateRefs {
		targetNamespace := ls.Namespace
		if certRef.Namespace != nil && string(*certRef.Namespace) != "" {
			targetNamespace = string(*certRef.Namespace)
		}
		if targetNamespace != ls.Namespace {
			certName := certRef.Name
			if !referenceGranted(
				state.referenceGrants,
				targetNamespace,
				gatewayv1beta1.ReferenceGrantFrom{
					Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
					Kind:      gatewayv1beta1.Kind("ListenerSet"),
					Namespace: gatewayv1beta1.Namespace(ls.Namespace),
				},
				gatewayv1beta1.ReferenceGrantTo{
					Group: gatewayv1beta1.Group(""),
					Kind:  gatewayv1beta1.Kind("Secret"),
					Name:  &certName,
				},
			) {
				return string(gatewayv1.ListenerReasonRefNotPermitted), "Cross-namespace CertificateRef is not permitted", false
			}
		}
	}
	return "", "", true
}
