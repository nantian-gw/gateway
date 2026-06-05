package mesh

import gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

func RouteUsesOnlyServiceParents(
	parentRefs []gatewayv1.ParentReference,
	defaultNamespace string,
) bool {
	if len(parentRefs) == 0 {
		return false
	}

	for _, parentRef := range parentRefs {
		if _, ok := ParentServiceRef(parentRef, defaultNamespace); !ok {
			return false
		}
	}

	return true
}
