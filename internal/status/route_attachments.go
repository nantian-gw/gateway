package status

import "sigs.k8s.io/controller-runtime/pkg/client"

func evaluateRouteAttachments(state *clusterState) map[listenerKey]routeAttachmentSet {
	ctx := newRouteEvaluationContext(state)
	attachments := make(map[listenerKey]routeAttachmentSet)

	for _, route := range state.httpRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, ctx.evaluateRouteAttachments(httpRouteInput(route)))
	}

	for _, route := range state.grpcRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, ctx.evaluateRouteAttachments(grpcRouteInput(route)))
	}

	for _, route := range state.tcpRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, ctx.evaluateRouteAttachments(tcpRouteInput(route)))
	}

	for _, route := range state.udpRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, ctx.evaluateRouteAttachments(udpRouteInput(route)))
	}

	for _, route := range state.tlsRoutes {
		key := client.ObjectKeyFromObject(&route)
		recordAttachments(attachments, key, ctx.evaluateRouteAttachments(tlsRouteInput(route)))
	}

	return attachments
}
