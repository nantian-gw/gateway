package status

import "sigs.k8s.io/controller-runtime/pkg/client"

func evaluateRouteAttachments(state *clusterState) map[listenerKey]map[string]struct{} {
	attachments := make(map[listenerKey]map[string]struct{})

	for _, route := range state.httpRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, evaluateRouteAttachmentsForInput(state, httpRouteInput(route)))
	}

	for _, route := range state.grpcRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, evaluateRouteAttachmentsForInput(state, grpcRouteInput(route)))
	}

	for _, route := range state.tcpRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, evaluateRouteAttachmentsForInput(state, tcpRouteInput(route)))
	}

	for _, route := range state.udpRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, evaluateRouteAttachmentsForInput(state, udpRouteInput(route)))
	}

	for _, route := range state.tlsRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, evaluateRouteAttachmentsForInput(state, tlsRouteInput(route)))
	}

	return attachments
}

func evaluateRouteAttachmentsForInput(state *clusterState, route routeInput) []routeParentEvaluation {
	parentRefs := routeEffectiveParentRefs(state, route)
	if len(parentRefs) == 0 {
		return nil
	}

	out := make([]routeParentEvaluation, 0, len(parentRefs))
	for _, parentRef := range parentRefs {
		eval, ok := evaluateParentRef(state, route, parentRef, routeResolutionEvaluation{
			resolvedCondition: conditionSpec{
				ObservedGeneration: route.generation,
			},
		})
		if !ok {
			continue
		}
		out = append(out, eval)
	}

	return out
}
