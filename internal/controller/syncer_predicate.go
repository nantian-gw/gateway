package controller

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/constants"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	routepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/resources"
)

func snapshotInputMutationPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		snapshotRelevantAnnotationChangedPredicate(),
		snapshotRouteLabelsChangedPredicate(),
		snapshotLifecycleEventPredicate(),
	)
}

type snapshotObjectInputFunc func(client.Object) (any, bool)

func snapshotObjectMutationPredicate(input snapshotObjectInputFunc) predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			oldInput, oldOK := input(updateEvent.ObjectOld)
			newInput, newOK := input(updateEvent.ObjectNew)
			if !oldOK || !newOK {
				return true
			}
			return !apiequality.Semantic.DeepEqual(oldInput, newInput)
		},
	}
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
			if oldAffects != newAffects {
				return true
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
	Ports       []servicePortSnapshotInput
	Annotations map[string]string
	Labels      map[string]string
}

type servicePortSnapshotInput struct {
	Name        string
	Port        int32
	Protocol    corev1.Protocol
	AppProtocol string
}

func serviceSnapshotInputValue(service *corev1.Service) serviceSnapshotInput {
	if service == nil {
		return serviceSnapshotInput{}
	}
	return serviceSnapshotInput{
		Ports: canonicalServicePorts(service.Spec.Ports),
		Annotations: filterStringMapKeys(service.Annotations, []string{
			mesh.ManagedServiceAnnotation,
			mesh.ShadowServiceAnnotation,
		}),
		Labels: filterStringMapKeys(service.Labels, []string{
			resources.ServiceRoleKey,
			mesh.OriginalServiceNamespaceLabel,
			mesh.OriginalServiceNameLabel,
		}),
	}
}

