package infrastructure

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	sharedEndpointSliceRoleValue  = "shared-frontend-endpoints"
	gatewayEndpointSliceRoleValue = "gateway-frontend-endpoints"

	sharedEndpointSliceNamePrefix  = "nantian-gw-shared-ep-"
	gatewayEndpointSliceNamePrefix = "nantian-gw-gateway-ep-"
)

type serviceEndpointState struct {
	endpoints     map[string]corev1.Endpoints
	managedSlices map[string]map[string]discoveryv1.EndpointSlice
	foreignSlices map[string]map[string]discoveryv1.EndpointSlice
}

func loadServiceEndpointState(
	ctx context.Context,
	cl client.Client,
	serviceKeys map[string]struct{},
	managedRole string,
) (serviceEndpointState, error) {
	state := serviceEndpointState{
		endpoints:     make(map[string]corev1.Endpoints, len(serviceKeys)),
		managedSlices: make(map[string]map[string]discoveryv1.EndpointSlice, len(serviceKeys)),
		foreignSlices: make(map[string]map[string]discoveryv1.EndpointSlice, len(serviceKeys)),
	}
	if len(serviceKeys) == 0 {
		return state, nil
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
			state.endpoints[key] = *endpoint
		}
	}

	if err := loadEndpointSlicesForServices(
		ctx,
		cl,
		endpointSliceNamespaceQueries(serviceKeys),
		managedRole,
		state.managedSlices,
		state.foreignSlices,
	); err != nil {
		return state, err
	}

	return state, nil
}

type endpointSliceNamespaceQuery struct {
	namespace    string
	serviceNames []string
	allowed      map[string]struct{}
}

func endpointSliceNamespaceQueries(serviceKeys map[string]struct{}) []endpointSliceNamespaceQuery {
	byNamespace := make(map[string]map[string]struct{})
	for key := range serviceKeys {
		namespace, serviceName, ok := splitServiceKey(key)
		if !ok {
			continue
		}
		if byNamespace[namespace] == nil {
			byNamespace[namespace] = make(map[string]struct{})
		}
		byNamespace[namespace][serviceName] = struct{}{}
	}

	namespaces := make([]string, 0, len(byNamespace))
	for namespace := range byNamespace {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)

	out := make([]endpointSliceNamespaceQuery, 0, len(namespaces))
	for _, namespace := range namespaces {
		allowed := byNamespace[namespace]
		serviceNames := make([]string, 0, len(allowed))
		for serviceName := range allowed {
			serviceNames = append(serviceNames, serviceName)
		}
		sort.Strings(serviceNames)
		out = append(out, endpointSliceNamespaceQuery{
			namespace:    namespace,
			serviceNames: serviceNames,
			allowed:      allowed,
		})
	}
	return out
}

func loadEndpointSlicesForServices(
	ctx context.Context,
	cl client.Client,
	queries []endpointSliceNamespaceQuery,
	managedRole string,
	managed map[string]map[string]discoveryv1.EndpointSlice,
	foreign map[string]map[string]discoveryv1.EndpointSlice,
) error {
	for _, query := range queries {
		selector, err := endpointSliceServiceNameSelector(query.serviceNames)
		if err != nil {
			return err
		}

		var endpointSlices discoveryv1.EndpointSliceList
		if err := cl.List(
			ctx,
			&endpointSlices,
			client.InNamespace(query.namespace),
			client.MatchingLabelsSelector{Selector: selector},
		); err != nil {
			return err
		}

		for _, endpointSlice := range endpointSlices.Items {
			serviceName := endpointSlice.Labels[discoveryv1.LabelServiceName]
			if _, ok := query.allowed[serviceName]; !ok {
				continue
			}

			key := serviceKey(endpointSlice.Namespace, serviceName)
			target := foreign
			if endpointSlice.Labels[discoveryv1.LabelManagedBy] == managedByValue &&
				endpointSlice.Labels[serviceRoleLabel] == managedRole {
				target = managed
			}
			if target[key] == nil {
				target[key] = make(map[string]discoveryv1.EndpointSlice)
			}
			target[key][endpointSlice.Name] = endpointSlice
		}
	}
	return nil
}

func endpointSliceServiceNameSelector(serviceNames []string) (labels.Selector, error) {
	requirement, err := labels.NewRequirement(
		discoveryv1.LabelServiceName,
		selection.In,
		serviceNames,
	)
	if err != nil {
		return nil, err
	}
	return labels.NewSelector().Add(*requirement), nil
}

