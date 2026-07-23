package routes

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type RawHTTPRouteFilterConfigs [][]map[string]any

func LoadHTTPRouteRawFilterConfigs(
	ctx context.Context,
	cl client.Client,
	routes []gatewayv1.HTTPRoute,
) (map[client.ObjectKey]RawHTTPRouteFilterConfigs, error) {
	keys := httpRoutesNeedingRawFilterConfigs(routes)
	if len(keys) == 0 {
		return nil, nil
	}

	out := make(map[client.ObjectKey]RawHTTPRouteFilterConfigs, len(keys))
	for _, key := range keys {
		item := &unstructured.Unstructured{}
		item.SetAPIVersion("gateway.networking.k8s.io/v1")
		item.SetKind("HTTPRoute")
		if err := cl.Get(ctx, key, item); err != nil {
			return nil, err
		}
		out[key] = rawHTTPRouteFilterConfigsFromObject(item.Object)
	}

	return out, nil
}

func httpRoutesNeedingRawFilterConfigs(routes []gatewayv1.HTTPRoute) []client.ObjectKey {
	keys := make([]client.ObjectKey, 0)
	for index := range routes {
		if !httpRouteNeedsRawFilterConfigs(routes[index]) {
			continue
		}
		keys = append(keys, client.ObjectKeyFromObject(&routes[index]))
	}
	return keys
}

func httpRouteNeedsRawFilterConfigs(route gatewayv1.HTTPRoute) bool {
	for _, rule := range route.Spec.Rules {
		for _, filter := range rule.Filters {
			if httpRouteFilterNeedsRawConfig(filter) {
				return true
			}
		}
	}
	return false
}

func httpRouteFilterNeedsRawConfig(filter gatewayv1.HTTPRouteFilter) bool {
	switch filter.Type {
	case gatewayv1.HTTPRouteFilterType("CORS"):
		return true
	default:
		return false
	}
}

func rawHTTPRouteFilterConfigsFromObject(object map[string]any) RawHTTPRouteFilterConfigs {
	rules, exists, err := unstructured.NestedSlice(object, "spec", "rules")
	if err != nil {
		return nil
	}
	if !exists {
		return nil
	}
	routeConfigs := make(RawHTTPRouteFilterConfigs, 0, len(rules))
	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]any)
		if !ok {
			routeConfigs = append(routeConfigs, nil)
			continue
		}

		filters, ok := nestedMapSlice(ruleMap, "filters")
		if !ok {
			routeConfigs = append(routeConfigs, nil)
			continue
		}
		ruleConfigs := make([]map[string]any, 0, len(filters))
		ruleConfigs = append(ruleConfigs, filters...)
		routeConfigs = append(routeConfigs, ruleConfigs)
	}
	return routeConfigs
}

func RawHTTPFilterConfig(
	configs RawHTTPRouteFilterConfigs,
	ruleIndex int,
	filterIndex int,
) map[string]any {
	if ruleIndex >= len(configs) || filterIndex >= len(configs[ruleIndex]) {
		return nil
	}
	return configs[ruleIndex][filterIndex]
}

func nestedMapSlice(object map[string]any, field string) ([]map[string]any, bool) {
	value, ok := object[field]
	if !ok {
		return nil, false
	}
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if mapped := cloneStringAnyMap(item); mapped != nil {
			out = append(out, mapped)
		} else {
			out = append(out, nil)
		}
	}
	return out, true
}

func cloneStringAnyMap(value any) map[string]any {
	items, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]any, len(items))
	for key, raw := range items {
		out[key] = raw
	}
	return out
}
