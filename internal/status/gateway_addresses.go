package status

import (
	"fmt"
	"k8s.io/apimachinery/pkg/util/validation"
	"net"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type gatewayAddressEvaluation struct {
	addresses           []gatewayv1.GatewayStatusAddress
	acceptedCondition   conditionSpec
	programmedCondition conditionSpec
}

func evaluateGatewayAddresses(
	specAddresses []gatewayv1.GatewaySpecAddress,
	publishedAddresses []string,
	advertisedAddresses []string,
	generation int64,
) gatewayAddressEvaluation {
	evaluation := gatewayAddressEvaluation{
		addresses: buildStatusAddresses(publishedAddresses),
		acceptedCondition: conditionSpec{
			Type:               string(gatewayv1.GatewayConditionAccepted),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.GatewayReasonAccepted),
			Message:            "Gateway is accepted by nantian-gw",
			ObservedGeneration: generation,
		},
		programmedCondition: conditionSpec{
			Type:               string(gatewayv1.GatewayConditionProgrammed),
			Status:             metav1.ConditionTrue,
			Reason:             string(gatewayv1.GatewayReasonProgrammed),
			Message:            "Gateway is programmed",
			ObservedGeneration: generation,
		},
	}
	if len(specAddresses) == 0 {
		return evaluation
	}

	evaluation.addresses = nil
	resolvedAddresses := make([]gatewayv1.GatewayStatusAddress, 0, len(specAddresses))
	seen := make(map[string]struct{}, len(specAddresses))

	for _, address := range specAddresses {
		addressType := gatewayAddressType(address.Type, address.Value)
		if !supportedGatewayAddressType(addressType) {
			message := fmt.Sprintf(
				"Gateway address %q uses unsupported type %q",
				address.Value,
				addressType,
			)
			evaluation.acceptedCondition.Status = metav1.ConditionFalse
			evaluation.acceptedCondition.Reason = string(gatewayv1.GatewayReasonUnsupportedAddress)
			evaluation.acceptedCondition.Message = message
			evaluation.programmedCondition.Status = metav1.ConditionFalse
			evaluation.programmedCondition.Reason = string(gatewayv1.GatewayReasonInvalid)
			evaluation.programmedCondition.Message = message
			return evaluation
		}
		if strings.TrimSpace(address.Value) == "" {
			statusAddress, ok := assignedGatewayStatusAddress(addressType, publishedAddresses, seen)
			if !ok {
				evaluation.programmedCondition.Status = metav1.ConditionFalse
				evaluation.programmedCondition.Reason = string(gatewayv1.GatewayReasonAddressNotAssigned)
				evaluation.programmedCondition.Message = fmt.Sprintf(
					"Gateway address of type %q could not be assigned by nantian-gw",
					addressType,
				)
				continue
			}

			key := gatewayStatusAddressKey(addressType, statusAddress.Value)
			seen[key] = struct{}{}
			resolvedAddresses = append(resolvedAddresses, statusAddress)
			continue
		}
		if !gatewayAddressValueSupported(addressType, address.Value) {
			message := fmt.Sprintf(
				"Gateway address %q is not a valid %q value",
				address.Value,
				addressType,
			)
			evaluation.acceptedCondition.Status = metav1.ConditionFalse
			evaluation.acceptedCondition.Reason = string(gatewayv1.GatewayReasonUnsupportedAddress)
			evaluation.acceptedCondition.Message = message
			evaluation.programmedCondition.Status = metav1.ConditionFalse
			evaluation.programmedCondition.Reason = string(gatewayv1.GatewayReasonInvalid)
			evaluation.programmedCondition.Message = message
			return evaluation
		}

		statusAddress := gatewayv1.GatewayStatusAddress{
			Type:  &addressType,
			Value: address.Value,
		}
		if !gatewayAddressUsable(statusAddress, advertisedAddresses) {
			evaluation.programmedCondition.Status = metav1.ConditionFalse
			evaluation.programmedCondition.Reason = string(gatewayv1.GatewayReasonAddressNotUsable)
			evaluation.programmedCondition.Message = fmt.Sprintf(
				"Gateway address %q cannot be programmed by nantian-gw",
				address.Value,
			)
			continue
		}

		key := gatewayStatusAddressKey(addressType, address.Value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		resolvedAddresses = append(resolvedAddresses, statusAddress)
	}

	evaluation.addresses = resolvedAddresses
	return evaluation
}

func gatewayAddressType(rawType *gatewayv1.AddressType, value string) gatewayv1.AddressType {
	if rawType != nil && *rawType != "" {
		return *rawType
	}
	if strings.TrimSpace(value) == "" {
		return gatewayv1.IPAddressType
	}
	if net.ParseIP(value) != nil {
		return gatewayv1.IPAddressType
	}
	return gatewayv1.HostnameAddressType
}

func supportedGatewayAddressType(addressType gatewayv1.AddressType) bool {
	return addressType == gatewayv1.IPAddressType || addressType == gatewayv1.HostnameAddressType
}

func gatewayAddressValueSupported(addressType gatewayv1.AddressType, value string) bool {
	switch addressType {
	case gatewayv1.IPAddressType:
		return net.ParseIP(strings.TrimSpace(value)) != nil
	case gatewayv1.HostnameAddressType:
		normalized := normalizeHostnameValue(value)
		return normalized != "" && len(validation.IsDNS1123Subdomain(normalized)) == 0
	default:
		return false
	}
}

func gatewayAddressUsable(address gatewayv1.GatewayStatusAddress, advertisedAddresses []string) bool {
	addressType := gatewayAddressType(address.Type, address.Value)
	switch addressType {
	case gatewayv1.IPAddressType:
		ip := net.ParseIP(strings.TrimSpace(address.Value))
		if ip == nil {
			return false
		}
		if ip.IsUnspecified() {
			return false
		}
		if ip.IsLoopback() {
			return true
		}
		if isDocumentationIP(ip) {
			return false
		}
		if ipInAdvertised(ip, advertisedAddresses) {
			return true
		}
		return true
	case gatewayv1.HostnameAddressType:
		return len(advertisedAddresses) == 0 || hostnameInAdvertised(address.Value, advertisedAddresses)
	default:
		return false
	}
}

func ipInAdvertised(ip net.IP, advertisedAddresses []string) bool {
	for _, adv := range advertisedAddresses {
		advIP := net.ParseIP(strings.TrimSpace(adv))
		if advIP != nil && ip.Equal(advIP) {
			return true
		}
	}
	return false
}

func isDocumentationIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		return false
	}
	// TEST-NET-1: 192.0.2.0/24 (RFC 5737)
	if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
		return true
	}
	// TEST-NET-2: 198.51.100.0/24 (RFC 5737)
	if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
		return true
	}
	// TEST-NET-3: 203.0.113.0/24 (RFC 5737)
	if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
		return true
	}
	return false
}

