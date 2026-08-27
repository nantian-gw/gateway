package routepolicy

import (
	"sort"
	"time"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/constants"
	rp "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	"github.com/nantian-gw/gateway/internal/ir"
)

type routePolicyScope string

const (
	routePolicyScopeNamespace routePolicyScope = "namespace"
	routePolicyScopeGateway   routePolicyScope = "gateway"
	routePolicyScopeRoute     routePolicyScope = "route"
)

var routePolicyScopeOrder = []routePolicyScope{
	routePolicyScopeNamespace,
	routePolicyScopeGateway,
	routePolicyScopeRoute,
}

type translatedRoutePolicy struct {
	namespace string
	name      string
	routeKeys []string
	config    *ir.RoutePolicyConfig
	scope     routePolicyScope
	createdAt time.Time
}

func BuildRoutePolicyIndexes(
	policies []rp.RoutePolicy,
	httpRoutes []ir.HTTPRoute,
	gateways []gatewayv1.Gateway,
) map[string]*ir.RoutePolicyConfig {
	gatewayToRoutes := buildGatewayRouteIndex(httpRoutes, gateways)
	translations := make([]translatedRoutePolicy, 0, len(policies))

	for _, policy := range policies {
		config := translateRoutePolicyDefault(policy.Spec.Default)
		if config == nil {
			continue
		}

		scope, routeKeys, ok := resolveRoutePolicyTargets(policy, httpRoutes, gatewayToRoutes)
		if !ok || len(routeKeys) == 0 {
			continue
		}

		translations = append(translations, translatedRoutePolicy{
			namespace: policy.Namespace,
			name:      policy.Name,
			routeKeys: routeKeys,
			config:    config,
			scope:     scope,
			createdAt: policy.CreationTimestamp.Time,
		})
	}

	// Winner per (routeKey, scope): the policy with the earliest creation
	// timestamp wins. Conflicting policies are no longer dropped as a group;
	// instead the oldest policy is applied deterministically.
	winners := make(map[string]map[routePolicyScope]translatedRoutePolicy)

	for _, scope := range routePolicyScopeOrder {
		for _, tp := range translations {
			if tp.scope != scope {
				continue
			}
			for _, routeKey := range tp.routeKeys {
				if winners[routeKey] == nil {
					winners[routeKey] = make(map[routePolicyScope]translatedRoutePolicy)
				}
				current, exists := winners[routeKey][scope]
				if !exists || routePolicyPrecedes(tp, current) {
					winners[routeKey][scope] = tp
				}
			}
		}
	}

	// Apply winners in scope priority order, merging lower-priority (more
	// specific) scopes over higher-priority (less specific) ones.
	result := make(map[string]*ir.RoutePolicyConfig)

	for _, scope := range routePolicyScopeOrder {
		for routeKey, scopeWinners := range winners {
			tp, ok := scopeWinners[scope]
			if !ok {
				continue
			}

			existing := result[routeKey]
			if existing == nil || scope == routePolicyScopeNamespace {
				result[routeKey] = tp.config
			} else {
				result[routeKey] = mergeRoutePolicyConfig(existing, tp.config)
			}
		}
	}

	return result
}

func routePolicyPrecedes(left, right translatedRoutePolicy) bool {
	if left.createdAt.Before(right.createdAt) {
		return true
	}
	if right.createdAt.Before(left.createdAt) {
		return false
	}
	if left.namespace != right.namespace {
		return left.namespace < right.namespace
	}
	return left.name < right.name
}

func buildGatewayRouteIndex(
	httpRoutes []ir.HTTPRoute,
	gateways []gatewayv1.Gateway,
) map[string]map[string]struct{} {
	gatewaySet := make(map[string]struct{}, len(gateways))
	for _, gw := range gateways {
		key := gw.Namespace + "/" + gw.Name
		gatewaySet[key] = struct{}{}
	}

	index := make(map[string]map[string]struct{})
	for _, route := range httpRoutes {
		routeKey := route.Namespace + "/" + route.Name
		for _, parent := range route.ParentRefs {
			gwKey := parent.Namespace + "/" + parent.Name
			if _, exists := gatewaySet[gwKey]; !exists {
				continue
			}
			if index[gwKey] == nil {
				index[gwKey] = make(map[string]struct{})
			}
			index[gwKey][routeKey] = struct{}{}
		}
	}
	return index
}

