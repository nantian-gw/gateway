package infrastructure

import (
	"context"
	"fmt"
	"hash/fnv"
	"net/netip"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const meshEndpointSliceRoleValue = "mesh-frontend-endpoints"

func reconcileMeshEndpointSlices(
	ctx context.Context,
	cl client.Client,
	service corev1.Service,
	dataplanePods []corev1.Pod,
	current map[string]discoveryv1.EndpointSlice,
) error {
	return reconcileMeshEndpointSlicesFromDataplaneEndpoints(
		ctx,
		cl,
		service,
		meshDataplaneEndpoints(dataplanePods),
		current,
	)
}

func reconcileMeshEndpointSlicesFromDataplaneEndpoints(
	ctx context.Context,
	cl client.Client,
	service corev1.Service,
	endpointsByFamily map[discoveryv1.AddressType][]discoveryv1.Endpoint,
	current map[string]discoveryv1.EndpointSlice,
) error {
	desired := desiredMeshEndpointSlicesFromDataplaneEndpoints(service, endpointsByFamily)
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

func deleteMeshEndpointSlices(
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

func desiredMeshEndpointSlices(
	service corev1.Service,
	dataplanePods []corev1.Pod,
) []*discoveryv1.EndpointSlice {
	return desiredMeshEndpointSlicesFromDataplaneEndpoints(
		service,
		meshDataplaneEndpoints(dataplanePods),
	)
}

func desiredMeshEndpointSlicesFromDataplaneEndpoints(
	service corev1.Service,
	endpointsByFamily map[discoveryv1.AddressType][]discoveryv1.Endpoint,
) []*discoveryv1.EndpointSlice {
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
		out = append(out, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:            meshEndpointSliceName(service.Namespace, service.Name, family),
				Namespace:       service.Namespace,
				OwnerReferences: desiredEndpointSliceOwnerReferences(service),
				Labels: map[string]string{
					managedByLabel:               managedByValue,
					serviceRoleLabel:             meshEndpointSliceRoleValue,
					discoveryv1.LabelManagedBy:   managedByValue,
					discoveryv1.LabelServiceName: service.Name,
				},
			},
			AddressType: family,
			Endpoints:   endpoints,
			Ports:       ports,
		})
	}

	return out
}

func meshDataplaneEndpoints(
	pods []corev1.Pod,
) map[discoveryv1.AddressType][]discoveryv1.Endpoint {
	out := map[discoveryv1.AddressType][]discoveryv1.Endpoint{
		discoveryv1.AddressTypeIPv4: {},
		discoveryv1.AddressTypeIPv6: {},
	}

	for _, pod := range pods {
		if !podReady(pod) {
			continue
		}

		for _, address := range podAddresses(pod) {
			family, ok := addressType(address)
			if !ok {
				continue
			}

			ready := true
			endpoint := discoveryv1.Endpoint{
				Addresses: []string{address},
				Conditions: discoveryv1.EndpointConditions{
					Ready: &ready,
				},
				TargetRef: &corev1.ObjectReference{
					APIVersion: "v1",
					Kind:       "Pod",
					Namespace:  pod.Namespace,
					Name:       pod.Name,
					UID:        pod.UID,
				},
			}
			out[family] = append(out[family], endpoint)
		}
	}

	for family := range out {
		sort.Slice(out[family], func(i, j int) bool {
			return out[family][i].Addresses[0] < out[family][j].Addresses[0]
		})
	}

	return out
}

func meshEndpointSlicePorts(ports []corev1.ServicePort) []discoveryv1.EndpointPort {
	out := make([]discoveryv1.EndpointPort, 0, len(ports))
	for _, port := range ports {
		endpointPort := int32(serviceTargetPort(port))
		item := discoveryv1.EndpointPort{
			Port:     &endpointPort,
			Protocol: &port.Protocol,
		}
		if port.Name != "" {
			name := port.Name
			item.Name = &name
		}
		if port.AppProtocol != nil {
			appProtocol := *port.AppProtocol
			item.AppProtocol = &appProtocol
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		leftName := ""
		rightName := ""
		if out[i].Name != nil {
			leftName = *out[i].Name
		}
		if out[j].Name != nil {
			rightName = *out[j].Name
		}
		if leftName != rightName {
			return leftName < rightName
		}
		return *out[i].Port < *out[j].Port
	})

	return out
}

