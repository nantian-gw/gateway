package infrastructure

import (
	"context"
	"fmt"
	"hash/fnv"
	"net"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
)

type servicePortKey struct {
	port     int32
	protocol corev1.Protocol
}

func desiredSharedService(
	current *corev1.Service,
	gateways []gatewayv1.Gateway,
	options Options,
) *corev1.Service {
	ports := sharedServicePorts(gateways)
	if len(ports) == 0 {
		return nil
	}

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      options.SharedServiceName,
			Namespace: options.DataplaneNamespace,
			Labels: map[string]string{
				managedByLabel:   managedByValue,
				serviceRoleLabel: serviceRoleShared,
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
		},
	}

	if current.Name != "" {
		desired.ResourceVersion = current.ResourceVersion
		desired.UID = current.UID
		desired.Spec.ClusterIP = current.Spec.ClusterIP
		desired.Spec.ClusterIPs = append([]string(nil), current.Spec.ClusterIPs...)
		desired.Spec.IPFamilies = append([]corev1.IPFamily(nil), current.Spec.IPFamilies...)
		desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
		desired.Spec.InternalTrafficPolicy = current.Spec.InternalTrafficPolicy
		desired.Spec.ExternalTrafficPolicy = current.Spec.ExternalTrafficPolicy
		desired.Spec.SessionAffinity = current.Spec.SessionAffinity
	}

	if len(options.DataplaneSelector) > 0 {
		desired.Spec.Selector = options.DataplaneSelector
	}
	desired.Spec.Ports = assignSharedNodePorts(mergeServicePorts(current.Spec.Ports, ports, true), options)
	return desired
}

func desiredGatewayService(
	current *corev1.Service,
	gateway gatewayv1.Gateway,
	params gatewayServiceParameters,
	gatewayClassParametersRef string,
) *corev1.Service {
	ports := gatewayServicePorts(gateway)
	if len(ports) == 0 {
		return nil
	}

	labels := map[string]string{
		managedByLabel:        managedByValue,
		serviceRoleLabel:      serviceRoleGateway,
		gatewayNameLabel:      gateway.Name,
		gatewayNamespaceLabel: gateway.Namespace,
	}
	if gateway.Spec.Infrastructure != nil {
		for key, value := range gateway.Spec.Infrastructure.Labels {
			labels[string(key)] = string(value)
		}
	}

	desired := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            gatewayServiceName(gateway.Name),
			Namespace:       gateway.Namespace,
			Labels:          labels,
			Annotations:     desiredGatewayServiceAnnotations(gateway, params, gatewayClassParametersRef),
			OwnerReferences: desiredGatewayServiceOwnerReferences(gateway),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Add Selector only when the Gateway is in the same namespace as dataplane pods;
	// cross-namespace gateways rely on managed EndpointSlices.
	if gateway.Namespace == defaultDataplaneNamespace {
		desired.Spec.Selector = map[string]string{"app": "nantian-gw-dataplane"}
	}

	if current.Name != "" {
		desired.ResourceVersion = current.ResourceVersion
		desired.UID = current.UID
		desired.Spec.ClusterIP = current.Spec.ClusterIP
		desired.Spec.ClusterIPs = append([]string(nil), current.Spec.ClusterIPs...)
		desired.Spec.IPFamilies = append([]corev1.IPFamily(nil), current.Spec.IPFamilies...)
		desired.Spec.IPFamilyPolicy = current.Spec.IPFamilyPolicy
	}

	applyGatewayServiceParameters(&desired.Spec, params)
	applyGatewayStaticAddresses(&desired.Spec, gateway.Spec.Addresses)
	applyGatewayStaticAddressFamilies(&desired.Spec, gateway.Spec.Addresses)
	desired.Spec.Ports = mergeServicePorts(
		current.Spec.Ports,
		ports,
		shouldPreserveNodePorts(desired.Spec),
	)
	return desired
}

