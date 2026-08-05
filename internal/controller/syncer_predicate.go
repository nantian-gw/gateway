package controller

import (
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const snapshotRelevantAnnotationPrefix = "gateway.nantian.dev/"

func snapshotInputMutationPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		snapshotRelevantAnnotationChangedPredicate(),
		predicate.LabelChangedPredicate{},
		predicate.Funcs{
			// Also trigger on any resource version change (e.g. status updates on GatewayClass),
			// not just generation changes. This ensures the initial full build is triggered
			// even when the GatewayClass was created before the controller-runtime watch started.
			CreateFunc: func(event.CreateEvent) bool { return true },
			UpdateFunc: func(event.UpdateEvent) bool { return true },
			DeleteFunc: func(event.DeleteEvent) bool { return true },
		},
	)
}

func snapshotListenerSetMutationPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		predicate.LabelChangedPredicate{},
		listenerSetAcceptedStatusChangedPredicate(),
	)
}

func listenerSetAcceptedStatusChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldSet, ok := updateEvent.ObjectOld.(*gatewayv1.ListenerSet)
			if !ok {
				return false
			}
			newSet, ok := updateEvent.ObjectNew.(*gatewayv1.ListenerSet)
			if !ok {
				return false
			}
			return listenerSetAcceptedSnapshotValue(oldSet) != listenerSetAcceptedSnapshotValue(newSet)
		},
	}
}

type acceptedConditionSnapshotValue struct {
	status             string
	observedGeneration int64
}

func listenerSetAcceptedSnapshotValue(listenerSet *gatewayv1.ListenerSet) string {
	if listenerSet == nil {
		return ""
	}

	parts := make([]string, 0, 1+len(listenerSet.Status.Listeners))
	parts = append(parts, acceptedConditionValue(listenerSet.Status.Conditions, string(gatewayv1.ListenerSetConditionAccepted)).String())
	for _, listener := range listenerSet.Status.Listeners {
		parts = append(parts, string(listener.Name)+"="+acceptedConditionValue(listener.Conditions, string(gatewayv1.ListenerConditionAccepted)).String())
	}
	return strings.Join(parts, "|")
}

func acceptedConditionValue(conditions []metav1.Condition, conditionType string) acceptedConditionSnapshotValue {
	for _, condition := range conditions {
		if condition.Type != conditionType {
			continue
		}
		return acceptedConditionSnapshotValue{
			status:             string(condition.Status),
			observedGeneration: condition.ObservedGeneration,
		}
	}
	return acceptedConditionSnapshotValue{status: "<missing>"}
}

func (v acceptedConditionSnapshotValue) String() string {
	return v.status + "@" + strconv.FormatInt(v.observedGeneration, 10)
}

func snapshotRelevantAnnotationChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return relevantSnapshotAnnotationsChanged(updateEvent.ObjectOld, updateEvent.ObjectNew)
		},
	}
}

func relevantSnapshotAnnotationsChanged(oldObject, newObject client.Object) bool {
	if !supportsRelevantSnapshotAnnotations(oldObject) || !supportsRelevantSnapshotAnnotations(newObject) {
		return false
	}

	return !stringMapsEqual(
		filterRelevantSnapshotAnnotations(oldObject.GetAnnotations()),
		filterRelevantSnapshotAnnotations(newObject.GetAnnotations()),
	)
}

func supportsRelevantSnapshotAnnotations(object client.Object) bool {
	switch object.(type) {
	case *gatewayv1.HTTPRoute,
		*gatewayv1.GRPCRoute,
		*gatewayv1alpha2.TCPRoute,
		*gatewayv1alpha2.UDPRoute,
		*gatewayv1alpha2.TLSRoute:
		return true
	default:
		return false
	}
}

func filterRelevantSnapshotAnnotations(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[string]string)
	for key, value := range values {
		if strings.HasPrefix(key, snapshotRelevantAnnotationPrefix) {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringMapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
