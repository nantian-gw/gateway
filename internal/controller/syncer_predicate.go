package controller

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/resources"
)

const snapshotRelevantAnnotationPrefix = "gateway.nantian.dev/"

func snapshotInputMutationPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		snapshotRelevantAnnotationChangedPredicate(),
		predicate.LabelChangedPredicate{},
		snapshotLifecycleEventPredicate(),
	)
}

func snapshotServiceMutationPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return resources.ShouldAffectSnapshot(createEvent.Object)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return resources.ShouldAffectSnapshot(deleteEvent.Object)
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return resources.ShouldAffectSnapshot(genericEvent.Object)
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldAffects := resources.ShouldAffectSnapshot(updateEvent.ObjectOld)
			newAffects := resources.ShouldAffectSnapshot(updateEvent.ObjectNew)
			if !oldAffects && !newAffects {
				return false
			}

			oldService, oldOK := updateEvent.ObjectOld.(*corev1.Service)
			newService, newOK := updateEvent.ObjectNew.(*corev1.Service)
			if !oldOK || !newOK {
				return true
			}

			return !apiequality.Semantic.DeepEqual(
				serviceSnapshotInputValue(oldService),
				serviceSnapshotInputValue(newService),
			)
		},
	}
}

type serviceSnapshotInput struct {
	Ports       []corev1.ServicePort
	Annotations map[string]string
	Labels      map[string]string
}

func serviceSnapshotInputValue(service *corev1.Service) serviceSnapshotInput {
	if service == nil {
		return serviceSnapshotInput{}
	}
	return serviceSnapshotInput{
		Ports: service.Spec.Ports,
		Annotations: filterStringMapKeys(service.Annotations, []string{
			mesh.ManagedServiceAnnotation,
			mesh.ShadowServiceAnnotation,
		}),
		Labels: filterStringMapKeys(service.Labels, []string{
			resources.ManagedByLabel,
			resources.ServiceRoleKey,
			mesh.OriginalServiceNamespaceLabel,
			mesh.OriginalServiceNameLabel,
		}),
	}
}

func snapshotPodMutationPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			pod, ok := createEvent.Object.(*corev1.Pod)
			return ok && podSnapshotInputValue(pod).HasWorkloadIdentity()
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			pod, ok := deleteEvent.Object.(*corev1.Pod)
			return ok && podSnapshotInputValue(pod).HasWorkloadIdentity()
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			pod, ok := genericEvent.Object.(*corev1.Pod)
			return ok && podSnapshotInputValue(pod).HasWorkloadIdentity()
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldPod, oldOK := updateEvent.ObjectOld.(*corev1.Pod)
			newPod, newOK := updateEvent.ObjectNew.(*corev1.Pod)
			if !oldOK || !newOK {
				return true
			}

			return podSnapshotInputValue(oldPod) != podSnapshotInputValue(newPod)
		},
	}
}

type podSnapshotInput struct {
	Namespace string
	Name      string
	PodIP     string
	Phase     corev1.PodPhase
}

func podSnapshotInputValue(pod *corev1.Pod) podSnapshotInput {
	if pod == nil {
		return podSnapshotInput{}
	}
	return podSnapshotInput{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		PodIP:     pod.Status.PodIP,
		Phase:     pod.Status.Phase,
	}
}

func (v podSnapshotInput) HasWorkloadIdentity() bool {
	if v.Namespace == "" || v.Name == "" || v.PodIP == "" {
		return false
	}
	return v.Phase == corev1.PodRunning || v.Phase == corev1.PodPending
}

func snapshotEndpointSliceMutationPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return resources.ShouldAffectSnapshot(createEvent.Object)
		},
		DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
			return resources.ShouldAffectSnapshot(deleteEvent.Object)
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return resources.ShouldAffectSnapshot(genericEvent.Object)
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldAffects := resources.ShouldAffectSnapshot(updateEvent.ObjectOld)
			newAffects := resources.ShouldAffectSnapshot(updateEvent.ObjectNew)
			if !oldAffects && !newAffects {
				return false
			}

			oldSlice, oldOK := updateEvent.ObjectOld.(*discoveryv1.EndpointSlice)
			newSlice, newOK := updateEvent.ObjectNew.(*discoveryv1.EndpointSlice)
			if !oldOK || !newOK {
				return true
			}

			return !apiequality.Semantic.DeepEqual(
				endpointSliceSnapshotInputValue(oldSlice),
				endpointSliceSnapshotInputValue(newSlice),
			)
		},
	}
}