func GatewayServiceMetadataMatches(
	current corev1.Service,
	gateway gatewayv1.Gateway,
	gatewayClassParametersRef string,
) bool {
	desiredLabels := map[string]string{
		managedByLabel:        managedByValue,
		serviceRoleLabel:      serviceRoleGateway,
		gatewayNameLabel:      gateway.Name,
		gatewayNamespaceLabel: gateway.Namespace,
	}
	if gateway.Spec.Infrastructure != nil {
		for key, value := range gateway.Spec.Infrastructure.Labels {
			desiredLabels[string(key)] = string(value)
		}
	}

	ownerReferencesMatch := true
	if gateway.UID != "" {
		ownerReferencesMatch = ownerReferencesEqual(
			current.OwnerReferences,
			desiredGatewayServiceOwnerReferences(gateway),
		)
	}

	return current.Name == gatewayServiceName(gateway.Name) &&
		current.Namespace == gateway.Namespace &&
		stringMapEqual(current.Labels, desiredLabels) &&
		stringMapEqual(filterGatewayServiceUserAnnotations(current.Annotations), gatewayInfrastructureAnnotations(gateway)) &&
		stringMapEqual(
			filterGatewayServiceConvergenceAnnotations(current.Annotations, gateway.UID != ""),
			desiredGatewayServiceConvergenceAnnotations(gateway, gatewayClassParametersRef),
		) &&
		ownerReferencesMatch
}

func applyService(ctx context.Context, cl client.Client, current, desired *corev1.Service) error {
	if current.Name == "" {
		return cl.Create(ctx, desired)
	}

	if serviceEqual(current, desired) {
		return nil
	}

	return cl.Update(ctx, desired)
}

func serviceEqual(current, desired *corev1.Service) bool {
	if current.Name != desired.Name ||
		current.Namespace != desired.Namespace ||
		!stringMapEqual(current.Labels, desired.Labels) ||
		!stringMapEqual(current.Annotations, desired.Annotations) ||
		!ownerReferencesEqual(current.OwnerReferences, desired.OwnerReferences) {
		return false
	}

	return current.Spec.Type == desired.Spec.Type &&
		stringMapEqual(current.Spec.Selector, desired.Spec.Selector) &&
		current.Spec.ClusterIP == desired.Spec.ClusterIP &&
		stringSliceEqual(current.Spec.ClusterIPs, desired.Spec.ClusterIPs) &&
		stringSliceEqual(current.Spec.ExternalIPs, desired.Spec.ExternalIPs) &&
		current.Spec.LoadBalancerIP == desired.Spec.LoadBalancerIP &&
		ipFamiliesEqual(current.Spec.IPFamilies, desired.Spec.IPFamilies) &&
		ipFamilyPolicyEqual(current.Spec.IPFamilyPolicy, desired.Spec.IPFamilyPolicy) &&
		internalTrafficPolicyEqual(current.Spec.InternalTrafficPolicy, desired.Spec.InternalTrafficPolicy) &&
		current.Spec.ExternalTrafficPolicy == desired.Spec.ExternalTrafficPolicy &&
		current.Spec.SessionAffinity == desired.Spec.SessionAffinity &&
		current.Spec.PublishNotReadyAddresses == desired.Spec.PublishNotReadyAddresses &&
		stringSliceEqual(current.Spec.LoadBalancerSourceRanges, desired.Spec.LoadBalancerSourceRanges) &&
		stringPointerEqual(current.Spec.LoadBalancerClass, desired.Spec.LoadBalancerClass) &&
		boolPointerEqual(
			current.Spec.AllocateLoadBalancerNodePorts,
			desired.Spec.AllocateLoadBalancerNodePorts,
		) &&
		servicePortsEqual(current.Spec.Ports, desired.Spec.Ports)
}

func sharedServicePorts(gateways []gatewayv1.Gateway) []corev1.ServicePort {
	return collectListenerPorts(gateways)
}

func gatewayServicePorts(gateway gatewayv1.Gateway) []corev1.ServicePort {
	index := make(map[servicePortKey]corev1.ServicePort)
	for _, listener := range gatewayapi.InfrastructureListeners(gateway) {
		protocol, ok := serviceProtocol(listener.Protocol)
		if !ok {
			continue
		}
		port := listener.Port
		key := servicePortKey{port: port, protocol: protocol}
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = corev1.ServicePort{
			Name:       servicePortName(protocol, port),
			Port:       port,
			TargetPort: intstr.FromInt(int(port)),
			Protocol:   protocol,
		}
	}
	out := make([]corev1.ServicePort, 0, len(index))
	for _, port := range index {
		out = append(out, port)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Protocol < out[j].Protocol
	})
	return out
}

