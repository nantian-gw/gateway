package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/event"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
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

func TestSnapshotInputMutationPredicateSkipsGatewayLabelUpdates(t *testing.T) {
	predicate := snapshotInputMutationPredicate()
	oldGateway := &gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "gw",
			Namespace:  "default",
			Generation: 1,
		},
	}
	newGateway := oldGateway.DeepCopy()
	newGateway.Labels = map[string]string{"example.com/owner": "platform"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldGateway, ObjectNew: newGateway}) {
		t.Fatal("expected Gateway label-only update to be ignored")
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

func TestSnapshotServiceMutationPredicateSkipsNonFrontendManagedByLabelUpdates(t *testing.T) {
	predicate := snapshotServiceMutationPredicate()
	oldService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	}
	newService := oldService.DeepCopy()
	newService.Labels = map[string]string{resources.ManagedByLabel: resources.ManagedByValue}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected non-frontend managed-by Service label update to be ignored")
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

func TestSnapshotServiceMutationPredicateSkipsPortReordering(t *testing.T) {
	predicate := snapshotServiceMutationPredicate()
	appProtocol := "kubernetes.io/h2c"
	oldService := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "grpc", Port: 9000, Protocol: corev1.ProtocolTCP, AppProtocol: &appProtocol},
				{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	newService := oldService.DeepCopy()
	newService.Spec.Ports[0], newService.Spec.Ports[1] = newService.Spec.Ports[1], newService.Spec.Ports[0]

	if predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected Service port reordering to be ignored")
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

func TestSnapshotPodMutationPredicateSkipsStatusNoise(t *testing.T) {
	predicate := snapshotPodMutationPredicate()
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.10",
		},
	}
	newPod := oldPod.DeepCopy()
	newPod.Status.Conditions = []corev1.PodCondition{{
		Type:   corev1.PodReady,
		Status: corev1.ConditionTrue,
	}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatal("expected Pod condition-only update to be ignored")
	}
}

func TestSnapshotPodMutationPredicateSkipsEquivalentWorkloadPhaseChange(t *testing.T) {
	predicate := snapshotPodMutationPredicate()
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			PodIP: "10.0.0.10",
		},
	}
	newPod := oldPod.DeepCopy()
	newPod.Status.Phase = corev1.PodRunning

	if predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatal("expected Pending-to-Running with the same Pod IP to be ignored")
	}
}

func TestSnapshotPodMutationPredicateAllowsWorkloadIdentityChanges(t *testing.T) {
	predicate := snapshotPodMutationPredicate()
	oldPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.10",
		},
	}

	newPod := oldPod.DeepCopy()
	newPod.Status.PodIP = "10.0.0.11"
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatal("expected Pod IP update to trigger workload snapshot rebuild")
	}

	newPod = oldPod.DeepCopy()
	newPod.Status.Phase = corev1.PodSucceeded
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldPod, ObjectNew: newPod}) {
		t.Fatal("expected Pod phase update to trigger workload snapshot rebuild")
	}
}

func TestSnapshotPodMutationPredicateSkipsCreateWithoutWorkloadIdentity(t *testing.T) {
	predicate := snapshotPodMutationPredicate()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	if predicate.Create(event.CreateEvent{Object: pod}) {
		t.Fatal("expected Pod create without IP to be ignored")
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

func TestSnapshotEndpointSliceMutationPredicateSkipsReadyNilToTrue(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	oldSlice.Endpoints[0].Conditions.Ready = nil
	newSlice := endpointSliceForPredicateTest("10.0.0.10", true)

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected EndpointSlice ready nil-to-true update to be ignored")
	}
}

func TestSnapshotEndpointSliceMutationPredicateSkipsPortReordering(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	grpcPort := int32(9000)
	oldSlice.Ports = append(oldSlice.Ports, discoveryv1.EndpointPort{Name: ptr("grpc"), Port: &grpcPort})
	newSlice := oldSlice.DeepCopy()
	newSlice.Ports[0], newSlice.Ports[1] = newSlice.Ports[1], newSlice.Ports[0]

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected EndpointSlice port reordering to be ignored")
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

func TestSnapshotEndpointSliceMutationPredicateSkipsNonFrontendManagedByLabelUpdates(t *testing.T) {
	predicate := snapshotEndpointSliceMutationPredicate()
	oldSlice := endpointSliceForPredicateTest("10.0.0.10", true)
	newSlice := oldSlice.DeepCopy()
	newSlice.Labels[discoveryv1.LabelManagedBy] = resources.ManagedByValue

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSlice, ObjectNew: newSlice}) {
		t.Fatal("expected non-frontend managed-by EndpointSlice label update to be ignored")
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

func TestSnapshotListenerSetMutationPredicateSkipsLabelUpdates(t *testing.T) {
	predicate := snapshotListenerSetMutationPredicate()
	oldSet := &gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{Name: "ls", Namespace: "default", Generation: 1},
	}
	newSet := oldSet.DeepCopy()
	newSet.Labels = map[string]string{"example.com/owner": "platform"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSet, ObjectNew: newSet}) {
		t.Fatal("expected ListenerSet label-only update to be ignored")
	}
}