func serviceTargetPort(port corev1.ServicePort) int {
	if port.TargetPort.Type == intstr.Int && port.TargetPort.IntValue() > 0 {
		return port.TargetPort.IntValue()
	}
	if port.TargetPort.Type == intstr.String && port.TargetPort.StrVal != "" {
		return int(port.Port)
	}
	if port.TargetPort.IntValue() > 0 {
		return port.TargetPort.IntValue()
	}
	return int(port.Port)
}

func applyEndpointSlice(
	ctx context.Context,
	cl client.Client,
	current *discoveryv1.EndpointSlice,
	desired *discoveryv1.EndpointSlice,
) error {
	if current.Name == "" {
		return cl.Create(ctx, desired)
	}

	desired.ResourceVersion = current.ResourceVersion
	if endpointSliceEqual(current, desired) {
		return nil
	}
	return cl.Update(ctx, desired)
}

func endpointSliceEqual(
	current *discoveryv1.EndpointSlice,
	desired *discoveryv1.EndpointSlice,
) bool {
	if current.Name != desired.Name ||
		current.Namespace != desired.Namespace ||
		!stringMapEqual(current.Labels, desired.Labels) ||
		!stringMapEqual(current.Annotations, desired.Annotations) ||
		!ownerReferencesEqual(current.OwnerReferences, desired.OwnerReferences) ||
		current.AddressType != desired.AddressType ||
		len(current.Ports) != len(desired.Ports) ||
		len(current.Endpoints) != len(desired.Endpoints) {
		return false
	}

	for idx := range current.Ports {
		if !endpointPortEqual(current.Ports[idx], desired.Ports[idx]) {
			return false
		}
	}
	for idx := range current.Endpoints {
		if !endpointEqual(current.Endpoints[idx], desired.Endpoints[idx]) {
			return false
		}
	}

	return true
}

func endpointPortEqual(left, right discoveryv1.EndpointPort) bool {
	return stringPointerEqual(left.Name, right.Name) &&
		stringPointerEqual(left.AppProtocol, right.AppProtocol) &&
		int32PointerEqual(left.Port, right.Port) &&
		protocolPointerEqual(left.Protocol, right.Protocol)
}

func endpointEqual(left, right discoveryv1.Endpoint) bool {
	return stringSliceEqual(left.Addresses, right.Addresses) &&
		boolPointerEqual(left.Conditions.Ready, right.Conditions.Ready) &&
		objectReferenceEqual(left.TargetRef, right.TargetRef)
}

func objectReferenceEqual(left, right *corev1.ObjectReference) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return left.APIVersion == right.APIVersion &&
		left.Kind == right.Kind &&
		left.Namespace == right.Namespace &&
		left.Name == right.Name &&
		left.UID == right.UID
}

func endpointSliceOrEmpty(
	endpointSlice discoveryv1.EndpointSlice,
) *discoveryv1.EndpointSlice {
	if endpointSlice.Name == "" {
		return &discoveryv1.EndpointSlice{}
	}
	return &endpointSlice
}

func meshEndpointSliceName(
	namespace string,
	serviceName string,
	addressType discoveryv1.AddressType,
) string {
	const prefix = "nantian-gw-mesh-ep-"
	const maxLen = 63

	suffix := strings.ToLower(string(addressType))
	base := prefix + serviceName + "-" + suffix
	if len(base) <= maxLen {
		return base
	}

	hash := hashString(namespace + "/" + serviceName + "/" + suffix)
	hashSuffix := fmt.Sprintf("%08x", hash)
	trimmed := serviceName[:maxLen-len(prefix)-len(hashSuffix)-len(suffix)-2]
	return prefix + trimmed + "-" + suffix + "-" + hashSuffix
}

func podReady(pod corev1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

func podAddresses(pod corev1.Pod) []string {
	if len(pod.Status.PodIPs) > 0 {
		out := make([]string, 0, len(pod.Status.PodIPs))
		for _, podIP := range pod.Status.PodIPs {
			if podIP.IP != "" {
				out = append(out, podIP.IP)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if pod.Status.PodIP == "" {
		return nil
	}
	return []string{pod.Status.PodIP}
}

func addressType(address string) (discoveryv1.AddressType, bool) {
	parsed, err := netip.ParseAddr(address)
	if err != nil {
		return "", false
	}
	if parsed.Is4() {
		return discoveryv1.AddressTypeIPv4, true
	}
	if parsed.Is6() {
		return discoveryv1.AddressTypeIPv6, true
	}
	return "", false
}

func stringPointerEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func int32PointerEqual(left, right *int32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func protocolPointerEqual(left, right *corev1.Protocol) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func boolPointerEqual(left, right *bool) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func hashString(value string) uint32 {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(value))
	return sum.Sum32()
}