func canonicalServicePorts(ports []corev1.ServicePort) []servicePortSnapshotInput {
	if len(ports) == 0 {
		return nil
	}
	out := make([]servicePortSnapshotInput, 0, len(ports))
	for _, port := range ports {
		appProtocol := ""
		if port.AppProtocol != nil {
			appProtocol = *port.AppProtocol
		}
		out = append(out, servicePortSnapshotInput{
			Name:        port.Name,
			Port:        port.Port,
			Protocol:    port.Protocol,
			AppProtocol: appProtocol,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		if out[i].Protocol != out[j].Protocol {
			return out[i].Protocol < out[j].Protocol
		}
		return out[i].AppProtocol < out[j].AppProtocol
	})
	return out
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
	Included  bool
}

func podSnapshotInputValue(pod *corev1.Pod) podSnapshotInput {
	if pod == nil {
		return podSnapshotInput{}
	}
	return podSnapshotInput{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		PodIP:     pod.Status.PodIP,
		Included:  podContributesWorkload(pod.Status),
	}
}

func (v podSnapshotInput) HasWorkloadIdentity() bool {
	return v.Namespace != "" && v.Name != "" && v.PodIP != "" && v.Included
}

func podContributesWorkload(status corev1.PodStatus) bool {
	if status.PodIP == "" {
		return false
	}
	return status.Phase == corev1.PodRunning || status.Phase == corev1.PodPending
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
			if oldAffects != newAffects {
				return true
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
	Ports       []endpointPortSnapshotInput
	Endpoints   []endpointSnapshotInput
}

type endpointPortSnapshotInput struct {
	Name     string
	Port     int32
	HasPort  bool
	Protocol corev1.Protocol
}

type endpointSnapshotInput struct {
	Addresses []string
	Ready     bool
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

		zone := ""
		if endpoint.Zone != nil {
			zone = *endpoint.Zone
		}

		endpoints = append(endpoints, endpointSnapshotInput{
			Addresses: addresses,
			Ready:     endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready,
			Zone:      zone,
		})
	}
	sort.Slice(endpoints, func(i, j int) bool {
		left := strings.Join(endpoints[i].Addresses, ",") + "\x00" + strconv.FormatBool(endpoints[i].Ready) + "\x00" + endpoints[i].Zone
		right := strings.Join(endpoints[j].Addresses, ",") + "\x00" + strconv.FormatBool(endpoints[j].Ready) + "\x00" + endpoints[j].Zone
		return left < right
	})

	return endpointSliceSnapshotInput{
		AddressType: endpointSlice.AddressType,
		Labels: filterStringMapKeys(endpointSlice.Labels, []string{
			discoveryv1.LabelServiceName,
			mcsv1alpha1.LabelServiceName,
		}),
		Ports:     canonicalEndpointPorts(endpointSlice.Ports),
		Endpoints: endpoints,
	}
}

func canonicalEndpointPorts(ports []discoveryv1.EndpointPort) []endpointPortSnapshotInput {
	if len(ports) == 0 {
		return nil
	}
	out := make([]endpointPortSnapshotInput, 0, len(ports))
	for _, port := range ports {
		item := endpointPortSnapshotInput{}
		if port.Name != nil {
			item.Name = *port.Name
		}
		if port.Port != nil {
			item.Port = *port.Port
			item.HasPort = true
		}
		if port.Protocol != nil {
			item.Protocol = *port.Protocol
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		if out[i].HasPort != out[j].HasPort {
			return !out[i].HasPort
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Protocol < out[j].Protocol
	})
	return out
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

func snapshotServiceImportMutationPredicate() predicate.Predicate {
	return snapshotObjectMutationPredicate(serviceImportSnapshotInputValue)
}

type serviceImportSnapshotInput struct {
	Namespace string
	Name      string
	Spec      mcsv1alpha1.ServiceImportSpec
}

func serviceImportSnapshotInputValue(object client.Object) (any, bool) {
	item, ok := object.(*mcsv1alpha1.ServiceImport)
	if !ok || item == nil {
		return nil, false
	}
	return serviceImportSnapshotInput{
		Namespace: item.Namespace,
		Name:      item.Name,
		Spec:      item.Spec,
	}, true
}

func snapshotBackendTLSPolicyMutationPredicate() predicate.Predicate {
	return snapshotObjectMutationPredicate(backendTLSPolicySnapshotInputValue)
}

type backendTLSPolicySnapshotInput struct {
	Namespace         string
	Name              string
	CreationTimestamp metav1.Time
	Spec              gatewayv1.BackendTLSPolicySpec
}

func backendTLSPolicySnapshotInputValue(object client.Object) (any, bool) {
	switch item := object.(type) {
	case *gatewayv1alpha3.BackendTLSPolicy:
		if item == nil {
			return nil, false
		}
		return backendTLSPolicySnapshotInput{
			Namespace:         item.Namespace,
			Name:              item.Name,
			CreationTimestamp: item.CreationTimestamp,
			Spec:              item.Spec,
		}, true
	case *unstructured.Unstructured:
		if item == nil || item.GroupVersionKind() != gatewayapi.BackendTLSPolicyV1GVK {
			return nil, false
		}
		policy, err := gatewayapi.DecodeBackendTLSPolicyV1(item)
		if err != nil {
			return nil, false
		}
		return backendTLSPolicySnapshotInput{
			Namespace:         policy.Namespace,
			Name:              policy.Name,
			CreationTimestamp: policy.CreationTimestamp,
			Spec:              policy.Spec,
		}, true
	default:
		return nil, false
	}
}

func snapshotBackendLBPolicyMutationPredicate() predicate.Predicate {
	return snapshotObjectMutationPredicate(backendLBPolicySnapshotInputValue)
}

type backendLBPolicySnapshotInput struct {
	Namespace         string
	Name              string
	CreationTimestamp metav1.Time
	Spec              backend.BackendLBPolicySpec
}

func backendLBPolicySnapshotInputValue(object client.Object) (any, bool) {
	item, ok := object.(*backend.BackendLBPolicy)
	if !ok || item == nil {
		return nil, false
	}
	return backendLBPolicySnapshotInput{
		Namespace:         item.Namespace,
		Name:              item.Name,
		CreationTimestamp: item.CreationTimestamp,
		Spec:              item.Spec,
	}, true
}

func snapshotAIServiceMutationPredicate() predicate.Predicate {
	return snapshotObjectMutationPredicate(aiServiceSnapshotInputValue)
}

type aiServiceSnapshotInput struct {
	Namespace string
	Name      string
	Spec      aiservice.AIServiceSpec
}

func aiServiceSnapshotInputValue(object client.Object) (any, bool) {
	item, ok := object.(*aiservice.AIService)
	if !ok || item == nil {
		return nil, false
	}
	return aiServiceSnapshotInput{
		Namespace: item.Namespace,
		Name:      item.Name,
		Spec:      item.Spec,
	}, true
}

func snapshotTokenPolicyMutationPredicate() predicate.Predicate {
	return snapshotObjectMutationPredicate(tokenPolicySnapshotInputValue)
}

type tokenPolicySnapshotInput struct {
	Namespace string
	Name      string
	Spec      tokenpolicy.TokenPolicySpec
}

func tokenPolicySnapshotInputValue(object client.Object) (any, bool) {
	item, ok := object.(*tokenpolicy.TokenPolicy)
	if !ok || item == nil {
		return nil, false
	}
	return tokenPolicySnapshotInput{
		Namespace: item.Namespace,
		Name:      item.Name,
		Spec:      item.Spec,
	}, true
}

func snapshotWasmPluginMutationPredicate() predicate.Predicate {
	return snapshotObjectMutationPredicate(wasmPluginSnapshotInputValue)
}

type wasmPluginSnapshotInput struct {
	Namespace string
	Name      string
	Spec      wasmplugin.WasmPluginSpec
}

func wasmPluginSnapshotInputValue(object client.Object) (any, bool) {
	item, ok := object.(*wasmplugin.WasmPlugin)
	if !ok || item == nil {
		return nil, false
	}
	return wasmPluginSnapshotInput{
		Namespace: item.Namespace,
		Name:      item.Name,
		Spec:      item.Spec,
	}, true
}

func snapshotRoutePolicyMutationPredicate() predicate.Predicate {
	return snapshotObjectMutationPredicate(routePolicySnapshotInputValue)
}

type routePolicySnapshotInput struct {
	Namespace         string
	Name              string
	CreationTimestamp metav1.Time
	Spec              routepolicy.RoutePolicySpec
}

func routePolicySnapshotInputValue(object client.Object) (any, bool) {
	item, ok := object.(*routepolicy.RoutePolicy)
	if !ok || item == nil {
		return nil, false
	}
	return routePolicySnapshotInput{
		Namespace:         item.Namespace,
		Name:              item.Name,
		CreationTimestamp: item.CreationTimestamp,
		Spec:              item.Spec,
	}, true
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

func snapshotRouteLabelsChangedPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc:  func(event.CreateEvent) bool { return true },
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return true },
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			if !supportsRouteLabels(updateEvent.ObjectOld) || !supportsRouteLabels(updateEvent.ObjectNew) {
				return false
			}
			return !stringMapsEqual(updateEvent.ObjectOld.GetLabels(), updateEvent.ObjectNew.GetLabels())
		},
	}
}

func supportsRouteLabels(object client.Object) bool {
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
		if strings.HasPrefix(key, constants.SnapshotRelevantAnnotationPrefix) {
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
