package gwapi

import (
	"fmt"
	"strconv"
	"strings"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type HTTPRouteRuleValidationSummary struct {
	InvalidRuleIndexes  []int
	invalidRuleMessages []string
	acceptedRuleIndexes []int
	acceptedMessages    []string
}

func ValidateHTTPRouteRules(route gatewayv1.HTTPRoute) HTTPRouteRuleValidationSummary {
	out := HTTPRouteRuleValidationSummary{
		InvalidRuleIndexes:  make([]int, 0, len(route.Spec.Rules)),
		invalidRuleMessages: make([]string, 0, len(route.Spec.Rules)),
		acceptedRuleIndexes: make([]int, 0, len(route.Spec.Rules)),
		acceptedMessages:    make([]string, 0, len(route.Spec.Rules)),
	}

	for index, rule := range route.Spec.Rules {
		if filterType, ok := HTTPRuleHasUnsupportedFilter(rule); ok {
			out.addInvalidRule(index, fmt.Sprintf("uses unsupported %s filter", filterType), true)
			continue
		}
		if HTTPRuleHasRedirectAndRewrite(rule.Filters) {
			out.addInvalidRule(index, "must not combine RequestRedirect and URLRewrite filters", false)
		}
	}

	return out
}

func (s *HTTPRouteRuleValidationSummary) addInvalidRule(index int, message string, acceptedError bool) {
	s.InvalidRuleIndexes = append(s.InvalidRuleIndexes, index)
	s.invalidRuleMessages = append(s.invalidRuleMessages, message)
	if acceptedError {
		s.acceptedRuleIndexes = append(s.acceptedRuleIndexes, index)
		s.acceptedMessages = append(s.acceptedMessages, message)
	}
}

func HTTPRuleHasRedirectAndRewrite(filters []gatewayv1.HTTPRouteFilter) bool {
	var hasRedirect bool
	var hasRewrite bool

	for _, filter := range filters {
		switch filter.Type {
		case gatewayv1.HTTPRouteFilterRequestRedirect:
			hasRedirect = true
		case gatewayv1.HTTPRouteFilterURLRewrite:
			hasRewrite = true
		}
	}

	return hasRedirect && hasRewrite
}

func HTTPRuleHasUnsupportedFilter(rule gatewayv1.HTTPRouteRule) (gatewayv1.HTTPRouteFilterType, bool) {
	if filterType, ok := httpFiltersHaveUnsupportedFilter(rule.Filters, true); ok {
		return filterType, true
	}
	for _, backendRef := range rule.BackendRefs {
		if filterType, ok := httpFiltersHaveUnsupportedFilter(backendRef.Filters, true); ok {
			return filterType, true
		}
	}
	return "", false
}

func httpFiltersHaveUnsupportedFilter(filters []gatewayv1.HTTPRouteFilter, allowExternalAuth bool) (gatewayv1.HTTPRouteFilterType, bool) {
	for _, filter := range filters {
		if httpRouteFilterSupported(filter, allowExternalAuth) {
			continue
		}
		return filter.Type, true
	}
	return "", false
}

func httpRouteFilterSupported(filter gatewayv1.HTTPRouteFilter, allowExternalAuth bool) bool {
	switch filter.Type {
	case gatewayv1.HTTPRouteFilterRequestHeaderModifier,
		gatewayv1.HTTPRouteFilterResponseHeaderModifier,
		gatewayv1.HTTPRouteFilterRequestMirror,
		gatewayv1.HTTPRouteFilterRequestRedirect,
		gatewayv1.HTTPRouteFilterURLRewrite,
		gatewayv1.HTTPRouteFilterCORS,
		gatewayv1.HTTPRouteFilterExtensionRef:
		return true
	case gatewayv1.HTTPRouteFilterExternalAuth:
		return allowExternalAuth && supportedHTTPExternalAuthFilter(filter.ExternalAuth)
	default:
		return false
	}
}

func supportedHTTPExternalAuthFilter(filter *gatewayv1.HTTPExternalAuthFilter) bool {
	if filter == nil {
		return false
	}
	if filter.ExternalAuthProtocol == gatewayv1.HTTPRouteExternalAuthGRPCProtocol {
		return false
	}
	if filter.ExternalAuthProtocol != gatewayv1.HTTPRouteExternalAuthHTTPProtocol {
		return false
	}
	if filter.ForwardBody != nil && filter.ForwardBody.MaxSize > 0 {
		return false
	}
	return true
}

func externalAuthFilterRejectionReason(filter *gatewayv1.HTTPExternalAuthFilter) string {
	if filter == nil {
		return "nil ExternalAuth filter"
	}
	if filter.ExternalAuthProtocol == gatewayv1.HTTPRouteExternalAuthGRPCProtocol {
		return "GRPC ExtAuth protocol is not yet supported (only HTTP ExtAuth)"
	}
	if filter.ExternalAuthProtocol != gatewayv1.HTTPRouteExternalAuthHTTPProtocol {
		return fmt.Sprintf("unsupported ExtAuth protocol %q (only HTTP ExtAuth supported)", filter.ExternalAuthProtocol)
	}
	if filter.ForwardBody != nil && filter.ForwardBody.MaxSize > 0 {
		return "ExternalAuth forwardBody.maxSize is not yet supported"
	}
	return "unsupported ExternalAuth configuration"
}

func (s HTTPRouteRuleValidationSummary) FullyInvalid(totalRules int) bool {
	return totalRules > 0 && len(s.InvalidRuleIndexes) == totalRules
}

func (s HTTPRouteRuleValidationSummary) PartiallyInvalid(totalRules int) bool {
	return len(s.InvalidRuleIndexes) > 0 && len(s.InvalidRuleIndexes) < totalRules
}

func (s HTTPRouteRuleValidationSummary) InvalidRuleMessage() string {
	if len(s.InvalidRuleIndexes) == 0 {
		return ""
	}

	return httpRouteRuleMessage("HTTPRoute", s.InvalidRuleIndexes, s.invalidRuleMessages)
}

func (s HTTPRouteRuleValidationSummary) AcceptedErrorMessage() string {
	if len(s.acceptedRuleIndexes) == 0 {
		return ""
	}

	return httpRouteRuleMessage("HTTPRoute", s.acceptedRuleIndexes, s.acceptedMessages)
}

func (s HTTPRouteRuleValidationSummary) DroppedRulesMessage() string {
	if len(s.InvalidRuleIndexes) == 0 {
		return ""
	}

	return fmt.Sprintf(
		"Dropped %s %s because %s",
		httpRouteDroppedRuleNoun(len(s.InvalidRuleIndexes)),
		httpRouteRuleNumbers(s.InvalidRuleIndexes),
		s.InvalidRuleMessage(),
	)
}

func httpRouteRuleMessage(prefix string, indexes []int, messages []string) string {
	if len(indexes) == 0 {
		return ""
	}
	if len(indexes) == 1 && len(messages) == 1 {
		return fmt.Sprintf("%s rule %d %s", prefix, indexes[0]+1, messages[0])
	}
	if len(messages) == len(indexes) && allStringsEqual(messages) {
		return fmt.Sprintf(
			"%s %s %s %s",
			prefix,
			httpRouteRuleNoun(len(indexes)),
			httpRouteRuleNumbers(indexes),
			messages[0],
		)
	}

	parts := make([]string, 0, len(indexes))
	for index, ruleIndex := range indexes {
		message := "is invalid"
		if index < len(messages) && messages[index] != "" {
			message = messages[index]
		}
		parts = append(parts, fmt.Sprintf("rule %d %s", ruleIndex+1, message))
	}
	return fmt.Sprintf(
		"%s %s %s are invalid: %s",
		prefix,
		httpRouteRuleNoun(len(indexes)),
		httpRouteRuleNumbers(indexes),
		strings.Join(parts, "; "),
	)
}

func allStringsEqual(items []string) bool {
	if len(items) == 0 {
		return true
	}
	first := items[0]
	for _, item := range items[1:] {
		if item != first {
			return false
		}
	}
	return true
}

func httpRouteRuleNoun(count int) string {
	if count == 1 {
		return "rule"
	}
	return "rules"
}

func httpRouteDroppedRuleNoun(count int) string {
	if count == 1 {
		return "Rule"
	}
	return "Rules"
}

func httpRouteRuleNumbers(indexes []int) string {
	if len(indexes) == 0 {
		return ""
	}

	out := make([]string, 0, len(indexes))
	for _, index := range indexes {
		out = append(out, strconv.Itoa(index+1))
	}
	return strings.Join(out, ", ")
}
