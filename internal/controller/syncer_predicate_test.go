package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/resources"
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

func TestSnapshotServiceMutationPredicateSkipsStatusOnlyServiceUpdates(t *testing.T) {
	predicate := snapshotServiceMutationPredicate()
	oldService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
	newService := oldService.DeepCopy()
	newService.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected status-only Service update to be ignored")
	}
}

func TestSnapshotServiceMutationPredicateSkipsIrrelevantServiceMetadataUpdates(t *testing.T) {
	predicate := snapshotServiceMutationPredicate()
	oldService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
	newService := oldService.DeepCopy()
	newService.Annotations = map[string]string{"example.com/restarted-at": "now"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected irrelevant Service metadata update to be ignored")
	}
}

func TestSnapshotServiceMutationPredicateAllowsPortChanges(t *testing.T) {
	predicate := snapshotServiceMutationPredicate()
	oldService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
	newService := oldService.DeepCopy()
	newService.Spec.Ports[0].Port = 8080

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected Service port update to trigger snapshot rebuild")
	}
}

func TestSnapshotServiceMutationPredicateAllowsMeshMetadataChanges(t *testing.T) {
	predicate := snapshotServiceMutationPredicate()
	oldService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
	newService := oldService.DeepCopy()
	newService.Annotations = map[string]string{
		mesh.ManagedServiceAnnotation: "true",
		mesh.ShadowServiceAnnotation:  "nantian-gw-shadow-echo",
	}

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected mesh Service metadata update to trigger snapshot rebuild")
	}
}

func TestSnapshotServiceMutationPredicateAllowsTransitionToManagedFrontend(t *testing.T) {
	predicate := snapshotServiceMutationPredicate()
	oldService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
	newService := oldService.DeepCopy()
	newService.Labels = map[string]string{
		resources.ManagedByLabel: resources.ManagedByValue,
		resources.ServiceRoleKey: resources.ServiceRoleShared,
	}

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected transition from user Service to managed frontend Service to trigger snapshot rebuild")
	}
}

func TestSnapshotEndpointSliceMutationPredicateSkipsIrrelevantMetadataUpdates(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	newSlice := oldSlice.DeepCopy()
	newSlice.Annotations = map[string]string{"example.com/restarted-at": "now"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected irrelevant EndpointSlice metadata update to be ignored")
	}
}

func TestSnapshotEndpointSliceMutationPredicateSkipsEndpointReordering(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	oldSlice.Endpoints = append(oldSlice.Endpoints, discoveryv1.Endpoint{Addresses: []string{"10.0.0.11"}})
	newSlice := oldSlice.DeepCopy()
	newSlice.Endpoints[0], newSlice.Endpoints[1] = newSlice.Endpoints[1], newSlice.Endpoints[0]

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected EndpointSlice endpoint reordering to be ignored")
	}
}

func TestSnapshotEndpointSliceMutationPredicateAllowsEndpointReadinessChanges(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	newSlice := endpointSliceForPredicateTest("10.0.0.10", false)

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected EndpointSlice readiness update to trigger snapshot rebuild")
	}
}

func TestSnapshotEndpointSliceMutationPredicateAllowsServiceLabelChanges(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	newSlice := oldSlice.DeepCopy()
	newSlice.Labels[discoveryv1.LabelServiceName] = "orders"

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected EndpointSlice service-name label update to trigger snapshot rebuild")
	}
}

func TestSnapshotEndpointSliceMutationPredicateIgnoresManagedFrontendEndpointSlice(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	oldSlice.Labels = map[string]string{
		discoveryv1.LabelManagedBy: resources.ManagedByValue,
		resources.ServiceRoleKey:   resources.EndpointSliceRoleGatewayFrontend,
	}
	newSlice := oldSlice.DeepCopy()
	newSlice.Endpoints[0].Addresses = []string{"10.0.0.11"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected managed frontend EndpointSlice update to be ignored")
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

func endpointSliceForPredicateTest(address string, ready bool) *discoveryv1.EndpointSlice {
	port := int32(8080)
	return &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "echo-1",
			Namespace: "default",
			Labels: map[string]string{
				discoveryv1.LabelServiceName: "echo",
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Ports:       []discoveryv1.EndpointPort{{Name: ptr("http"), Port: &port}},
		Endpoints: []discoveryv1.Endpoint{{
			Addresses:  []string{address},
			Conditions: discoveryv1.EndpointConditions{Ready: &ready},
		}},
	}
}