func TestSnapshotNamespaceMutationPredicateSkipsMetadataOnlyUpdates(t *testing.T) {
	predicate := snapshotNamespaceMutationPredicate()
	oldNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "apps", Labels: map[string]string{"team": "blue"}},
	}
	newNamespace := oldNamespace.DeepCopy()
	newNamespace.Annotations = map[string]string{"example.com/restarted-at": "now"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldNamespace, ObjectNew: newNamespace}) {
		t.Fatal("expected Namespace annotation-only update to be ignored")
	}
}

func TestSnapshotNamespaceMutationPredicateAllowsLabelUpdates(t *testing.T) {
	predicate := snapshotNamespaceMutationPredicate()
	oldNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "apps", Labels: map[string]string{"team": "blue"}},
	}
	newNamespace := oldNamespace.DeepCopy()
	newNamespace.Labels["team"] = "green"

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldNamespace, ObjectNew: newNamespace}) {
		t.Fatal("expected Namespace label update to trigger attachment rebuild")
	}
}

func TestSnapshotSecretMutationPredicateSkipsMetadataOnlyUpdates(t *testing.T) {
	predicate := snapshotSecretMutationPredicate()
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cert", Namespace: "default"},
		Data:       map[string][]byte{"tls.crt": []byte("cert")},
	}
	newSecret := oldSecret.DeepCopy()
	newSecret.Annotations = map[string]string{"example.com/restarted-at": "now"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: newSecret}) {
		t.Fatal("expected Secret metadata-only update to be ignored")
	}
}

func TestSnapshotSecretMutationPredicateAllowsDataUpdates(t *testing.T) {
	predicate := snapshotSecretMutationPredicate()
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cert", Namespace: "default"},
		Data:       map[string][]byte{"tls.crt": []byte("cert")},
	}
	newSecret := oldSecret.DeepCopy()
	newSecret.Data["tls.crt"] = []byte("new-cert")

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: newSecret}) {
		t.Fatal("expected Secret data update to trigger listener rebuild")
	}
}

func TestSnapshotConfigMapMutationPredicateSkipsMetadataOnlyUpdates(t *testing.T) {
	predicate := snapshotConfigMapMutationPredicate()
	oldConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "filter", Namespace: "default"},
		Data:       map[string]string{"filter.yaml": "type: RequestHeaderModifier"},
	}
	newConfigMap := oldConfigMap.DeepCopy()
	newConfigMap.Labels = map[string]string{"example.com/owner": "ops"}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldConfigMap, ObjectNew: newConfigMap}) {
		t.Fatal("expected ConfigMap metadata-only update to be ignored")
	}
}

func TestSnapshotConfigMapMutationPredicateAllowsDataUpdates(t *testing.T) {
	predicate := snapshotConfigMapMutationPredicate()
	oldConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "filter", Namespace: "default"},
		Data:       map[string]string{"filter.yaml": "type: RequestHeaderModifier"},
		BinaryData: map[string][]byte{"plugin.wasm": []byte("wasm")},
	}

	newConfigMap := oldConfigMap.DeepCopy()
	newConfigMap.Data["filter.yaml"] = "type: CORS"
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldConfigMap, ObjectNew: newConfigMap}) {
		t.Fatal("expected ConfigMap data update to trigger snapshot rebuild")
	}

	newConfigMap = oldConfigMap.DeepCopy()
	newConfigMap.BinaryData["plugin.wasm"] = []byte("new-wasm")
	if !predicate.Update(event.UpdateEvent{ObjectOld: oldConfigMap, ObjectNew: newConfigMap}) {
		t.Fatal("expected ConfigMap binary data update to trigger snapshot rebuild")
	}
}

