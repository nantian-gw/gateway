package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

func TestSnapshotInputMutationPredicateSkipsStatusOnlyHTTPRouteUpdates(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "route",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newRoute := oldRoute.DeepCopy()
	newRoute.Status.Parents = []gatewayv1.RouteParentStatus{{
		ControllerName: "gateway.networking.k8s.io/nantian-gw",
	}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldRoute, ObjectNew: newRoute}) {
		t.Fatal("expected status-only HTTPRoute update to be ignored")
	}
}

func TestSnapshotInputMutationPredicateAllowsHTTPRouteAnnotationUpdates(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "route",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newRoute := oldRoute.DeepCopy()
	newRoute.Annotations = map[string]string{
		"gateway.nantian.dev/access-log-mode": "json",
	}

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldRoute, ObjectNew: newRoute}) {
		t.Fatal("expected relevant annotation-only HTTPRoute update to trigger rebuild")
	}
}

func TestSnapshotInputMutationPredicateSkipsIrrelevantHTTPRouteAnnotationUpdates(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "route",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newRoute := oldRoute.DeepCopy()
	newRoute.Annotations = map[string]string{
		"example.com/trace": "enabled",
	}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldRoute, ObjectNew: newRoute}) {
		t.Fatal("expected irrelevant annotation-only HTTPRoute update to be ignored")
	}
}

func TestSnapshotInputMutationPredicateAllowsRelevantTLSRouteAnnotationUpdates(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldRoute := &gatewayv1alpha2.TLSRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "route",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newRoute := oldRoute.DeepCopy()
	newRoute.Annotations = map[string]string{
		"gateway.nantian.dev/access-log-path": "/var/log/tls.log",
	}

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldRoute, ObjectNew: newRoute}) {
		t.Fatal("expected relevant TLSRoute annotation-only update to trigger rebuild")
	}
}

func TestSnapshotInputMutationPredicateAllowsHTTPRouteLabelUpdates(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "route",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newRoute := oldRoute.DeepCopy()
	newRoute.Labels = map[string]string{
		"gateway.networking.k8s.io/test": "true",
	}

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldRoute, ObjectNew: newRoute}) {
		t.Fatal("expected label-only HTTPRoute update to trigger rebuild")
	}
}

func TestSnapshotInputMutationPredicateAllowsGatewayGenerationChanges(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newGateway := oldGateway.DeepCopy()
	newGateway.Generation = 2

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldGateway, ObjectNew: newGateway}) {
		t.Fatal("expected generation-changing Gateway update to trigger rebuild")
	}
}

func TestSnapshotInputMutationPredicateSkipsIrrelevantGatewayAnnotationUpdates(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newGateway := oldGateway.DeepCopy()
	newGateway.Annotations = map[string]string{
		"example.com/trace": "enabled",
	}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldGateway, ObjectNew: newGateway}) {
		t.Fatal("expected irrelevant Gateway annotation-only update to be ignored")
	}
}

func TestSnapshotInputMutationPredicateAllowsLifecycleEvents(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	route := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "route",
			Namespace: "default",
		},
	}

	if !predicate.Create(event.CreateEvent{Object: route}) {
		t.Fatal("expected create event to trigger rebuild")
	}
	if !predicate.Delete(event.DeleteEvent{Object: route}) {
		t.Fatal("expected delete event to trigger rebuild")
	}
	if !predicate.Generic(event.GenericEvent{Object: route}) {
		t.Fatal("expected generic event to trigger rebuild")
	}
}

func TestSnapshotListenerSetMutationPredicateAllowsAcceptedStatusUpdates(t *testing.T) {
	predicate := snapshotListenerSetMutationPredicate()
	oldSet := &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
		Status: gatewayv1.ListenerSetStatus{
			Conditions: []metav1.Condition{{
				Type:               string(gatewayv1.ListenerSetConditionAccepted),
				Status:             metav1.ConditionFalse,
				ObservedGeneration: 1,
			}},
		},
	}
	newSet := oldSet.DeepCopy()
	newSet.Status.Conditions[0].Status = metav1.ConditionTrue

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldSet, ObjectNew: newSet}) {
		t.Fatal("expected ListenerSet Accepted status update to trigger snapshot rebuild")
	}
}

func TestSnapshotListenerSetMutationPredicateAllowsAcceptedObservedGenerationUpdates(t *testing.T) {
	predicate := snapshotListenerSetMutationPredicate()
	oldSet := &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 2},
		Status: gatewayv1.ListenerSetStatus{
			Conditions: []metav1.Condition{{
				Type:               string(gatewayv1.ListenerSetConditionAccepted),
				Status:             metav1.ConditionTrue,
				ObservedGeneration: 1,
			}},
		},
	}
	newSet := oldSet.DeepCopy()
	newSet.Status.Conditions[0].ObservedGeneration = 2

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldSet, ObjectNew: newSet}) {
		t.Fatal("expected ListenerSet Accepted observedGeneration update to trigger snapshot rebuild")
	}
}

func TestSnapshotListenerSetMutationPredicateAllowsListenerAcceptedStatusUpdates(t *testing.T) {
	predicate := snapshotListenerSetMutationPredicate()
	oldSet := &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
		Status: gatewayv1.ListenerSetStatus{
			Listeners: []gatewayv1.ListenerEntryStatus{{
				Name: "http",
				Conditions: []metav1.Condition{{
					Type:               string(gatewayv1.ListenerConditionAccepted),
					Status:             metav1.ConditionFalse,
					ObservedGeneration: 1,
				}},
			}},
		},
	}
	newSet := oldSet.DeepCopy()
	newSet.Status.Listeners[0].Conditions[0].Status = metav1.ConditionTrue

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldSet, ObjectNew: newSet}) {
		t.Fatal("expected ListenerSet listener Accepted status update to trigger snapshot rebuild")
	}
}

func TestSnapshotListenerSetMutationPredicateSkipsAttachedRouteStatusUpdates(t *testing.T) {
	predicate := snapshotListenerSetMutationPredicate()
	oldSet := &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
		Status: gatewayv1.ListenerSetStatus{
			Listeners: []gatewayv1.ListenerEntryStatus{{
				Name:           "http",
				AttachedRoutes: 1,
			}},
		},
	}
	newSet := oldSet.DeepCopy()
	newSet.Status.Listeners[0].AttachedRoutes = 2

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSet, ObjectNew: newSet}) {
		t.Fatal("expected attached-route-only ListenerSet status update to be ignored")
	}
}
