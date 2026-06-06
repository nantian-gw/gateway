package translator

import (
	"time"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/extensionfilter"
	"github.com/nantian-gw/gateway/internal/ir"
)

func filtersFromHTTP(filters []gatewayv1.HTTPRouteFilter, defaultNamespace string) []ir.Filter {
	return filtersFromHTTPWithResolver(
		filters,
		defaultNamespace,
		extensionfilter.Resolver{},
		extensionfilter.TargetHTTP,
		nil,
		0,
	)
}

func filtersFromHTTPWithResolver(
	filters []gatewayv1.HTTPRouteFilter,
	defaultNamespace string,
	resolver extensionfilter.Resolver,
	target extensionfilter.Target,
	rawFilterConfigs rawHTTPRouteFilterConfigs,
	ruleIndex int,
) []ir.Filter {
	out := make([]ir.Filter, 0, len(filters))
	for filterIndex, filter := range filters {
		item := ir.Filter{Type: string(filter.Type)}
		rawFilterConfig := rawHTTPFilterConfig(rawFilterConfigs, ruleIndex, filterIndex)
		switch filter.Type {
		case gatewayv1.HTTPRouteFilterRequestHeaderModifier:
			item.Config = headerFilterConfig(filter.RequestHeaderModifier)
		case gatewayv1.HTTPRouteFilterResponseHeaderModifier:
			item.Config = headerFilterConfig(filter.ResponseHeaderModifier)
		case gatewayv1.HTTPRouteFilterRequestRedirect:
			item.Config = requestRedirectConfig(filter.RequestRedirect)
		case gatewayv1.HTTPRouteFilterURLRewrite:
			item.Config = urlRewriteConfig(filter.URLRewrite)
		case gatewayv1.HTTPRouteFilterType("CORS"):
			item.Config = corsFilterConfig(rawFilterConfig)
		case gatewayv1.HTTPRouteFilterRequestMirror:
			item.Config = requestMirrorConfig(filter.RequestMirror, defaultNamespace)
		case gatewayv1.HTTPRouteFilterExternalAuth:
			item.Config = externalAuthConfig(filter.ExternalAuth, defaultNamespace)
		case gatewayv1.HTTPRouteFilterExtensionRef:
			resolved := resolver.Resolve(extensionfilter.RefFromLocalRef(defaultNamespace, filter.ExtensionRef), target)
			item.Type = resolved.Type
			item.Config = resolved.Config
		}
		out = append(out, item)
	}
	return out
}

func filtersFromGRPC(filters []gatewayv1.GRPCRouteFilter, defaultNamespace string) []ir.Filter {
	return filtersFromGRPCWithResolver(filters, defaultNamespace, extensionfilter.Resolver{}, extensionfilter.TargetGRPC)
}

func filtersFromGRPCWithResolver(
	filters []gatewayv1.GRPCRouteFilter,
	defaultNamespace string,
	resolver extensionfilter.Resolver,
	target extensionfilter.Target,
) []ir.Filter {
	out := make([]ir.Filter, 0, len(filters))
	for _, filter := range filters {
		item := ir.Filter{Type: string(filter.Type)}
		switch filter.Type {
		case gatewayv1.GRPCRouteFilterRequestHeaderModifier:
			item.Config = headerFilterConfig(filter.RequestHeaderModifier)
		case gatewayv1.GRPCRouteFilterResponseHeaderModifier:
			item.Config = headerFilterConfig(filter.ResponseHeaderModifier)
		case gatewayv1.GRPCRouteFilterRequestMirror:
			item.Config = requestMirrorConfig(filter.RequestMirror, defaultNamespace)
		case gatewayv1.GRPCRouteFilterExtensionRef:
			resolved := resolver.Resolve(extensionfilter.RefFromLocalRef(defaultNamespace, filter.ExtensionRef), target)
			item.Type = resolved.Type
			item.Config = resolved.Config
		}
		out = append(out, item)
	}
	return out
}

func headerFilterConfig(filter *gatewayv1.HTTPHeaderFilter) map[string]any {
	if filter == nil {
		return nil
	}

	config := map[string]any{}
	if set := headersToConfig(filter.Set); len(set) > 0 {
		config["set"] = set
	}
	if add := headersToConfig(filter.Add); len(add) > 0 {
		config["add"] = add
	}
	if len(filter.Remove) > 0 {
		remove := make([]any, 0, len(filter.Remove))
		for _, name := range filter.Remove {
			remove = append(remove, name)
		}
		config["remove"] = remove
	}
	if len(config) == 0 {
		return nil
	}

	return config
}

