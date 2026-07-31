package status

import (
	"sort"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func collectBackendTLSPolicyAncestors(
	state *clusterState,
	routeState routeState,
) map[string][]gatewayv1.ParentReference {
	index := make(map[string]map[string]gatewayv1.ParentReference)

	for _, route := range state.httpRoutes {
		addBackendPolicyRouteAncestors(
			state,
			index,
			route.Namespace,
			httpRouteBackends(route),
			routeState.http[client.ObjectKeyFromObject(&route)],
		)
	}
	for _, route := range state.grpcRoutes {
		addBackendPolicyRouteAncestors(
			state,
			index,
			route.Namespace,
			grpcRouteBackends(route),
			routeState.grpc[client.ObjectKeyFromObject(&route)],
		)
	}
	for _, route := range state.tcpRoutes {
		addBackendPolicyRouteAncestors(
			state,
			index,
			route.Namespace,
			tcpRouteBackends(route),
			routeState.tcp[client.ObjectKeyFromObject(&route)],
		)
	}
	for _, route := range state.udpRoutes {
		addBackendPolicyRouteAncestors(
			state,
			index,
			route.Namespace,
			udpRouteBackends(route),
			routeState.udp[client.ObjectKeyFromObject(&route)],
		)
	}
	for _, route := range state.tlsRoutes {
		addBackendPolicyRouteAncestors(
			state,
			index,
			route.Namespace,
			tlsRouteBackends(route),
			routeState.tls[client.ObjectKeyFromObject(&route)],
		)
	}

	out := make(map[string][]gatewayv1.ParentReference, len(index))
	for backendKey, ancestors := range index {
		items := make([]gatewayv1.ParentReference, 0, len(ancestors))
		for _, ancestor := range ancestors {
			items = append(items, ancestor)
		}
		sort.Slice(items, func(i, j int) bool {
			return parentStatusKey(items[i], gatewayv1.GatewayController(state.controllerName)) <
				parentStatusKey(items[j], gatewayv1.GatewayController(state.controllerName))
		})
		out[backendKey] = items
	}
	return out
}

func addBackendPolicyRouteAncestors(
	state *clusterState,
	index map[string]map[string]gatewayv1.ParentReference,
	routeNamespace string,
	backends []backendInput,
	evals []routeParentEvaluation,
) {
	ancestors := acceptedGatewayPolicyAncestors(evals)
	if len(ancestors) == 0 {
		return
	}

	for _, backendKey := range backendPolicyKeysForInputs(state, routeNamespace, backends) {
		if _, ok := index[backendKey]; !ok {
			index[backendKey] = make(map[string]gatewayv1.ParentReference)
		}
		for _, ancestor := range ancestors {
			key := parentStatusKey(ancestor, gatewayv1.GatewayController(state.controllerName))
			index[backendKey][key] = ancestor
		}
	}
}

func acceptedGatewayPolicyAncestors(
	evals []routeParentEvaluation,
) []gatewayv1.ParentReference {
	out := make([]gatewayv1.ParentReference, 0, len(evals))
	for _, eval := range evals {
		if eval.acceptedCondition.Status != metav1.ConditionTrue || isServiceParentRef(eval.parentRef) {
			continue
		}
		out = append(out, eval.parentRef)
	}
	return out
}

func backendPolicyKeysForInputs(
	state *clusterState,
	routeNamespace string,
	backends []backendInput,
) []string {
	keys := make([]string, 0, len(backends))
	for _, backend := range backends {
		targetKind, ok := backendKindForStatus(backend.Group, backend.Kind)
		if !ok {
			continue
		}

		targetNamespace := backend.Namespace
		if targetNamespace == "" {
			targetNamespace = routeNamespace
		}

		switch targetKind {
		case "Service":
			service, ok := state.serviceByKey[namespacedName(targetNamespace, backend.Name)]
			if !ok {
				continue
			}
			if backend.Port == 0 {
				for _, port := range service.Spec.Ports {
					keys = append(keys, backendTLSPolicyBackendKey(targetNamespace, backend.Name, port.Port))
				}
				continue
			}
			keys = append(keys, backendTLSPolicyBackendKey(targetNamespace, backend.Name, int32(backend.Port)))
		case "ServiceImport":
			serviceImport, ok := state.serviceImportByKey[namespacedName(targetNamespace, backend.Name)]
			if !ok {
				continue
			}
			if backend.Port == 0 {
				for _, port := range serviceImport.Spec.Ports {
					keys = append(keys, backendTLSPolicyBackendKey(targetNamespace, backend.Name, port.Port))
				}
				continue
			}
			keys = append(keys, backendTLSPolicyBackendKey(targetNamespace, backend.Name, int32(backend.Port)))
		}
	}

	sort.Strings(keys)
	return compactPolicyBackendKeys(keys)
}

func backendTLSPolicyServiceKeys(
	namespace string,
	ports []corev1.ServicePort,
	sectionName *gatewayv1.SectionName,
	name string,
) ([]string, bool) {
	keys := make([]string, 0, len(ports))
	for _, port := range ports {
		if sectionName != nil && port.Name != string(*sectionName) {
			continue
		}
		keys = append(keys, backendTLSPolicyBackendKey(namespace, name, port.Port))
	}
	if sectionName != nil && len(keys) == 0 {
		return nil, false
	}
	return keys, len(keys) > 0
}

func backendTLSPolicyServiceImportKeys(
	namespace string,
	ports []mcsv1alpha1.ServicePort,
	sectionName *gatewayv1.SectionName,
	name string,
) ([]string, bool) {
	keys := make([]string, 0, len(ports))
	for _, port := range ports {
		if sectionName != nil && port.Name != string(*sectionName) {
			continue
		}
		keys = append(keys, backendTLSPolicyBackendKey(namespace, name, port.Port))
	}
	if sectionName != nil && len(keys) == 0 {
		return nil, false
	}
	return keys, len(keys) > 0
}

func policyStatusesForAncestors(
	ancestors []gatewayv1.ParentReference,
	controllerName string,
	conditions ...conditionSpec,
) []gatewayv1.PolicyAncestorStatus {
	out := make([]gatewayv1.PolicyAncestorStatus, 0, len(ancestors))
	for _, ancestor := range ancestors {
		itemConditions := make([]metav1.Condition, 0, len(conditions))
		for _, spec := range conditions {
			setCondition(&itemConditions, spec)
		}
		out = append(out, gatewayv1.PolicyAncestorStatus{
			AncestorRef:    ancestor,
			ControllerName: gatewayv1.GatewayController(controllerName),
			Conditions:     itemConditions,
		})
	}
	return out
}

func policyGatewayAncestors(
	backendKeys []string,
	backendAncestors map[string][]gatewayv1.ParentReference,
) []gatewayv1.ParentReference {
	index := make(map[string]gatewayv1.ParentReference)
	for _, backendKey := range backendKeys {
		for _, ancestor := range backendAncestors[backendKey] {
			index[parentStatusKey(ancestor, "")] = ancestor
		}
	}

	out := make([]gatewayv1.ParentReference, 0, len(index))
	for _, ancestor := range index {
		out = append(out, ancestor)
	}
	sort.Slice(out, func(i, j int) bool {
		return parentStatusKey(out[i], "") < parentStatusKey(out[j], "")
	})
	return out
}

func fallbackPolicyAncestors(
	ancestors []gatewayv1.ParentReference,
) []gatewayv1.ParentReference {
	if len(ancestors) > 0 {
		return ancestors
	}
	return []gatewayv1.ParentReference{{
		Group:     groupPtr(gatewayGroup),
		Kind:      kindPtr("Service"),
		Name:      "unknown",
		Namespace: gatewayNamespacePtr(""),
	}}
}

func rawPolicyFallbackAncestors(
	policy gatewayv1alpha3.BackendTLSPolicy,
) []gatewayv1.ParentReference {
	out := make([]gatewayv1.ParentReference, 0, len(policy.Spec.TargetRefs))
	for _, targetRef := range policy.Spec.TargetRefs {
		out = append(out, policyTargetAncestor(policy.Namespace, targetRef))
	}
	return out
}

func policyTargetAncestor(
	namespace string,
	targetRef gatewayv1.LocalPolicyTargetReferenceWithSectionName,
) gatewayv1.ParentReference {
	ref := gatewayv1.ParentReference{
		Group:     groupPtr(string(targetRef.Group)),
		Kind:      kindPtr(string(targetRef.Kind)),
		Name:      targetRef.Name,
		Namespace: gatewayNamespacePtr(namespace),
	}
	if targetRef.SectionName != nil {
		ref.SectionName = targetRef.SectionName
	}
	return ref
}

func policyConflicted(
	keys []string,
	policyKey client.ObjectKey,
	winners map[string]client.ObjectKey,
) bool {
	for _, key := range keys {
		if winner, ok := winners[key]; ok && winner != policyKey {
			return true
		}
	}
	return false
}

func mergePolicyAncestors(
	existing []gatewayv1.PolicyAncestorStatus,
	desired []gatewayv1.PolicyAncestorStatus,
	controllerName string,
) []gatewayv1.PolicyAncestorStatus {
	controller := gatewayv1.GatewayController(controllerName)
	index := make(map[string]gatewayv1.PolicyAncestorStatus, len(existing))
	for _, item := range existing {
		index[parentStatusKey(item.AncestorRef, item.ControllerName)] = item
	}

	out := make([]gatewayv1.PolicyAncestorStatus, 0, len(existing)+len(desired))
	for _, item := range existing {
		if item.ControllerName == controller {
			continue
		}
		out = append(out, item)
	}
	for _, item := range desired {
		key := parentStatusKey(item.AncestorRef, controller)
		current := index[key]
		current.AncestorRef = item.AncestorRef
		current.ControllerName = controller
		current.Conditions = append([]metav1.Condition(nil), current.Conditions...)
		for _, condition := range item.Conditions {
			setCondition(&current.Conditions, conditionSpec{
				Type:               condition.Type,
				Status:             condition.Status,
				Reason:             condition.Reason,
				Message:            condition.Message,
				ObservedGeneration: condition.ObservedGeneration,
			})
		}
		out = append(out, current)
	}
	sort.Slice(out, func(i, j int) bool {
		return parentStatusKey(out[i].AncestorRef, out[i].ControllerName) <
			parentStatusKey(out[j].AncestorRef, out[j].ControllerName)
	})
	return out
}

func acceptedPolicyCondition(
	generation int64,
	status metav1.ConditionStatus,
	reason string,
	message string,
) conditionSpec {
	return conditionSpec{
		Type:               string(gatewayv1.PolicyConditionAccepted),
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}
}

func invalidAcceptedPolicyCondition(generation int64, message string) conditionSpec {
	return acceptedPolicyCondition(
		generation,
		metav1.ConditionFalse,
		string(gatewayv1.PolicyReasonInvalid),
		message,
	)
}

func noValidCACertificateAcceptedCondition(generation int64) conditionSpec {
	return acceptedPolicyCondition(
		generation,
		metav1.ConditionTrue,
		backendTLSPolicyReasonNoValidCACert,
		"BackendTLSPolicy does not contain any valid CA certificate references",
	)
}

func resolvedPolicyCondition(
	generation int64,
	status metav1.ConditionStatus,
	reason string,
	message string,
) conditionSpec {
	return conditionSpec{
		Type:               backendTLSPolicyConditionResolvedRefs,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}
}

func compactPolicyBackendKeys(items []string) []string {
	if len(items) < 2 {
		return items
	}

	out := items[:1]
	for _, item := range items[1:] {
		if item == out[len(out)-1] {
			continue
		}
		out = append(out, item)
	}
	return out
}

func backendTLSPolicyBackendKey(namespace, name string, port int32) string {
	return namespace + "/" + name + ":" + fmtInt32(port)
}

func backendTLSPolicyClaimKey(
	namespace string,
	targetRef gatewayv1.LocalPolicyTargetReferenceWithSectionName,
) string {
	return namespace +
		"/" + string(targetRef.Group) +
		"/" + string(targetRef.Kind) +
		"/" + string(targetRef.Name) +
		"/" + sectionNameString(targetRef.SectionName)
}

func sectionNameString(sectionName *gatewayv1.SectionName) string {
	if sectionName == nil {
		return ""
	}
	return string(*sectionName)
}

func fmtInt32(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}

func gatewayNamespacePtr(namespace string) *gatewayv1.Namespace {
	value := gatewayv1.Namespace(namespace)
	return &value
}