func collectListenerPorts(gateways []gatewayv1.Gateway) []corev1.ServicePort {
	index := make(map[servicePortKey]corev1.ServicePort)
	for _, gateway := range gateways {
		for _, listener := range gatewayapi.InfrastructureListeners(gateway) {
			protocol, ok := serviceProtocol(listener.Protocol)
			if !ok {
				continue
			}

			port := listener.Port
			key := servicePortKey{port: port, protocol: protocol}
			if _, exists := index[key]; exists {
				continue
			}

			index[key] = corev1.ServicePort{
				Name:       servicePortName(protocol, port),
				Port:       port,
				TargetPort: intstr.FromInt(int(port)),
				Protocol:   protocol,
			}
		}
	}

	out := make([]corev1.ServicePort, 0, len(index))
	for _, port := range index {
		out = append(out, port)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Protocol < out[j].Protocol
	})
	return out
}

func mergeServicePorts(
	current []corev1.ServicePort,
	desired []corev1.ServicePort,
	preserveNodePorts bool,
) []corev1.ServicePort {
	currentByKey := make(map[servicePortKey]corev1.ServicePort, len(current))
	for _, port := range current {
		currentByKey[servicePortKey{port: port.Port, protocol: port.Protocol}] = port
	}

	out := make([]corev1.ServicePort, 0, len(desired))
	for _, port := range desired {
		if existing, ok := currentByKey[servicePortKey{port: port.Port, protocol: port.Protocol}]; ok {
			if preserveNodePorts {
				port.NodePort = existing.NodePort
			}
			if existing.AppProtocol != nil {
				value := *existing.AppProtocol
				port.AppProtocol = &value
			}
		}
		out = append(out, port)
	}
	return out
}

func shouldPreserveNodePorts(spec corev1.ServiceSpec) bool {
	switch spec.Type {
	case corev1.ServiceTypeNodePort:
		return true
	case corev1.ServiceTypeLoadBalancer:
		return spec.AllocateLoadBalancerNodePorts == nil || *spec.AllocateLoadBalancerNodePorts
	default:
		return false
	}
}

func assignSharedNodePorts(ports []corev1.ServicePort, opts Options) []corev1.ServicePort {
	for idx := range ports {
		if ports[idx].NodePort != 0 {
			continue
		}
		ports[idx].NodePort = sharedNodePortFor(ports[idx].Port, ports[idx].Protocol, opts)
	}
	return ports
}

func gatewayServiceObjectKey(gateway gatewayv1.Gateway) client.ObjectKey {
	return client.ObjectKey{
		Namespace: gateway.Namespace,
		Name:      gatewayServiceName(gateway.Name),
	}
}

func GatewayServiceObjectKey(gateway gatewayv1.Gateway) client.ObjectKey {
	return gatewayServiceObjectKey(gateway)
}

func gatewayServiceName(gatewayName string) string {
	const prefix = "nantian-gw-"
	const maxLen = 63

	base := prefix + gatewayName
	if len(base) <= maxLen {
		return base
	}

	sum := fnv.New32a()
	_, _ = sum.Write([]byte(gatewayName))
	suffix := fmt.Sprintf("%08x", sum.Sum32())
	trimmed := gatewayName[:maxLen-len(prefix)-len(suffix)-1]
	return prefix + trimmed + "-" + suffix
}

func GatewayServiceName(gatewayName string) string {
	return gatewayServiceName(gatewayName)
}

func serviceProtocol(protocol gatewayv1.ProtocolType) (corev1.Protocol, bool) {
	switch strings.ToUpper(string(protocol)) {
	case "HTTP", "HTTPS", "GRPC", "TCP", "TLS":
		return corev1.ProtocolTCP, true
	case "UDP", "HTTP3":
		return corev1.ProtocolUDP, true
	default:
		return "", false
	}
}

