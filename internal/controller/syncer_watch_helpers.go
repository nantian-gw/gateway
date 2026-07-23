package controller

import (
	"context"
	"sort"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)


func (s *Syncer) logDependencyLookupError(kind, namespace, name string, err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	s.logger.Error(
		"failed to resolve snapshot rebuild dependencies; falling back to full rebuild trigger",
		"kind", kind,
		"namespace", namespace,
		"name", name,
		"error", err,
	)
}

func isMissingFieldIndexError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "no index with name") ||
		(strings.Contains(message, "specifies selector on field") && strings.Contains(message, "has been registered")) ||
		strings.Contains(message, "field label not supported")
}

func sortedIndexValues(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (s *Syncer) gatewaysForFieldIndex(
	ctx context.Context,
	field string,
	indexValue string,
) ([]gatewayv1.Gateway, error) {
	var list gatewayv1.GatewayList
	if err := s.client.List(ctx, &list, client.MatchingFields{field: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) gatewaysForGatewayClassName(
	ctx context.Context,
	className string,
) ([]gatewayv1.Gateway, error) {
	var list gatewayv1.GatewayList
	if err := s.client.List(ctx, &list, client.MatchingFields{gatewayGatewayClassNameIndex: className}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) httpRoutesForConfigMapIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1.HTTPRoute, error) {
	var list gatewayv1.HTTPRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{httpRouteConfigMapReferenceIndex: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) grpcRoutesForConfigMapIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1.GRPCRoute, error) {
	var list gatewayv1.GRPCRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{grpcRouteConfigMapReferenceIndex: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) httpRoutesForGatewayParentIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1.HTTPRoute, error) {
	var list gatewayv1.HTTPRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{httpRouteParentGatewayIndex: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) grpcRoutesForGatewayParentIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1.GRPCRoute, error) {
	var list gatewayv1.GRPCRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{grpcRouteParentGatewayIndex: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) tcpRoutesForGatewayParentIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1alpha2.TCPRoute, error) {
	var list gatewayv1alpha2.TCPRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{tcpRouteParentGatewayIndex: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) udpRoutesForGatewayParentIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1alpha2.UDPRoute, error) {
	var list gatewayv1alpha2.UDPRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{udpRouteParentGatewayIndex: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) tlsRoutesForGatewayParentIndex(
	ctx context.Context,
	indexValue string,
) ([]gatewayv1alpha2.TLSRoute, error) {
	var list gatewayv1alpha2.TLSRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{tlsRouteParentGatewayIndex: indexValue}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) httpRoutesForReferenceGrantNamespace(
	ctx context.Context,
	namespace string,
) ([]gatewayv1.HTTPRoute, error) {
	var list gatewayv1.HTTPRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{httpRouteReferenceGrantNamespaceIndex: namespace}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) grpcRoutesForReferenceGrantNamespace(
	ctx context.Context,
	namespace string,
) ([]gatewayv1.GRPCRoute, error) {
	var list gatewayv1.GRPCRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{grpcRouteReferenceGrantNamespaceIndex: namespace}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) tcpRoutesForReferenceGrantNamespace(
	ctx context.Context,
	namespace string,
) ([]gatewayv1alpha2.TCPRoute, error) {
	var list gatewayv1alpha2.TCPRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{tcpRouteReferenceGrantNamespaceIndex: namespace}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) udpRoutesForReferenceGrantNamespace(
	ctx context.Context,
	namespace string,
) ([]gatewayv1alpha2.UDPRoute, error) {
	var list gatewayv1alpha2.UDPRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{udpRouteReferenceGrantNamespaceIndex: namespace}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func (s *Syncer) tlsRoutesForReferenceGrantNamespace(
	ctx context.Context,
	namespace string,
) ([]gatewayv1alpha2.TLSRoute, error) {
	var list gatewayv1alpha2.TLSRouteList
	if err := s.client.List(ctx, &list, client.MatchingFields{tlsRouteReferenceGrantNamespaceIndex: namespace}); err != nil {
		return nil, err
	}
	sort.Slice(list.Items, func(i, j int) bool {
		left := list.Items[i].Namespace + "/" + list.Items[i].Name
		right := list.Items[j].Namespace + "/" + list.Items[j].Name
		return left < right
	})
	return list.Items, nil
}

func sortedReconcileRequestsMap(requests map[reconcile.Request]struct{}) []reconcile.Request {
	if len(requests) == 0 {
		return nil
	}
	out := make([]reconcile.Request, 0, len(requests))
	for request := range requests {
		out = append(out, request)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Namespace + "/" + out[i].Name
		right := out[j].Namespace + "/" + out[j].Name
		return left < right
	})
	return out
}
