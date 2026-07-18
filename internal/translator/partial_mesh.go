package translator

import (
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func collectMeshServiceFrontendsFromSnapshot(
	services []corev1.Service,
	current *ir.Snapshot,
) []mesh.ServiceFrontendPort {
	keys := make([]mesh.ServiceParentKey, 0)
	for _, route := range current.HTTPRoutes {
		keys = append(keys, collectMeshParentKeysFromIR(route.ParentRefs)...)
	}
	for _, route := range current.GRPCRoutes {
		keys = append(keys, collectMeshParentKeysFromIR(route.ParentRefs)...)
	}
	for _, route := range current.StreamRoutes {
		keys = append(keys, collectMeshParentKeysFromIR(route.ParentRefs)...)
	}
	return mesh.ExpandServiceFrontends(services, keys)
}

func meshParentServiceObjectKeysFromSnapshot(current *ir.Snapshot) []client.ObjectKey {
	keys := make(map[string]client.ObjectKey)
	if current == nil {
		return nil
	}

	add := func(parentRefs []ir.ParentRef) {
		for _, key := range collectMeshParentKeysFromIR(parentRefs) {
			objectKey := client.ObjectKey{Namespace: key.Namespace, Name: key.Name}
			keys[shared.BackendObjectKey(objectKey.Namespace, objectKey.Name)] = objectKey
		}
	}

	for _, route := range current.HTTPRoutes {
		add(route.ParentRefs)
	}
	for _, route := range current.GRPCRoutes {
		add(route.ParentRefs)
	}
	for _, route := range current.StreamRoutes {
		add(route.ParentRefs)
	}

	return sortedObjectKeys(keys)
}

func meshWorkloadNamespacesFromSnapshot(current *ir.Snapshot) []string {
	namespaces := make(map[string]struct{})
	if current == nil {
		return nil
	}

	add := func(routeNamespace string, parentRefs []ir.ParentRef) {
		for _, parentRef := range parentRefs {
			if !isServiceParentRef(parentRef) {
				continue
			}
			if routeNamespace != "" {
				namespaces[routeNamespace] = struct{}{}
			}
			return
		}
	}

	for _, route := range current.HTTPRoutes {
		add(route.Namespace, route.ParentRefs)
	}
	for _, route := range current.GRPCRoutes {
		add(route.Namespace, route.ParentRefs)
	}
	for _, route := range current.StreamRoutes {
		add(route.Namespace, route.ParentRefs)
	}

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func collectMeshParentKeysFromIR(parentRefs []ir.ParentRef) []mesh.ServiceParentKey {
	out := make([]mesh.ServiceParentKey, 0, len(parentRefs))
	for _, parentRef := range parentRefs {
		if !isServiceParentRef(parentRef) || parentRef.Namespace == "" {
			continue
		}
		out = append(out, mesh.ServiceParentKey{
			Namespace: parentRef.Namespace,
			Name:      parentRef.Name,
		})
	}
	return out
}