func requestRedirectConfig(filter *gatewayv1.HTTPRequestRedirectFilter) map[string]any {
	if filter == nil {
		return nil
	}

	config := map[string]any{}
	if filter.Scheme != nil {
		config["scheme"] = *filter.Scheme
	}
	if filter.Hostname != nil {
		config["hostname"] = string(*filter.Hostname)
	}
	if filter.Path != nil {
		config["path"] = pathModifierConfig(filter.Path)
	}
	if filter.Port != nil {
		config["port"] = int(*filter.Port)
	}
	if filter.StatusCode != nil {
		config["statusCode"] = *filter.StatusCode
	}
	if len(config) == 0 {
		return nil
	}

	return config
}

func urlRewriteConfig(filter *gatewayv1.HTTPURLRewriteFilter) map[string]any {
	if filter == nil {
		return nil
	}

	config := map[string]any{}
	if filter.Hostname != nil {
		config["hostname"] = string(*filter.Hostname)
	}
	if filter.Path != nil {
		config["path"] = pathModifierConfig(filter.Path)
	}
	if len(config) == 0 {
		return nil
	}

	return config
}

func requestMirrorConfig(filter *gatewayv1.HTTPRequestMirrorFilter, defaultNamespace string) map[string]any {
	if filter == nil {
		return nil
	}

	backendRef := map[string]any{
		"namespace": namespaceOrDefault(filter.BackendRef.Namespace, defaultNamespace),
		"name":      string(filter.BackendRef.Name),
		"port":      int(portValue(filter.BackendRef.Port)),
	}
	if group := stringValue(filter.BackendRef.Group); group != "" {
		backendRef["group"] = group
	}
	if kind := stringValue(filter.BackendRef.Kind); kind != "" {
		backendRef["kind"] = kind
	}

	config := map[string]any{
		"backendRef": backendRef,
	}
	if filter.Percent != nil {
		config["percent"] = int(*filter.Percent)
	}
	if filter.Fraction != nil {
		denominator := int32(100)
		if filter.Fraction.Denominator != nil {
			denominator = *filter.Fraction.Denominator
		}
		config["fraction"] = map[string]any{
			"numerator":   int(filter.Fraction.Numerator),
			"denominator": int(denominator),
		}
	}

	return config
}

func externalAuthConfig(filter *gatewayv1.HTTPExternalAuthFilter, defaultNamespace string) map[string]any {
	if filter == nil {
		return nil
	}

	backendRef := map[string]any{
		"namespace": namespaceOrDefault(filter.BackendRef.Namespace, defaultNamespace),
		"name":      string(filter.BackendRef.Name),
		"port":      int(portValue(filter.BackendRef.Port)),
	}
	if group := stringValue(filter.BackendRef.Group); group != "" {
		backendRef["group"] = group
	}
	if kind := stringValue(filter.BackendRef.Kind); kind != "" {
		backendRef["kind"] = kind
	}

	config := map[string]any{
		"protocol":   string(filter.ExternalAuthProtocol),
		"backendRef": backendRef,
	}
	if httpConfig := externalHTTPAuthConfig(filter.HTTPAuthConfig); len(httpConfig) > 0 {
		config["http"] = httpConfig
	}
	if grpcConfig := externalGRPCAuthConfig(filter.GRPCAuthConfig); len(grpcConfig) > 0 {
		config["grpc"] = grpcConfig
	}
	if filter.ForwardBody != nil && filter.ForwardBody.MaxSize > 0 {
		config["forwardBodyMaxSize"] = int(filter.ForwardBody.MaxSize)
	}
	return config
}

func externalHTTPAuthConfig(filter *gatewayv1.HTTPAuthConfig) map[string]any {
	if filter == nil {
		return nil
	}

	config := map[string]any{}
	if filter.Path != "" {
		config["path"] = filter.Path
	}
	if allowedHeaders := stringsToConfig(filter.AllowedRequestHeaders); len(allowedHeaders) > 0 {
		config["allowedHeaders"] = allowedHeaders
	}
	if allowedResponseHeaders := stringsToConfig(filter.AllowedResponseHeaders); len(allowedResponseHeaders) > 0 {
		config["allowedResponseHeaders"] = allowedResponseHeaders
	}
	return config
}

func externalGRPCAuthConfig(filter *gatewayv1.GRPCAuthConfig) map[string]any {
	if filter == nil {
		return nil
	}

	config := map[string]any{}
	if allowedHeaders := stringsToConfig(filter.AllowedRequestHeaders); len(allowedHeaders) > 0 {
		config["allowedHeaders"] = allowedHeaders
	}
	return config
}