func gatewayServiceAddressType(rawType *gatewayv1.AddressType, value string) gatewayv1.AddressType {
	if rawType != nil && *rawType != "" {
		return *rawType
	}
	if net.ParseIP(strings.TrimSpace(value)) != nil {
		return gatewayv1.IPAddressType
	}
	return gatewayv1.HostnameAddressType
}

func applyGatewayStaticAddresses(spec *corev1.ServiceSpec, addresses []gatewayv1.GatewaySpecAddress) {
	if spec == nil {
		return
	}

	projected := projectGatewayStaticIPAddresses(addresses)
	if !projected.sawIPAddress {
		return
	}
	if len(projected.ips) == 0 {
		spec.ExternalIPs = nil
		spec.LoadBalancerIP = ""
		return
	}
	spec.ExternalIPs = projected.ips
	if spec.Type == corev1.ServiceTypeLoadBalancer && len(projected.ips) > 0 {
		spec.LoadBalancerIP = projected.ips[0]
	} else {
		spec.LoadBalancerIP = ""
	}
}

func applyGatewayStaticAddressFamilies(spec *corev1.ServiceSpec, addresses []gatewayv1.GatewaySpecAddress) {
	if spec == nil {
		return
	}
	if spec.IPFamilyPolicy != nil || len(spec.IPFamilies) > 0 {
		return
	}

	projected := projectGatewayStaticIPAddresses(addresses)
	if !projected.sawIPAddress {
		return
	}
	if len(projected.families) == 0 {
		spec.IPFamilies = nil
		spec.IPFamilyPolicy = nil
		return
	}

	spec.IPFamilies = append([]corev1.IPFamily(nil), projected.families...)
	switch len(projected.families) {
	case 1:
		policy := corev1.IPFamilyPolicySingleStack
		spec.IPFamilyPolicy = &policy
	default:
		policy := corev1.IPFamilyPolicyPreferDualStack
		spec.IPFamilyPolicy = &policy
	}
}

type gatewayStaticIPProjection struct {
	sawIPAddress bool
	ips          []string
	families     []corev1.IPFamily
}

func projectGatewayStaticIPAddresses(addresses []gatewayv1.GatewaySpecAddress) gatewayStaticIPProjection {
	projectedIPs := make([]string, 0, len(addresses))
	families := make([]corev1.IPFamily, 0, 2)
	seenIPs := make(map[string]struct{}, len(addresses))
	seenFamilies := make(map[corev1.IPFamily]struct{}, 2)
	sawIPAddress := false

	for _, address := range addresses {
		addressType := gatewayServiceAddressType(address.Type, address.Value)
		if addressType != gatewayv1.IPAddressType {
			continue
		}
		sawIPAddress = true
		value := strings.TrimSpace(address.Value)
		if value == "" {
			continue
		}
		ip := net.ParseIP(value)
		if ip == nil {
			continue
		}
		if !isValidServiceExternalIP(ip) {
			continue
		}

		normalized := ip.String()
		if _, ok := seenIPs[normalized]; !ok {
			seenIPs[normalized] = struct{}{}
			projectedIPs = append(projectedIPs, normalized)
		}

		family := gatewayStaticIPFamily(ip)
		if _, ok := seenFamilies[family]; ok {
			continue
		}
		seenFamilies[family] = struct{}{}
		families = append(families, family)
	}

	sort.Strings(projectedIPs)
	sort.Slice(families, func(i, j int) bool {
		return families[i] < families[j]
	})

	return gatewayStaticIPProjection{
		sawIPAddress: sawIPAddress,
		ips:          projectedIPs,
		families:     families,
	}
}

func gatewayStaticIPFamily(ip net.IP) corev1.IPFamily {
	if ip.To4() != nil {
		return corev1.IPv4Protocol
	}
	return corev1.IPv6Protocol
}

func isValidServiceExternalIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}

func servicePortName(protocol corev1.Protocol, port int32) string {
	return strings.ToLower(string(protocol)) + "-" + fmt.Sprint(port)
}