type endpointSliceSnapshotInput struct {
	AddressType discoveryv1.AddressType
	Labels      map[string]string
	Ports       []discoveryv1.EndpointPort
	Endpoints   []endpointSnapshotInput
}

type endpointSnapshotInput struct {
	Addresses []string
	Ready     string
	Zone      string
}

func endpointSliceSnapshotInputValue(endpointSlice *discoveryv1.EndpointSlice) endpointSliceSnapshotInput {
	if endpointSlice == nil {
		return endpointSliceSnapshotInput{}
	}

	endpoints := make([]endpointSnapshotInput, 0, len(endpointSlice.Endpoints))
	for _, endpoint := range endpointSlice.Endpoints {
		addresses := append([]string(nil), endpoint.Addresses...)
		sort.Strings(addresses)

		ready := "<nil>"
		if endpoint.Conditions.Ready != nil {
			ready = strconv.FormatBool(*endpoint.Conditions.Ready)
		}

		zone := ""
		if endpoint.Zone != nil {
			zone = *endpoint.Zone
		}

		endpoints = append(endpoints, endpointSnapshotInput{
			Addresses: addresses,
			Ready:     ready,
			Zone:      zone,
		})
	}
	sort.Slice(endpoints, func(i, j int) bool {
		left := strings.Join(endpoints[i].Addresses, ",") + "\x00" + endpoints[i].Ready + "\x00" + endpoints[i].Zone
		right := strings.Join(endpoints[j].Addresses, ",") + "\x00" + endpoints[j].Ready + "\x00" + endpoints[j].Zone
		return left < right
	})

	return endpointSliceSnapshotInput{
		AddressType: endpointSlice.AddressType,
		Labels: filterStringMapKeys(endpointSlice.Labels, []string{
			discoveryv1.LabelServiceName,
			discoveryv1.LabelManagedBy,
			mcsv1alpha1.LabelServiceName,
			resources.ServiceRoleKey,
		}),
		Ports:     endpointSlice.Ports,
		Endpoints: endpoints,
	}
}

func snapshotNamespaceMutationPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldNamespace, oldOK := updateEvent.ObjectOld.(*corev1.Namespace)
			newNamespace, newOK := updateEvent.ObjectNew.(*corev1.Namespace)
			if !oldOK || !newOK {
				return true
			}
			return !stringMapsEqual(oldNamespace.Labels, newNamespace.Labels)
		},
	}
}

func snapshotSecretMutationPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldSecret, oldOK := updateEvent.ObjectOld.(*corev1.Secret)
			newSecret, newOK := updateEvent.ObjectNew.(*corev1.Secret)
			if !oldOK || !newOK {
				return true
			}
			return !apiequality.Semantic.DeepEqual(oldSecret.Data, newSecret.Data)
		},
	}
}

func snapshotConfigMapMutationPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldConfigMap, oldOK := updateEvent.ObjectOld.(*corev1.ConfigMap)
			newConfigMap, newOK := updateEvent.ObjectNew.(*corev1.ConfigMap)
			if !oldOK || !newOK {
				return true
			}
			return !apiequality.Semantic.DeepEqual(oldConfigMap.Data, newConfigMap.Data) ||
				!apiequality.Semantic.DeepEqual(oldConfigMap.BinaryData, newConfigMap.BinaryData)
		},
	}
}

func snapshotLifecycleEventPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc:  func(event.UpdateEvent) bool { return false },
	}
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

func filterStringMapKeys(values map[string]string, keys []string) map[string]string {
	if len(values) == 0 || len(keys) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, key := range keys {
		if value, ok := values[key]; ok {
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
