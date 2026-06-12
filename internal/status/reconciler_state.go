package status

import (
	"context"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func (r *Reconciler) loadState(ctx context.Context) (*clusterState, error) {
	state := &clusterState{
		controllerName:  r.controllerName,
		statusAddresses: append([]string(nil), r.statusAddresses...),
	}

	gatewayClasses, err := listGatewayClassesForController(ctx, r.listReader, r.controllerName)
	if err != nil {
		return nil, err
	}
	state.gatewayClasses = gatewayClasses

	if len(gatewayClasses) == 0 {
		var gateways gatewayv1.GatewayList
		if err := r.listReader.List(ctx, &gateways); err != nil {
			return nil, err
		}
		state.gateways = gateways.Items
	} else {
		state.gateways = make([]gatewayv1.Gateway, 0)
		for _, gatewayClass := range gatewayClasses {
			gateways, err := listGatewaysForGatewayClass(ctx, r.listReader, gatewayClass.Name)
			if err != nil {
				return nil, err
			}
			state.gateways = append(state.gateways, gateways...)
		}
	}
	state.index()

	httpRoutes, err := loadHTTPRoutesForState(ctx, r.listReader, state.managedGateways, r.options)
	if err != nil {
		return nil, err
	}
	state.httpRoutes = httpRoutes

	grpcRoutes, err := loadGRPCRoutesForState(ctx, r.listReader, state.managedGateways)
	if err != nil {
		return nil, err
	}
	state.grpcRoutes = grpcRoutes

	if r.experimentalGatewayEnabled() {
		tcpRoutes, err := loadTCPRoutesForState(ctx, r.listReader, state.managedGateways)
		if err != nil {
			return nil, err
		}
		state.tcpRoutes = tcpRoutes

		udpRoutes, err := loadUDPRoutesForState(ctx, r.listReader, state.managedGateways)
		if err != nil {
			return nil, err
		}
		state.udpRoutes = udpRoutes

		tlsRoutes, err := loadTLSRoutesForState(ctx, r.listReader, state.managedGateways)
		if err != nil {
			return nil, err
		}
		state.tlsRoutes = tlsRoutes

		listenerSets, err := loadListenerSetsForState(ctx, r.listReader, state.managedGateways)
		if err != nil {
			return nil, err
		}
		state.listenerSets = listenerSets
	}

	if err := r.loadRouteReferencedBackendPolicies(ctx, state); err != nil {
		return nil, err
	}

	if err := r.loadReferencedSupportResources(ctx, state); err != nil {
		return nil, err
	}
	state.index()

	return state, nil
}

// loadListenerSetsForState returns ListenerSets whose parentRef references
// one of the managed Gateways.
func loadListenerSetsForState(
	ctx context.Context,
	reader client.Reader,
	managedGateways []gatewayv1.Gateway,
) ([]gatewayv1.ListenerSet, error) {
	var allSets gatewayv1.ListenerSetList
	if err := reader.List(ctx, &allSets); err != nil {
		// If the ListenerSet CRD is not installed (standard-only install),
		// return an empty list instead of failing the reconciliation.
		if strings.Contains(err.Error(), "no matches for kind") {
			return nil, nil
		}
		return nil, fmt.Errorf("listing ListenerSets: %w", err)
	}
	managed := make(map[string]bool, len(managedGateways))
	for _, g := range managedGateways {
		managed[g.Namespace+"/"+g.Name] = true
	}
	out := make([]gatewayv1.ListenerSet, 0, len(allSets.Items))
	for _, ls := range allSets.Items {
		key := listenerSetParentGatewayKey(ls)
		if managed[key] {
			out = append(out, ls)
		}
	}
	return out, nil
}