func cleanupServiceEndpointResources(
	ctx context.Context,
	cl client.Client,
	key string,
	state serviceEndpointState,
) error {
	if err := deleteServiceEndpoints(ctx, cl, state.endpoints[key]); err != nil {
		return err
	}
	if err := deleteFrontendEndpointSlices(ctx, cl, state.managedSlices[key]); err != nil {
		return err
	}
	return deleteFrontendEndpointSlices(ctx, cl, state.foreignSlices[key])
}

func reconcileFrontendEndpointSlices(
	ctx context.Context,
	cl client.Client,
	service corev1.Service,
	dataplanePods []corev1.Pod,
	current map[string]discoveryv1.EndpointSlice,
	roleValue string,
	namePrefix string,
) error {
	desired := desiredFrontendEndpointSlices(service, dataplanePods, roleValue, namePrefix)
	desiredNames := make(map[string]struct{}, len(desired))

	for _, endpointSlice := range desired {
		desiredNames[endpointSlice.Name] = struct{}{}
		if err := applyEndpointSlice(
			ctx,
			cl,
			endpointSliceOrEmpty(current[endpointSlice.Name]),
			endpointSlice,
		); err != nil {
			return err
		}
	}

	for name, endpointSlice := range current {
		if _, ok := desiredNames[name]; ok {
			continue
		}
		if err := cl.Delete(ctx, &endpointSlice); client.IgnoreNotFound(err) != nil {
			return err
		}
	}

	return nil
}

func deleteFrontendEndpointSlices(
	ctx context.Context,
	cl client.Client,
	slices map[string]discoveryv1.EndpointSlice,
) error {
	for _, endpointSlice := range slices {
		if err := cl.Delete(ctx, &endpointSlice); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func desiredFrontendEndpointSlices(
	service corev1.Service,
	dataplanePods []corev1.Pod,
	roleValue string,
	namePrefix string,
) []*discoveryv1.EndpointSlice {
	endpointsByFamily := meshDataplaneEndpoints(dataplanePods)
	ports := meshEndpointSlicePorts(service.Spec.Ports)

	families := make([]discoveryv1.AddressType, 0, len(endpointsByFamily))
	for family, endpoints := range endpointsByFamily {
		if len(endpoints) == 0 {
			continue
		}
		families = append(families, family)
	}
	sort.Slice(families, func(i, j int) bool {
		return families[i] < families[j]
	})

	out := make([]*discoveryv1.EndpointSlice, 0, len(families))
	for _, family := range families {
		endpoints := endpointsByFamily[family]
		labels, annotations := frontendEndpointSliceMetadata(service, roleValue)
		out = append(out, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:            frontendEndpointSliceName(namePrefix, service.Namespace, service.Name, family),
				Namespace:       service.Namespace,
				Labels:          labels,
				Annotations:     annotations,
				OwnerReferences: desiredEndpointSliceOwnerReferences(service),
			},
			AddressType: family,
			Endpoints:   endpoints,
			Ports:       ports,
		})
	}

	return out
}

func frontendEndpointSliceMetadata(
	service corev1.Service,
	roleValue string,
) (map[string]string, map[string]string) {
	labels := cloneStringMap(service.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[managedByLabel] = managedByValue
	labels[serviceRoleLabel] = roleValue
	labels[discoveryv1.LabelManagedBy] = managedByValue
	labels[discoveryv1.LabelServiceName] = service.Name

	return labels, cloneStringMap(service.Annotations)
}

func GatewayFrontendEndpointSliceMetadataMatches(
	current discoveryv1.EndpointSlice,
	service corev1.Service,
) bool {
	labels, annotations := frontendEndpointSliceMetadata(service, gatewayEndpointSliceRoleValue)
	return current.Namespace == service.Namespace &&
		stringMapEqual(current.Labels, labels) &&
		stringMapEqual(current.Annotations, annotations) &&
		ownerReferencesEqual(current.OwnerReferences, desiredEndpointSliceOwnerReferences(service))
}

func frontendEndpointSliceName(
	prefix string,
	namespace string,
	serviceName string,
	addressType discoveryv1.AddressType,
) string {
	const maxLen = 63

	suffix := strings.ToLower(string(addressType))
	base := prefix + serviceName + "-" + suffix
	if len(base) <= maxLen {
		return base
	}

	hashSuffix := fmt.Sprintf("%08x", hashString(namespace+"/"+serviceName+"/"+suffix))
	trimmed := serviceName[:maxLen-len(prefix)-len(hashSuffix)-len(suffix)-2]
	return prefix + trimmed + "-" + suffix + "-" + hashSuffix
}

func serviceKey(namespace string, name string) string {
	return namespace + "/" + name
}

func splitServiceKey(key string) (string, string, bool) {
	namespace, name, ok := strings.Cut(key, "/")
	if !ok || namespace == "" || name == "" {
		return "", "", false
	}
	return namespace, name, true
}
