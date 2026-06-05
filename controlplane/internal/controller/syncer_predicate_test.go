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
	newRoute.Status.RouteStatus.Parents = []gatewayv1.RouteParentStatus{{
		ControllerName: "gateway.networking.k8s.io/aether-gateway",
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
		t.Fatal("expected annotation-only HTTPRoute update to trigger rebuild")
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