func sharedNodePortFor(port int32, protocol corev1.Protocol, opts Options) int32 {
	switch protocol {
	case corev1.ProtocolUDP:
		return opts.NodePortBaseUDP + (port % 1000)
	default:
		if port < 1024 {
			return opts.NodePortBasePrivileged + port
		}
		nodePort := opts.NodePortBaseDefault + (port % 1000)
		if nodePort > opts.NodePortRangeMax {
			nodePort -= 1000
		}
		return nodePort
	}
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}

	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func gatewayInfrastructureAnnotations(gateway gatewayv1.Gateway) map[string]string {
	if gateway.Spec.Infrastructure == nil || len(gateway.Spec.Infrastructure.Annotations) == 0 {
		return nil
	}

	out := make(map[string]string, len(gateway.Spec.Infrastructure.Annotations))
	for key, value := range gateway.Spec.Infrastructure.Annotations {
		out[string(key)] = string(value)
	}
	return out
}

func filterGatewayServiceUserAnnotations(annotations map[string]string) map[string]string {
	if len(annotations) == 0 {
		return nil
	}

	out := make(map[string]string, len(annotations))
	for key, value := range annotations {
		if isManagedGatewayServiceAnnotation(key) {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isManagedGatewayServiceAnnotation(key string) bool {
	switch key {
	case derivedResourceOwnerKindAnnotation,
		derivedResourceOwnerNamespaceAnnotation,
		derivedResourceOwnerNameAnnotation,
		derivedResourceOwnerUIDAnnotation,
		derivedResourceOwnerGenerationAnnotation,
		derivedResourceGatewayClassNameAnnotation,
		derivedResourceGatewayClassParametersRefAnnotation,
		derivedResourceInfrastructureParametersRefAnnotation,
		derivedResourceServiceParametersHashAnnotation:
		return true
	default:
		return false
	}
}

func ownerReferencesEqual(left, right []metav1.OwnerReference) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx].APIVersion != right[idx].APIVersion ||
			left[idx].Kind != right[idx].Kind ||
			left[idx].Name != right[idx].Name ||
			left[idx].UID != right[idx].UID ||
			!boolPointerEqual(left[idx].Controller, right[idx].Controller) ||
			!boolPointerEqual(left[idx].BlockOwnerDeletion, right[idx].BlockOwnerDeletion) {
			return false
		}
	}
	return true
}

func stringMapEqual(left, right map[string]string) bool {
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

func stringSliceEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func ipFamiliesEqual(left, right []corev1.IPFamily) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func ipFamilyPolicyEqual(left, right *corev1.IPFamilyPolicy) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func internalTrafficPolicyEqual(left, right *corev1.ServiceInternalTrafficPolicy) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func servicePortsEqual(left, right []corev1.ServicePort) bool {
	if len(left) != len(right) {
		return false
	}

	for idx := range left {
		if left[idx].Name != right[idx].Name ||
			left[idx].Port != right[idx].Port ||
			left[idx].TargetPort != right[idx].TargetPort ||
			left[idx].Protocol != right[idx].Protocol ||
			left[idx].NodePort != right[idx].NodePort {
			return false
		}

		switch {
		case left[idx].AppProtocol == nil || right[idx].AppProtocol == nil:
			if left[idx].AppProtocol != nil || right[idx].AppProtocol != nil {
				return false
			}
		case *left[idx].AppProtocol != *right[idx].AppProtocol:
			return false
		}
	}

	return true
}

func mustGetService(
	ctx context.Context,
	cl client.Client,
	key client.ObjectKey,
) (*corev1.Service, error) {
	service := &corev1.Service{}
	if err := cl.Get(ctx, key, service); err != nil {
		return nil, err
	}
	return service, nil
}

func serviceAfterApply(
	ctx context.Context,
	cl client.Client,
	service *corev1.Service,
) (*corev1.Service, error) {
	if service == nil {
		return nil, fmt.Errorf("service must not be nil")
	}
	if service.UID != "" {
		return service, nil
	}
	return mustGetService(ctx, cl, client.ObjectKeyFromObject(service))
}

func serviceMissing(err error) bool {
	return apierrors.IsNotFound(err)
}
