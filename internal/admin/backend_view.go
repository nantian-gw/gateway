package admin

import (
	"strconv"
	"strings"

	"github.com/nantian-gw/gateway/internal/ir"
)

func visibleBackends(snapshot *ir.Snapshot, includeAll bool) []ir.BackendCluster {
	if snapshot == nil || len(snapshot.Backends) == 0 {
		return nil
	}
	if includeAll {
		return snapshot.Backends
	}

	keys := referencedBackendKeys(snapshot)
	if len(keys) == 0 {
		return nil
	}

	out := make([]ir.BackendCluster, 0, len(keys))
	for _, backend := range snapshot.Backends {
		if _, ok := keys[backendKey(backend)]; !ok {
			continue
		}
		out = append(out, backend)
	}
	return out
}

func referencedBackendCount(snapshot *ir.Snapshot) int {
	return len(visibleBackends(snapshot, false))
}

func referencedBackendKeys(snapshot *ir.Snapshot) map[string]struct{} {
	keys := make(map[string]struct{})
	if snapshot == nil {
		return keys
	}

	for _, route := range snapshot.HTTPRoutes {
		collectHTTPRuleBackendKeys(keys, route.Namespace, route.Rules)
	}
	for _, route := range snapshot.GRPCRoutes {
		collectGRPCRuleBackendKeys(keys, route.Namespace, route.Rules)
	}
	for _, route := range snapshot.StreamRoutes {
		collectStreamRuleBackendKeys(keys, route.Namespace, route.Rules)
	}

	return keys
}

func collectHTTPRuleBackendKeys(keys map[string]struct{}, namespace string, rules []ir.HTTPRule) {
	for _, rule := range rules {
		collectBackendRefKeys(keys, namespace, rule.BackendRefs)
	}
}

func collectGRPCRuleBackendKeys(keys map[string]struct{}, namespace string, rules []ir.GRPCRule) {
	for _, rule := range rules {
		collectBackendRefKeys(keys, namespace, rule.BackendRefs)
	}
}

func collectStreamRuleBackendKeys(keys map[string]struct{}, namespace string, rules []ir.StreamRule) {
	for _, rule := range rules {
		collectBackendRefKeys(keys, namespace, rule.BackendRefs)
	}
}

func collectBackendRefKeys(keys map[string]struct{}, defaultNamespace string, refs []ir.BackendRef) {
	for _, ref := range refs {
		namespace := ref.Namespace
		if namespace == "" {
			namespace = defaultNamespace
		}
		keys[backendKeyFromRef(namespace, ref.Name, ref.Port)] = struct{}{}
	}
}

func backendKey(cluster ir.BackendCluster) string {
	service := cluster.Metadata["service"]
	if service == "" {
		service = cluster.Name
	}
	return backendKeyFromRef(cluster.Namespace, service, backendPort(cluster.Name))
}

func backendKeyFromRef(namespace, service string, port uint32) string {
	return namespace + "/" + service + ":" + strconv.FormatUint(uint64(port), 10)
}

func backendPort(name string) uint32 {
	raw := name
	if idx := strings.LastIndex(raw, ":"); idx >= 0 && idx+1 < len(raw) {
		raw = raw[idx+1:]
	}
	value, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(value)
}