func resolveRoutePolicyTargets(
	policy rp.RoutePolicy,
	httpRoutes []ir.HTTPRoute,
	gatewayToRoutes map[string]map[string]struct{},
) (routePolicyScope, []string, bool) {
	if len(policy.Spec.TargetRefs) == 0 {
		routesInNamespace := routeKeysInNamespace(httpRoutes, policy.Namespace)
		return routePolicyScopeNamespace, routesInNamespace, true
	}

	var routeKeys []string
	scope := routePolicyScopeRoute
	seen := make(map[string]struct{})

	for _, targetRef := range policy.Spec.TargetRefs {
		kind := string(targetRef.Kind)
		name := string(targetRef.Name)

		switch kind {
		case constants.KubeGateway:
			scope = routePolicyScopeGateway
			gwKey := policy.Namespace + "/" + name
			if routes, ok := gatewayToRoutes[gwKey]; ok {
				for rk := range routes {
					if _, exists := seen[rk]; !exists {
						seen[rk] = struct{}{}
						routeKeys = append(routeKeys, rk)
					}
				}
			}
		case constants.KubeHTTPRoute, "GRPCRoute":
			rk := policy.Namespace + "/" + name
			if _, exists := seen[rk]; !exists {
				seen[rk] = struct{}{}
				routeKeys = append(routeKeys, rk)
			}
		}
	}

	if len(routeKeys) == 0 {
		return scope, nil, false
	}

	sort.Strings(routeKeys)
	return scope, routeKeys, true
}

func GrpcRoutesToHTTP(routes []ir.GRPCRoute) []ir.HTTPRoute {
	out := make([]ir.HTTPRoute, len(routes))
	for i, r := range routes {
		out[i] = ir.HTTPRoute{
			Name:       r.Name,
			Namespace:  r.Namespace,
			ParentRefs: r.ParentRefs,
		}
	}
	return out
}

func routeKeysInNamespace(httpRoutes []ir.HTTPRoute, namespace string) []string {
	var keys []string
	for _, route := range httpRoutes {
		if route.Namespace == namespace {
			keys = append(keys, route.Namespace+"/"+route.Name)
		}
	}
	sort.Strings(keys)
	return keys
}

func mergeRoutePolicyConfig(parent, child *ir.RoutePolicyConfig) *ir.RoutePolicyConfig {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}

	result := &ir.RoutePolicyConfig{}
	result.Timeout = mergeRouteTimeout(parent.Timeout, child.Timeout)
	result.BodyLimit = mergeRouteBodyLimit(parent.BodyLimit, child.BodyLimit)
	result.Proxy = mergeRouteProxy(parent.Proxy, child.Proxy)
	result.Connection = mergeRouteConnection(parent.Connection, child.Connection)
	return result
}

func mergeRouteTimeout(parent, child *ir.RouteTimeoutConfig) *ir.RouteTimeoutConfig {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	result := *parent
	if child.Request != 0 {
		result.Request = child.Request
	}
	if child.BackendRequest != 0 {
		result.BackendRequest = child.BackendRequest
	}
	if child.Connect != 0 {
		result.Connect = child.Connect
	}
	if child.NextUpstream != 0 {
		result.NextUpstream = child.NextUpstream
	}
	return &result
}

func mergeRouteBodyLimit(parent, child *ir.RouteBodyLimitConfig) *ir.RouteBodyLimitConfig {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	result := *parent
	if child.MaxRequestBodyBytes != 0 {
		result.MaxRequestBodyBytes = child.MaxRequestBodyBytes
	}
	if child.RequestBodyBufferBytes != 0 {
		result.RequestBodyBufferBytes = child.RequestBodyBufferBytes
	}
	if child.MaxRequestHeaderBytes != 0 {
		result.MaxRequestHeaderBytes = child.MaxRequestHeaderBytes
	}
	return &result
}

func mergeRouteProxy(parent, child *ir.RouteProxyConfig) *ir.RouteProxyConfig {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	result := *parent
	if child.RequestBuffering {
		result.RequestBuffering = child.RequestBuffering
	}
	if child.ResponseBuffering {
		result.ResponseBuffering = child.ResponseBuffering
	}
	if child.BufferSize != 0 {
		result.BufferSize = child.BufferSize
	}
	if child.BufferCount != 0 {
		result.BufferCount = child.BufferCount
	}
	return &result
}