func TestSnapshotServiceImportMutationPredicateSkipsStatusOnlyUpdates(t *testing.T) {
	predicate := snapshotServiceImportMutationPredicate()
	oldImport := &mcsv1alpha1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default", Generation: 1},
		Spec: mcsv1alpha1.ServiceImportSpec{
			Type:  mcsv1alpha1.ClusterSetIP,
			Ports: []mcsv1alpha1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
		},
	}
	newImport := oldImport.DeepCopy()
	newImport.Generation = 2
	newImport.Status.Clusters = []mcsv1alpha1.ClusterStatus{{Cluster: "cluster-a"}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldImport, ObjectNew: newImport}) {
		t.Fatal("expected ServiceImport status-only update to be ignored")
	}
}

func TestSnapshotServiceImportMutationPredicateAllowsPortChanges(t *testing.T) {
	predicate := snapshotServiceImportMutationPredicate()
	oldImport := &mcsv1alpha1.ServiceImport{
		ObjectMeta: metav1.ObjectMeta{Name: "echo", Namespace: "default", Generation: 1},
		Spec: mcsv1alpha1.ServiceImportSpec{
			Type:  mcsv1alpha1.ClusterSetIP,
			Ports: []mcsv1alpha1.ServicePort{{Name: "http", Port: 80, Protocol: corev1.ProtocolTCP}},
		},
	}
	newImport := oldImport.DeepCopy()
	newImport.Generation = 2
	newImport.Spec.Ports[0].Port = 8080

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldImport, ObjectNew: newImport}) {
		t.Fatal("expected ServiceImport port update to trigger snapshot rebuild")
	}
}

func TestSnapshotBackendLBPolicyMutationPredicateSkipsStatusOnlyUpdates(t *testing.T) {
	predicate := snapshotBackendLBPolicyMutationPredicate()
	oldPolicy := &backend.BackendLBPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "echo-lb", Namespace: "default", Generation: 1},
		Spec: backend.BackendLBPolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{{Kind: "Service", Name: "echo"}},
		},
	}
	newPolicy := oldPolicy.DeepCopy()
	newPolicy.Generation = 2
	newPolicy.Status.Ancestors = []gatewayv1.PolicyAncestorStatus{{
		ControllerName: gatewayv1.GatewayController("gateway.nantian.dev/controller"),
	}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldPolicy, ObjectNew: newPolicy}) {
		t.Fatal("expected BackendLBPolicy status-only update to be ignored")
	}
}

func TestSnapshotRoutePolicyMutationPredicateSkipsStatusOnlyUpdates(t *testing.T) {
	predicate := snapshotRoutePolicyMutationPredicate()
	oldPolicy := &routepolicy.RoutePolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "route-policy", Namespace: "default", Generation: 1},
		Spec: routepolicy.RoutePolicySpec{
			TargetRefs: []gatewayv1.LocalPolicyTargetReference{{Kind: "HTTPRoute", Name: "route"}},
		},
	}
	newPolicy := oldPolicy.DeepCopy()
	newPolicy.Generation = 2
	newPolicy.Status.Conditions = []metav1.Condition{{
		Type:               string(gatewayv1.PolicyConditionAccepted),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 2,
	}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldPolicy, ObjectNew: newPolicy}) {
		t.Fatal("expected RoutePolicy status-only update to be ignored")
	}
}

func TestSnapshotAIServiceMutationPredicateSkipsStatusOnlyUpdates(t *testing.T) {
	predicate := snapshotAIServiceMutationPredicate()
	oldService := &aiservice.AIService{
		ObjectMeta: metav1.ObjectMeta{Name: "openai", Namespace: "default", Generation: 1},
		Spec: aiservice.AIServiceSpec{
			Provider: "openai",
			Model:    "gpt-4o",
		},
	}
	newService := oldService.DeepCopy()
	newService.Generation = 2
	newService.Status.Conditions = []metav1.Condition{{
		Type:               "Accepted",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 2,
	}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldService, ObjectNew: newService}) {
		t.Fatal("expected AIService status-only update to be ignored")
	}
}

