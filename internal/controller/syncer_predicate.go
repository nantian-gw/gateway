package controller

import (
	"strings"

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
	)
}

func snapshotRelevantAnnotationChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event.CreateEvent) bool { return true },
		DeleteFunc: func(event.DeleteEvent) bool { return true },
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