func stringsToConfig(items []string) []any {
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func corsFilterConfig(rawFilter map[string]any) map[string]any {
	filter := cloneStringAnyMap(rawFilter["cors"])
	if len(filter) == 0 {
		return nil
	}

	config := map[string]any{}
	if allowOrigins := stringListFilterConfig(filter["allowOrigins"]); len(allowOrigins) > 0 {
		config["allowOrigins"] = allowOrigins
	}
	if allowMethods := stringListFilterConfig(filter["allowMethods"]); len(allowMethods) > 0 {
		config["allowMethods"] = allowMethods
	}
	if allowHeaders := stringListFilterConfig(filter["allowHeaders"]); len(allowHeaders) > 0 {
		config["allowHeaders"] = allowHeaders
	}
	if exposeHeaders := stringListFilterConfig(filter["exposeHeaders"]); len(exposeHeaders) > 0 {
		config["exposeHeaders"] = exposeHeaders
	}
	if allowCredentials, ok := filter["allowCredentials"].(bool); ok {
		config["allowCredentials"] = allowCredentials
	}
	if maxAge, ok := nonNegativeIntegerFilterConfig(filter["maxAge"]); ok {
		config["maxAge"] = maxAge
	}
	if len(config) == 0 {
		return nil
	}

	return config
}

func stringListFilterConfig(value any) []any {
	rawItems, ok := value.([]any)
	if !ok {
		return nil
	}

	items := make([]any, 0, len(rawItems))
	for _, item := range rawItems {
		if text, ok := item.(string); ok && text != "" {
			items = append(items, text)
		}
	}
	if len(items) == 0 {
		return nil
	}

	return items
}

func nonNegativeIntegerFilterConfig(value any) (int, bool) {
	switch item := value.(type) {
	case int:
		if item >= 0 {
			return item, true
		}
	case int32:
		if item >= 0 {
			return int(item), true
		}
	case int64:
		if item >= 0 {
			return int(item), true
		}
	case float64:
		if item >= 0 && item == float64(int(item)) {
			return int(item), true
		}
	}

	return 0, false
}

func pathModifierConfig(modifier *gatewayv1.HTTPPathModifier) map[string]any {
	if modifier == nil {
		return nil
	}

	config := map[string]any{
		"type": string(modifier.Type),
	}
	if modifier.ReplaceFullPath != nil {
		config["replaceFullPath"] = *modifier.ReplaceFullPath
	}
	if modifier.ReplacePrefixMatch != nil {
		config["replacePrefixMatch"] = *modifier.ReplacePrefixMatch
	}

	return config
}

func headersToConfig(headers []gatewayv1.HTTPHeader) []any {
	out := make([]any, 0, len(headers))
	for _, header := range headers {
		out = append(out, map[string]any{
			"name":  string(header.Name),
			"value": header.Value,
		})
	}

	return out
}

func backendRefsFromHTTP(refs []gatewayv1.HTTPBackendRef, defaultNamespace string) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ir.BackendRef{
			Group:     stringValue(ref.BackendRef.Group),
			Kind:      stringValue(ref.BackendRef.Kind),
			Namespace: namespaceOrDefault(ref.BackendRef.Namespace, defaultNamespace),
			Name:      string(ref.BackendRef.Name),
			Port:      portValue(ref.BackendRef.Port),
			Weight:    uint32(weightValue(ref.Weight)),
			Filters:   filtersFromHTTP(ref.Filters, defaultNamespace),
		})
	}

	return out
}

func backendRefsFromGRPC(refs []gatewayv1.GRPCBackendRef, defaultNamespace string) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ir.BackendRef{
			Group:     stringValue(ref.BackendRef.Group),
			Kind:      stringValue(ref.BackendRef.Kind),
			Namespace: namespaceOrDefault(ref.BackendRef.Namespace, defaultNamespace),
			Name:      string(ref.BackendRef.Name),
			Port:      portValue(ref.BackendRef.Port),
			Weight:    uint32(weightValue(ref.Weight)),
			Filters:   filtersFromGRPC(ref.Filters, defaultNamespace),
		})
	}

	return out
}

func backendRefsFromRouteRule(refs []gatewayv1.BackendRef, defaultNamespace string) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ir.BackendRef{
			Group:     stringValue(ref.Group),
			Kind:      stringValue(ref.Kind),
			Namespace: namespaceOrDefault(ref.Namespace, defaultNamespace),
			Name:      string(ref.Name),
			Port:      portValue(ref.Port),
			Weight:    uint32(weightValue(ref.Weight)),
		})
	}

	return out
}

var _ = time.Second