func mergeRouteConnection(parent, child *ir.RouteConnectionConfig) *ir.RouteConnectionConfig {
	if child == nil {
		return parent
	}
	if parent == nil {
		return child
	}
	result := *parent
	if child.KeepaliveRequests != 0 {
		result.KeepaliveRequests = child.KeepaliveRequests
	}
	if child.UpstreamKeepalivePoolSize != 0 {
		result.UpstreamKeepalivePoolSize = child.UpstreamKeepalivePoolSize
	}
	if child.KeepaliveTime != 0 {
		result.KeepaliveTime = child.KeepaliveTime
	}
	if child.KeepaliveTimeout != 0 {
		result.KeepaliveTimeout = child.KeepaliveTimeout
	}
	if child.UpstreamKeepaliveIdle != 0 {
		result.UpstreamKeepaliveIdle = child.UpstreamKeepaliveIdle
	}
	return &result
}

func translateRoutePolicyDefault(spec *rp.RoutePolicyDefault) *ir.RoutePolicyConfig {
	if spec == nil {
		return nil
	}

	config := &ir.RoutePolicyConfig{}

	if spec.Timeout != nil {
		config.Timeout = translateTimeoutConfig(spec.Timeout)
	}
	if spec.BodyLimit != nil {
		config.BodyLimit = translateBodyLimitConfig(spec.BodyLimit)
	}
	if spec.Proxy != nil {
		config.Proxy = translateProxyConfig(spec.Proxy)
	}
	if spec.Connection != nil {
		config.Connection = translateConnectionConfig(spec.Connection)
	}

	if config.Timeout == nil && config.BodyLimit == nil && config.Proxy == nil && config.Connection == nil {
		return nil
	}

	return config
}

func translateTimeoutConfig(cfg *rp.TimeoutConfig) *ir.RouteTimeoutConfig {
	result := &ir.RouteTimeoutConfig{}
	hasField := false

	if cfg.Request != nil {
		result.Request = cfg.Request.Duration
		hasField = true
	}
	if cfg.BackendRequest != nil {
		result.BackendRequest = cfg.BackendRequest.Duration
		hasField = true
	}
	if cfg.Connect != nil {
		result.Connect = cfg.Connect.Duration
		hasField = true
	}
	if cfg.NextUpstream != nil {
		result.NextUpstream = cfg.NextUpstream.Duration
		hasField = true
	}

	if !hasField {
		return nil
	}
	return result
}

func translateBodyLimitConfig(cfg *rp.BodyLimitConfig) *ir.RouteBodyLimitConfig {
	result := &ir.RouteBodyLimitConfig{}
	hasField := false

	if cfg.MaxRequestBodyBytes != nil {
		result.MaxRequestBodyBytes = *cfg.MaxRequestBodyBytes
		hasField = true
	}
	if cfg.RequestBodyBufferBytes != nil {
		result.RequestBodyBufferBytes = *cfg.RequestBodyBufferBytes
		hasField = true
	}
	if cfg.MaxRequestHeaderBytes != nil {
		result.MaxRequestHeaderBytes = *cfg.MaxRequestHeaderBytes
		hasField = true
	}

	if !hasField {
		return nil
	}
	return result
}

func translateProxyConfig(cfg *rp.ProxyConfig) *ir.RouteProxyConfig {
	result := &ir.RouteProxyConfig{}
	hasField := false

	if cfg.RequestBuffering != nil {
		result.RequestBuffering = *cfg.RequestBuffering
		hasField = true
	}
	if cfg.ResponseBuffering != nil {
		result.ResponseBuffering = *cfg.ResponseBuffering
		hasField = true
	}
	if cfg.BufferSize != nil {
		result.BufferSize = *cfg.BufferSize
		hasField = true
	}
	if cfg.BufferCount != nil {
		result.BufferCount = *cfg.BufferCount
		hasField = true
	}

	if !hasField {
		return nil
	}
	return result
}

func translateConnectionConfig(cfg *rp.ConnectionConfig) *ir.RouteConnectionConfig {
	result := &ir.RouteConnectionConfig{}
	hasField := false

	if cfg.KeepaliveRequests != nil {
		result.KeepaliveRequests = *cfg.KeepaliveRequests
		hasField = true
	}
	if cfg.UpstreamKeepalivePoolSize != nil {
		result.UpstreamKeepalivePoolSize = *cfg.UpstreamKeepalivePoolSize
		hasField = true
	}
	if cfg.KeepaliveTime != nil {
		result.KeepaliveTime = cfg.KeepaliveTime.Duration
		hasField = true
	}
	if cfg.KeepaliveTimeout != nil {
		result.KeepaliveTimeout = cfg.KeepaliveTimeout.Duration
		hasField = true
	}
	if cfg.UpstreamKeepaliveIdle != nil {
		result.UpstreamKeepaliveIdle = cfg.UpstreamKeepaliveIdle.Duration
		hasField = true
	}

	if !hasField {
		return nil
	}
	return result
}
