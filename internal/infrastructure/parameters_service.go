package infrastructure

import (
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func (p *gatewayServiceParameters) normalize() {
	if p.SessionAffinityConfig != nil && p.SessionAffinityConfig.ClientIP != nil &&
		p.SessionAffinityConfig.ClientIP.TimeoutSeconds != nil &&
		*p.SessionAffinityConfig.ClientIP.TimeoutSeconds <= 0 {
		p.SessionAffinityConfig.ClientIP.TimeoutSeconds = nil
	}
	if p.SessionAffinityConfig != nil && p.SessionAffinityConfig.ClientIP == nil {
		p.SessionAffinityConfig = nil
	}

	if p.IPFamilyPolicy != nil {
		value := corev1.IPFamilyPolicyType(strings.TrimSpace(string(*p.IPFamilyPolicy)))
		if value == "" {
			p.IPFamilyPolicy = nil
		} else {
			p.IPFamilyPolicy = &value
		}
	}

	if len(p.IPFamilies) > 0 {
		seen := make(map[corev1.IPFamily]struct{}, len(p.IPFamilies))
		normalizedFamilies := make([]corev1.IPFamily, 0, len(p.IPFamilies))
		for _, item := range p.IPFamilies {
			item = corev1.IPFamily(strings.TrimSpace(string(item)))
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			normalizedFamilies = append(normalizedFamilies, item)
		}
		sort.Slice(normalizedFamilies, func(i, j int) bool {
			return normalizedFamilies[i] < normalizedFamilies[j]
		})
		p.IPFamilies = normalizedFamilies
	}

	if p.LoadBalancerIP != nil {
		value := strings.TrimSpace(*p.LoadBalancerIP)
		if value == "" {
			p.LoadBalancerIP = nil
		} else {
			p.LoadBalancerIP = &value
		}
	}

	if p.LoadBalancerClass != nil {
		value := strings.TrimSpace(*p.LoadBalancerClass)
		if value == "" {
			p.LoadBalancerClass = nil
		} else {
			p.LoadBalancerClass = &value
		}
	}

	if len(p.ExternalIPs) > 0 {
		seen := make(map[string]struct{}, len(p.ExternalIPs))
		normalizedIPs := make([]string, 0, len(p.ExternalIPs))
		for _, item := range p.ExternalIPs {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			normalizedIPs = append(normalizedIPs, item)
		}
		sort.Strings(normalizedIPs)
		p.ExternalIPs = normalizedIPs
	}

	if len(p.LoadBalancerSourceRanges) == 0 {
		return
	}

	seen := make(map[string]struct{}, len(p.LoadBalancerSourceRanges))
	normalized := make([]string, 0, len(p.LoadBalancerSourceRanges))
	for _, item := range p.LoadBalancerSourceRanges {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		normalized = append(normalized, item)
	}
	sort.Strings(normalized)
	p.LoadBalancerSourceRanges = normalized
}

func (p gatewayServiceParameters) validate() error {
	switch p.Type {
	case "", corev1.ServiceTypeClusterIP, corev1.ServiceTypeNodePort, corev1.ServiceTypeLoadBalancer:
	default:
		return fmt.Errorf("unsupported service type %q", p.Type)
	}

	if p.ExternalTrafficPolicy != nil {
		switch *p.ExternalTrafficPolicy {
		case corev1.ServiceExternalTrafficPolicyCluster, corev1.ServiceExternalTrafficPolicyLocal:
		default:
			return fmt.Errorf("unsupported externalTrafficPolicy %q", *p.ExternalTrafficPolicy)
		}
		if p.Type != "" && p.Type != corev1.ServiceTypeNodePort && p.Type != corev1.ServiceTypeLoadBalancer {
			return fmt.Errorf("externalTrafficPolicy requires service type NodePort or LoadBalancer")
		}
	}

	if p.InternalTrafficPolicy != nil {
		switch *p.InternalTrafficPolicy {
		case corev1.ServiceInternalTrafficPolicyCluster, corev1.ServiceInternalTrafficPolicyLocal:
		default:
			return fmt.Errorf("unsupported internalTrafficPolicy %q", *p.InternalTrafficPolicy)
		}
	}

	if p.IPFamilyPolicy != nil {
		switch *p.IPFamilyPolicy {
		case corev1.IPFamilyPolicySingleStack, corev1.IPFamilyPolicyPreferDualStack, corev1.IPFamilyPolicyRequireDualStack:
		default:
			return fmt.Errorf("unsupported ipFamilyPolicy %q", *p.IPFamilyPolicy)
		}
	}

	for _, family := range p.IPFamilies {
		switch family {
		case corev1.IPv4Protocol, corev1.IPv6Protocol:
		default:
			return fmt.Errorf("unsupported ipFamily %q", family)
		}
	}

	if p.SessionAffinity != nil {
		switch *p.SessionAffinity {
		case corev1.ServiceAffinityNone, corev1.ServiceAffinityClientIP:
		default:
			return fmt.Errorf("unsupported sessionAffinity %q", *p.SessionAffinity)
		}
	}
	if p.SessionAffinityConfig != nil {
		if p.SessionAffinity == nil || *p.SessionAffinity != corev1.ServiceAffinityClientIP {
			return fmt.Errorf("sessionAffinityConfig requires sessionAffinity ClientIP")
		}
		if p.SessionAffinityConfig.ClientIP == nil || p.SessionAffinityConfig.ClientIP.TimeoutSeconds == nil {
			return fmt.Errorf("sessionAffinityConfig.clientIP.timeoutSeconds is required")
		}
		timeout := *p.SessionAffinityConfig.ClientIP.TimeoutSeconds
		if timeout <= 0 || timeout > 86400 {
			return fmt.Errorf("sessionAffinityConfig.clientIP.timeoutSeconds must be between 1 and 86400")
		}
	}

	if p.LoadBalancerClass != nil && p.Type != "" && p.Type != corev1.ServiceTypeLoadBalancer {
		return fmt.Errorf("loadBalancerClass requires service type LoadBalancer")
	}
	if p.LoadBalancerIP != nil && p.Type != "" && p.Type != corev1.ServiceTypeLoadBalancer {
		return fmt.Errorf("loadBalancerIP requires service type LoadBalancer")
	}
	if p.HealthCheckNodePort != nil {
		if *p.HealthCheckNodePort <= 0 {
			return fmt.Errorf("healthCheckNodePort must be greater than 0")
		}
		if p.Type != "" && p.Type != corev1.ServiceTypeLoadBalancer {
			return fmt.Errorf("healthCheckNodePort requires service type LoadBalancer")
		}
		if p.ExternalTrafficPolicy == nil || *p.ExternalTrafficPolicy != corev1.ServiceExternalTrafficPolicyLocal {
			return fmt.Errorf("healthCheckNodePort requires externalTrafficPolicy Local")
		}
	}
	if len(p.LoadBalancerSourceRanges) > 0 && p.Type != "" && p.Type != corev1.ServiceTypeLoadBalancer {
		return fmt.Errorf("loadBalancerSourceRanges requires service type LoadBalancer")
	}
	if p.AllocateLoadBalancerNodePorts != nil && p.Type != "" && p.Type != corev1.ServiceTypeLoadBalancer {
		return fmt.Errorf("allocateLoadBalancerNodePorts requires service type LoadBalancer")
	}

	return nil
}

func applyGatewayServiceParameters(spec *corev1.ServiceSpec, params gatewayServiceParameters) {
	if params.Type != "" {
		spec.Type = params.Type
	}

	spec.SessionAffinity = corev1.ServiceAffinityNone
	if params.SessionAffinity != nil {
		spec.SessionAffinity = *params.SessionAffinity
	}
	spec.SessionAffinityConfig = nil
	if params.SessionAffinityConfig != nil && params.SessionAffinityConfig.ClientIP != nil {
		timeout := *params.SessionAffinityConfig.ClientIP.TimeoutSeconds
		spec.SessionAffinityConfig = &corev1.SessionAffinityConfig{
			ClientIP: &corev1.ClientIPConfig{TimeoutSeconds: &timeout},
		}
	}

	spec.ExternalTrafficPolicy = ""
	if spec.Type == corev1.ServiceTypeNodePort || spec.Type == corev1.ServiceTypeLoadBalancer {
		spec.ExternalTrafficPolicy = corev1.ServiceExternalTrafficPolicyCluster
	}
	if params.ExternalTrafficPolicy != nil {
		spec.ExternalTrafficPolicy = *params.ExternalTrafficPolicy
	}

	spec.InternalTrafficPolicy = nil
	if params.InternalTrafficPolicy != nil {
		value := *params.InternalTrafficPolicy
		spec.InternalTrafficPolicy = &value
	}

	spec.IPFamilyPolicy = nil
	if params.IPFamilyPolicy != nil {
		value := *params.IPFamilyPolicy
		spec.IPFamilyPolicy = &value
	}
	spec.IPFamilies = append([]corev1.IPFamily(nil), params.IPFamilies...)

	spec.PublishNotReadyAddresses = false
	if params.PublishNotReadyAddresses != nil {
		spec.PublishNotReadyAddresses = *params.PublishNotReadyAddresses
	}

	spec.LoadBalancerClass = nil
	if params.LoadBalancerClass != nil {
		value := *params.LoadBalancerClass
		spec.LoadBalancerClass = &value
	}

	spec.ExternalIPs = append([]string(nil), params.ExternalIPs...)
	spec.LoadBalancerIP = ""
	if params.LoadBalancerIP != nil {
		spec.LoadBalancerIP = *params.LoadBalancerIP
	}
	spec.HealthCheckNodePort = 0
	if params.HealthCheckNodePort != nil {
		spec.HealthCheckNodePort = *params.HealthCheckNodePort
	}

	spec.LoadBalancerSourceRanges = append([]string(nil), params.LoadBalancerSourceRanges...)

	spec.AllocateLoadBalancerNodePorts = nil
	if params.AllocateLoadBalancerNodePorts != nil {
		value := *params.AllocateLoadBalancerNodePorts
		spec.AllocateLoadBalancerNodePorts = &value
	}
}

func mergeGatewayServiceParameters(base, override gatewayServiceParameters) gatewayServiceParameters {
	out := cloneGatewayServiceParameters(base)

	if override.Type != "" {
		out.Type = override.Type
	}
	if override.ExternalTrafficPolicy != nil {
		value := *override.ExternalTrafficPolicy
		out.ExternalTrafficPolicy = &value
	}
	if override.InternalTrafficPolicy != nil {
		value := *override.InternalTrafficPolicy
		out.InternalTrafficPolicy = &value
	}
	if override.IPFamilyPolicy != nil {
		value := *override.IPFamilyPolicy
		out.IPFamilyPolicy = &value
	}
	if override.IPFamilies != nil {
		out.IPFamilies = append([]corev1.IPFamily(nil), override.IPFamilies...)
	}
	if override.SessionAffinity != nil {
		value := *override.SessionAffinity
		out.SessionAffinity = &value
	}
	if override.SessionAffinityConfig != nil {
		out.SessionAffinityConfig = cloneSessionAffinityConfigParameters(override.SessionAffinityConfig)
	}
	if override.LoadBalancerClass != nil {
		value := *override.LoadBalancerClass
		out.LoadBalancerClass = &value
	}
	if override.ExternalIPs != nil {
		out.ExternalIPs = append([]string(nil), override.ExternalIPs...)
	}
	if override.LoadBalancerIP != nil {
		value := *override.LoadBalancerIP
		out.LoadBalancerIP = &value
	}
	if override.HealthCheckNodePort != nil {
		value := *override.HealthCheckNodePort
		out.HealthCheckNodePort = &value
	}
	if override.LoadBalancerSourceRanges != nil {
		out.LoadBalancerSourceRanges = append([]string(nil), override.LoadBalancerSourceRanges...)
	}
	if override.AllocateLoadBalancerNodePorts != nil {
		value := *override.AllocateLoadBalancerNodePorts
		out.AllocateLoadBalancerNodePorts = &value
	}
	if override.PublishNotReadyAddresses != nil {
		value := *override.PublishNotReadyAddresses
		out.PublishNotReadyAddresses = &value
	}

	return out
}

func cloneGatewayServiceParameters(in gatewayServiceParameters) gatewayServiceParameters {
	out := gatewayServiceParameters{
		Type:                     in.Type,
		IPFamilies:               append([]corev1.IPFamily(nil), in.IPFamilies...),
		ExternalIPs:              append([]string(nil), in.ExternalIPs...),
		LoadBalancerSourceRanges: append([]string(nil), in.LoadBalancerSourceRanges...),
	}
	if in.ExternalTrafficPolicy != nil {
		value := *in.ExternalTrafficPolicy
		out.ExternalTrafficPolicy = &value
	}
	if in.InternalTrafficPolicy != nil {
		value := *in.InternalTrafficPolicy
		out.InternalTrafficPolicy = &value
	}
	if in.IPFamilyPolicy != nil {
		value := *in.IPFamilyPolicy
		out.IPFamilyPolicy = &value
	}
	if in.SessionAffinity != nil {
		value := *in.SessionAffinity
		out.SessionAffinity = &value
	}
	if in.SessionAffinityConfig != nil {
		out.SessionAffinityConfig = cloneSessionAffinityConfigParameters(in.SessionAffinityConfig)
	}
	if in.LoadBalancerClass != nil {
		value := *in.LoadBalancerClass
		out.LoadBalancerClass = &value
	}
	if in.LoadBalancerIP != nil {
		value := *in.LoadBalancerIP
		out.LoadBalancerIP = &value
	}
	if in.HealthCheckNodePort != nil {
		value := *in.HealthCheckNodePort
		out.HealthCheckNodePort = &value
	}
	if in.AllocateLoadBalancerNodePorts != nil {
		value := *in.AllocateLoadBalancerNodePorts
		out.AllocateLoadBalancerNodePorts = &value
	}
	if in.PublishNotReadyAddresses != nil {
		value := *in.PublishNotReadyAddresses
		out.PublishNotReadyAddresses = &value
	}
	return out
}

func cloneSessionAffinityConfigParameters(in *sessionAffinityConfigParameters) *sessionAffinityConfigParameters {
	if in == nil {
		return nil
	}
	out := &sessionAffinityConfigParameters{}
	if in.ClientIP != nil {
		out.ClientIP = &clientIPConfigParameters{}
		if in.ClientIP.TimeoutSeconds != nil {
			value := *in.ClientIP.TimeoutSeconds
			out.ClientIP.TimeoutSeconds = &value
		}
	}
	return out
}
