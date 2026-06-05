package infrastructure

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/controlplane/internal/mesh"
)

type meshServiceState struct {
	servicesByKey      map[string]corev1.Service
	shadowByOriginal   map[string]corev1.Service
	managedServiceKeys map[string]struct{}
}

type meshServiceEndpointState struct {
	endpointsByService             map[string]corev1.Endpoints
	managedEndpointSlicesByService map[string]map[string]discoveryv1.EndpointSlice
	foreignEndpointSlicesByService map[string]map[string]discoveryv1.EndpointSlice
}

func loadMeshServiceState(
	ctx context.Context,
	cl client.Client,
	parentKeys []mesh.ServiceParentKey,
) (meshServiceState, error) {
	state := meshServiceState{
		servicesByKey:      make(map[string]corev1.Service, len(parentKeys)),
		shadowByOriginal:   make(map[string]corev1.Service, len(parentKeys)),
		managedServiceKeys: make(map[string]struct{}),
	}

	var managedServices corev1.ServiceList
	selector, err := meshManagedServiceSelector()
	if err != nil {
		return state, err
	}
	if err := cl.List(
		ctx,
		&managedServices,
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		return state, err
	}

	for _, service := range managedServices.Items {
		switch {
		case service.Labels[mesh.ShadowServiceRoleLabel] == mesh.ShadowServiceRoleValue:
			key := serviceKey(
				service.Labels[mesh.OriginalServiceNamespaceLabel],
				service.Labels[mesh.OriginalServiceNameLabel],
			)
			if _, _, ok := splitServiceKey(key); ok {
				state.shadowByOriginal[key] = service
			}
		case service.Annotations[mesh.ManagedServiceAnnotation] == "true" ||
			service.Labels[serviceRoleLabel] == serviceRoleMeshFrontend:
			key := serviceKey(service.Namespace, service.Name)
			state.servicesByKey[key] = service
			state.managedServiceKeys[key] = struct{}{}
		}
	}

	for _, key := range sortedMeshParentServiceKeys(parentKeys) {
		namespace, name, ok := splitServiceKey(key)
		if !ok {
			continue
		}

		if _, exists := state.servicesByKey[key]; !exists {
			service := &corev1.Service{}
			if err := cl.Get(
				ctx,
				client.ObjectKey{Namespace: namespace, Name: name},
				service,
			); client.IgnoreNotFound(err) != nil {
				return state, err
			}
			if service.Name != "" {
				state.servicesByKey[key] = *service
			}
		}

		if _, exists := state.shadowByOriginal[key]; !exists {
			shadow := &corev1.Service{}
			if err := cl.Get(
				ctx,
				client.ObjectKey{
					Namespace: namespace,
					Name:      mesh.ShadowServiceName(namespace, name),
				},
				shadow,
			); client.IgnoreNotFound(err) != nil {
				return state, err
			}
			if shadow.Name != "" {
				state.shadowByOriginal[key] = *shadow
			}
		}
	}

	return state, nil
}

func meshManagedServiceSelector() (labels.Selector, error) {
	managedByReq, err := labels.NewRequirement(managedByLabel, selection.Equals, []string{managedByValue})
	if err != nil {
		return nil, err
	}
	roleReq, err := labels.NewRequirement(
		serviceRoleLabel,
		selection.In,
		[]string{serviceRoleMeshFrontend, mesh.ShadowServiceRoleValue},
	)
	if err != nil {
		return nil, err
	}

	return labels.NewSelector().Add(*managedByReq, *roleReq), nil
}

func loadMeshServiceEndpointState(
	ctx context.Context,
	cl client.Client,
	serviceKeys map[string]struct{},
) (meshServiceEndpointState, error) {
	state := meshServiceEndpointState{
		endpointsByService:             make(map[string]corev1.Endpoints, len(serviceKeys)),
		managedEndpointSlicesByService: make(map[string]map[string]discoveryv1.EndpointSlice),
		foreignEndpointSlicesByService: make(map[string]map[string]discoveryv1.EndpointSlice, len(serviceKeys)),
	}

	for _, key := range sortedServiceKeys(serviceKeys) {
		namespace, serviceName, ok := splitServiceKey(key)
		if !ok {
			continue
		}

		endpoint := &corev1.Endpoints{}
		if err := cl.Get(
			ctx,
			client.ObjectKey{Namespace: namespace, Name: serviceName},
			endpoint,
		); client.IgnoreNotFound(err) != nil {
			return state, err
		}
		if endpoint.Name != "" {
			state.endpointsByService[key] = *endpoint
		}
	}

	if err := loadEndpointSlicesForServices(
		ctx,
		cl,
		endpointSliceNamespaceQueries(serviceKeys),
		meshEndpointSliceRoleValue,
		state.managedEndpointSlicesByService,
		state.foreignEndpointSlicesByService,
	); err != nil {
		return state, err
	}

	return state, nil
}

func sortedMeshParentServiceKeys(parentKeys []mesh.ServiceParentKey) []string {
	keys := make(map[string]struct{}, len(parentKeys))
	for _, parentKey := range parentKeys {
		keys[serviceKey(parentKey.Namespace, parentKey.Name)] = struct{}{}
	}
	return sortedServiceKeys(keys)
}

func sortedServiceKeys(keys map[string]struct{}) []string {
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func meshServices(state meshServiceState) []corev1.Service {
	services := make([]corev1.Service, 0, len(state.servicesByKey))
	for _, key := range sortedServiceKeys(serviceKeySet(state.servicesByKey)) {
		services = append(services, state.servicesByKey[key])
	}
	return services
}

func serviceKeySet(services map[string]corev1.Service) map[string]struct{} {
	keys := make(map[string]struct{}, len(services))
	for key := range services {
		keys[key] = struct{}{}
	}
	return keys
}
