package status

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
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
) map[string]listenerSetEvaluation {
	out := make(map[string]listenerSetEvaluation, len(lses))
	gwToLSes := groupListenerSetsByGateway(lses)

	for gwKey, gwLSes := range gwToLSes {
		gw, ok := managedGateways[gwKey]
		if !ok {
			continue
		}
		sort.Slice(gwLSes, func(i, j int) bool {
			return gwLSes[i].CreationTimestamp.Before(&gwLSes[j].CreationTimestamp)
		})

		conflictSet := make([]gatewayv1.Listener, len(gw.Spec.Listeners))
		copy(conflictSet, gw.Spec.Listeners)

		for _, ls := range gwLSes {
			eval := evaluateOneListenerSet(ls, gw, conflictSet, state)
			key := ls.Namespace + "/" + ls.Name
			out[key] = eval

			if eval.accepted.Status == metav1.ConditionTrue {
				for _, entry := range ls.Spec.Listeners {
					conflictSet = append(conflictSet, listenerEntryToInternalListener(entry, ls))
				}
			}
		}
	}
	return out
}

func groupListenerSetsByGateway(lses []gatewayv1.ListenerSet) map[string][]gatewayv1.ListenerSet {
	out := make(map[string][]gatewayv1.ListenerSet)
	for _, ls := range lses {
		ref := ls.Spec.ParentRef
		ns := ""
		if ref.Namespace != nil {
			ns = string(*ref.Namespace)
		}
		gwKey := ns + "/" + string(ref.Name)
		if gwKey == "/" {
			continue
		}
		out[gwKey] = append(out[gwKey], ls)
	}
	return out
}

func evaluateOneListenerSet(
	ls gatewayv1.ListenerSet,
	gw gatewayv1.Gateway,
	conflictListeners []gatewayv1.Listener,
	state *clusterState,
) listenerSetEvaluation {
	allowed := gatewayAllowsListenerSet(gw, ls, state.namespaceByName)
	if !allowed {
		return disallowedListenerSetEvaluation(ls)
	}

	listenerStatuses := make([]gatewayv1.ListenerEntryStatus, 0, len(ls.Spec.Listeners))
	hasValidListener := len(ls.Spec.Listeners) == 0

	for _, entry := range ls.Spec.Listeners {
		entryEval := evaluateListenerSetEntry(entry, ls, conflictListeners, state)
		if entryEval.accepted {
			hasValidListener = true
		}
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
	accepted bool
	status   gatewayv1.ListenerEntryStatus
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
	entryAccepted := true

	if policy.invalidKindRefs {
		resolvedRefsCond.Status = metav1.ConditionFalse
		resolvedRefsCond.Reason = string(gatewayv1.ListenerReasonInvalidRouteKinds)
		resolvedRefsCond.Message = "Listener contains unsupported route kinds"
		entryAccepted = false
	}
	if entryAccepted {
		if reason, message, ok := evaluateListenerSpec(entryListener); !ok {
			acceptedCond.Status = metav1.ConditionFalse
			acceptedCond.Reason = reason
			acceptedCond.Message = message
			entryAccepted = false
		}
	}
	if entryAccepted && entryListener.TLS != nil && len(entryListener.TLS.CertificateRefs) > 0 {
		reason, message, ok := evaluateListenerSetTLSRefs(entryListener, ls, state)
		if !ok {
			resolvedRefsCond.Status = metav1.ConditionFalse
			resolvedRefsCond.Reason = reason
			resolvedRefsCond.Message = message
		}
	}
	if entryAccepted {
		if reason, message, ok := evaluateListenerConflict(conflictListeners, entryListener); !ok {
			acceptedCond.Status = metav1.ConditionFalse
			acceptedCond.Reason = reason
			acceptedCond.Message = message
			programmedCond.Status = metav1.ConditionFalse
			programmedCond.Reason = reason
			programmedCond.Message = message
			extraConditions = append(extraConditions, metav1.Condition{Type: string(gatewayv1.ListenerConditionConflicted), Status: metav1.ConditionTrue, Reason: reason, Message: message, ObservedGeneration: ls.Generation, LastTransitionTime: metav1.Now()})
			entryAccepted = false
		}
	}
	if !entryAccepted {
		programmedCond.Status = metav1.ConditionFalse
		programmedCond.Reason = acceptedCond.Reason
		programmedCond.Message = acceptedCond.Message
	}

	conditions := []metav1.Condition{resolvedRefsCond, acceptedCond, programmedCond}
	conditions = append(conditions, extraConditions...)

	return listenerSetEntryEval{
		accepted: entryAccepted,
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

func gatewayAllowsListenerSet(gw gatewayv1.Gateway, ls gatewayv1.ListenerSet, namespaces map[string]corev1.Namespace) bool {
	if gw.Spec.AllowedListeners == nil || gw.Spec.AllowedListeners.Namespaces == nil { return false }
	ns := gw.Spec.AllowedListeners.Namespaces
	if ns.From == nil { return false }
	switch *ns.From {
	case gatewayv1.NamespacesFromAll: return true
	case gatewayv1.NamespacesFromSame: return ls.Namespace == gw.Namespace
	case gatewayv1.NamespacesFromSelector:
		if ns.Selector == nil { return true }
		selector, err := metav1.LabelSelectorAsSelector(ns.Selector)
		if err != nil { return false }
		nsObj, ok := namespaces[ls.Namespace]
		if !ok { return false }
		return selector.Matches(labels.Set(nsObj.Labels))
	default: return false
	}
}

func countAttachedListenerSets(state *clusterState, gateway gatewayv1.Gateway) int32 {
	gwKey := gateway.Namespace + "/" + gateway.Name
	gwLSes := groupListenerSetsByGateway(state.listenerSets)[gwKey]
	if len(gwLSes) == 0 { return 0 }
	sort.Slice(gwLSes, func(i, j int) bool { return gwLSes[i].CreationTimestamp.Before(&gwLSes[j].CreationTimestamp) })

	conflictSet := make([]gatewayv1.Listener, len(gateway.Spec.Listeners))
	copy(conflictSet, gateway.Spec.Listeners)
	var count int32
	for _, ls := range gwLSes {
		if !gatewayAllowsListenerSet(gateway, ls, state.namespaceByName) { continue }
		lsAccepted := false
		for _, entry := range ls.Spec.Listeners {
			if evaluateListenerSetEntry(entry, ls, conflictSet, state).accepted { count++; lsAccepted = true; break }
		}
		if lsAccepted {
			for _, entry := range ls.Spec.Listeners {
				if evaluateListenerSetEntry(entry, ls, conflictSet, state).accepted {
					conflictSet = append(conflictSet, listenerEntryToInternalListener(entry, ls))
				}
			}
		}
	}
	return count
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
			certName := gatewayv1beta1.ObjectName(certRef.Name)
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