func hostnameInAdvertised(value string, advertisedAddresses []string) bool {
	normalized := normalizeHostnameValue(value)
	for _, adv := range advertisedAddresses {
		if normalizeHostnameValue(adv) == normalized {
			return true
		}
	}
	return false
}

func assignedGatewayStatusAddress(
	addressType gatewayv1.AddressType,
	advertisedAddresses []string,
	seen map[string]struct{},
) (gatewayv1.GatewayStatusAddress, bool) {
	for _, advertisedAddress := range advertisedAddresses {
		value := strings.TrimSpace(advertisedAddress)
		if value == "" {
			continue
		}
		candidateType := gatewayAddressType(nil, value)
		if candidateType != addressType {
			continue
		}
		key := gatewayStatusAddressKey(candidateType, value)
		if _, exists := seen[key]; exists {
			continue
		}
		return gatewayv1.GatewayStatusAddress{
			Type:  &candidateType,
			Value: value,
		}, true
	}

	return gatewayv1.GatewayStatusAddress{}, false
}

func normalizeHostnameValue(value string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
}

func gatewayStatusAddressKey(addressType gatewayv1.AddressType, value string) string {
	switch addressType {
	case gatewayv1.IPAddressType:
		normalized := strings.TrimSpace(value)
		if ip := net.ParseIP(normalized); ip != nil {
			normalized = ip.String()
		}
		return string(addressType) + "|" + normalized
	case gatewayv1.HostnameAddressType:
		return string(addressType) + "|" + normalizeHostnameValue(value)
	default:
		return string(addressType) + "|" + strings.TrimSpace(value)
	}
}