func TestSnapshotTokenPolicyMutationPredicateAllowsSpecChanges(t *testing.T) {
	predicate := snapshotTokenPolicyMutationPredicate()
	oldPolicy := &tokenpolicy.TokenPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "token-limit", Namespace: "default", Generation: 1},
		Spec: tokenpolicy.TokenPolicySpec{
			TargetRefs:      []gatewayv1.LocalPolicyTargetReference{{Kind: "Service", Name: "echo"}},
			TokensPerMinute: 100,
		},
	}
	newPolicy := oldPolicy.DeepCopy()
	newPolicy.Generation = 2
	newPolicy.Spec.TokensPerMinute = 200

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldPolicy, ObjectNew: newPolicy}) {
		t.Fatal("expected TokenPolicy spec update to trigger snapshot rebuild")
	}
}

func TestSnapshotWasmPluginMutationPredicateSkipsStatusOnlyUpdates(t *testing.T) {
	predicate := snapshotWasmPluginMutationPredicate()
	oldPlugin := &wasmplugin.WasmPlugin{
		ObjectMeta: metav1.ObjectMeta{Name: "authz", Namespace: "default", Generation: 1},
		Spec: wasmplugin.WasmPluginSpec{
			TargetRefs: []wasmplugin.WasmPluginTargetRef{{Group: "", Kind: "Service", Name: "echo"}},
			Wasm:       wasmplugin.WasmSource{SHA256: "abc123"},
		},
	}
	newPlugin := oldPlugin.DeepCopy()
	newPlugin.Generation = 2
	newPlugin.Status.Conditions = []metav1.Condition{{
		Type:               "Accepted",
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 2,
	}}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldPlugin, ObjectNew: newPlugin}) {
		t.Fatal("expected WasmPlugin status-only update to be ignored")
	}
}

func TestSnapshotBackendTLSPolicyMutationPredicateSkipsStatusOnlyUpdates(t *testing.T) {
	predicate := snapshotBackendTLSPolicyMutationPredicate()
	oldPolicy := backendTLSPolicyForPredicateTest("echo.default.svc")
	newPolicy := oldPolicy.DeepCopy()
	newPolicy.SetGeneration(2)
	if err := unstructured.SetNestedField(
		newPolicy.Object,
		[]any{map[string]any{"controllerName": "gateway.nantian.dev/controller"}},
		"status",
		"ancestors",
	); err != nil {
		t.Fatalf("set BackendTLSPolicy status: %v", err)
	}

	if predicate.Update(event.UpdateEvent{ObjectOld: oldPolicy, ObjectNew: newPolicy}) {
		t.Fatal("expected BackendTLSPolicy status-only update to be ignored")
	}
}

func TestSnapshotBackendTLSPolicyMutationPredicateAllowsSpecChanges(t *testing.T) {
	predicate := snapshotBackendTLSPolicyMutationPredicate()
	oldPolicy := backendTLSPolicyForPredicateTest("echo.default.svc")
	newPolicy := oldPolicy.DeepCopy()
	newPolicy.SetGeneration(2)
	if err := unstructured.SetNestedField(
		newPolicy.Object,
		"orders.default.svc",
		"spec",
		"validation",
		"hostname",
	); err != nil {
		t.Fatalf("set BackendTLSPolicy validation hostname: %v", err)
	}

	if !predicate.Update(event.UpdateEvent{ObjectOld: oldPolicy, ObjectNew: newPolicy}) {
		t.Fatal("expected BackendTLSPolicy spec update to trigger snapshot rebuild")
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

func backendTLSPolicyForPredicateTest(hostname string) *unstructured.Unstructured {
	item := gatewayapi.NewBackendTLSPolicyV1Object()
	item.SetName("echo-tls")
	item.SetNamespace("default")
	item.SetGeneration(1)
	item.SetCreationTimestamp(metav1.Unix(100, 0))
	item.Object["spec"] = map[string]any{
		"targetRefs": []any{
			map[string]any{
				"group": "",
				"kind":  "Service",
				"name":  "echo",
			},
		},
		"validation": map[string]any{
			"hostname":                hostname,
			"wellKnownCACertificates": "System",
		},
	}
	return item
}
