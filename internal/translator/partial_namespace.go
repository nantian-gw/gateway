package translator

import (
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

type referencedBackendObjectKeys struct {
	services       []client.ObjectKey
	serviceImports []client.ObjectKey
}

func referencedBackendNamespaces(keys referencedBackendObjectKeys) []string {
	namespaces := make(map[string]struct{}, len(keys.services)+len(keys.serviceImports))
	for _, key := range keys.services {
		if key.Namespace != "" {
			namespaces[key.Namespace] = struct{}{}
		}
	}
	for _, key := range keys.serviceImports {
		if key.Namespace != "" {
			namespaces[key.Namespace] = struct{}{}
		}
	}

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func referencedBackendGrantNamespacesFromSnapshot(current *ir.Snapshot) []string {
	namespaces := make(map[string]struct{})
	if current == nil {
		return nil
	}

	add := func(routeNamespace string, allowCrossNamespaceRefs bool, backendRefs []ir.BackendRef) {
		if allowCrossNamespaceRefs {
			return
		}
		for _, ref := range backendRefs {
			if ref.Name == "" || ref.Namespace == "" || ref.Namespace == routeNamespace {
				continue
			}
			if _, ok := backends.BackendKindForRef(ref.Group, ref.Kind); !ok {
				continue
			}
			namespaces[ref.Namespace] = struct{}{}
		}
	}

	for _, route := range current.HTTPRoutes {
		allowCrossNamespaceRefs := backends.RouteUsesOnlyServiceParents(route.ParentRefs)
		for _, rule := range route.Rules {
			add(route.Namespace, allowCrossNamespaceRefs, rule.BackendRefs)
		}
	}
	for _, route := range current.GRPCRoutes {
		allowCrossNamespaceRefs := backends.RouteUsesOnlyServiceParents(route.ParentRefs)
		for _, rule := range route.Rules {
			add(route.Namespace, allowCrossNamespaceRefs, rule.BackendRefs)
		}
	}
	for _, route := range current.StreamRoutes {
		allowCrossNamespaceRefs := backends.RouteUsesOnlyServiceParents(route.ParentRefs)
		for _, rule := range route.Rules {
			add(route.Namespace, allowCrossNamespaceRefs, rule.BackendRefs)
		}
	}

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func referencedBackendObjectKeysFromSnapshot(current *ir.Snapshot) referencedBackendObjectKeys {
	serviceKeys := make(map[string]client.ObjectKey)
	serviceImportKeys := make(map[string]client.ObjectKey)
	if current == nil {
		return referencedBackendObjectKeys{}
	}

	add := func(ref ir.BackendRef) {
		if ref.Name == "" || ref.Namespace == "" {
			return
		}

		key := client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}
		switch kind, ok := backends.BackendKindForRef(ref.Group, ref.Kind); {
		case !ok:
			return
		case kind == "Service":
			serviceKeys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
		case kind == "ServiceImport":
			serviceImportKeys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
		}
	}

	for _, route := range current.HTTPRoutes {
		for _, rule := range route.Rules {
			for _, ref := range rule.BackendRefs {
				add(ref)
			}
		}
	}
	for _, route := range current.GRPCRoutes {
		for _, rule := range route.Rules {
			for _, ref := range rule.BackendRefs {
				add(ref)
			}
		}
	}
	for _, route := range current.StreamRoutes {
		for _, rule := range route.Rules {
			for _, ref := range rule.BackendRefs {
				add(ref)
			}
		}
	}

	return referencedBackendObjectKeys{
		services:       sortedObjectKeys(serviceKeys),
		serviceImports: sortedObjectKeys(serviceImportKeys),
	}
}