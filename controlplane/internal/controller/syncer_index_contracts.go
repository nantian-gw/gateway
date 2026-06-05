package controller

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
)

type IndexName string

func (n IndexName) String() string {
	return string(n)
}

type fieldIndexContract struct {
	Name           IndexName
	Object         client.Object
	Extract        client.IndexerFunc
	Owner          string
	WatchSource    string
	RequestMapping string
	Fallback       string
}

func RegisterIndex(ctx context.Context, indexer client.FieldIndexer, contract fieldIndexContract) error {
	if contract.Name == "" {
		return fmt.Errorf("register field index: missing index name")
	}
	if contract.Object == nil {
		return fmt.Errorf("register field index %q: missing object", contract.Name)
	}
	if contract.Extract == nil {
		return fmt.Errorf("register field index %q: missing extract function", contract.Name)
	}

	if err := indexer.IndexField(ctx, contract.Object, contract.Name.String(), contract.Extract); err != nil {
		return fmt.Errorf(
			"index %s %s for %s via %s: %w",
			contract.Owner,
			contract.Name,
			contract.WatchSource,
			contract.RequestMapping,
			err,
		)
	}
	return nil
}

func EnqueueRequestsFromX(mapFunc handler.MapFunc) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(mapFunc)
}

type indexFallbackSemantics struct {
	Owner          string
	Kind           string
	Index          IndexName
	IndexValue     string
	RequestMapping string
	FallbackScope  string
}

type missingFieldIndexFallbackLogKey struct {
	owner          string
	kind           string
	index          IndexName
	requestMapping string
	fallbackScope  string
}

type indexedListDecoder[T any] func(client.ObjectList) ([]T, error)
type indexedListFallback[T any] func(context.Context) ([]T, error)
type missingFieldIndexFallbackLogger func(indexFallbackSemantics, error)

func ListByIndexOrFallback[T any](
	ctx context.Context,
	cl client.Client,
	list client.ObjectList,
	index IndexName,
	indexValue string,
	decode indexedListDecoder[T],
	fallback indexedListFallback[T],
	semantics indexFallbackSemantics,
	logFallback missingFieldIndexFallbackLogger,
) ([]T, bool, error) {
	if cl == nil {
		var zero []T
		return zero, false, fmt.Errorf("list by index %q: nil client", index)
	}
	if list == nil {
		var zero []T
		return zero, false, fmt.Errorf("list by index %q: nil list", index)
	}
	if decode == nil {
		var zero []T
		return zero, false, fmt.Errorf("list by index %q: nil decoder", index)
	}

	if err := cl.List(ctx, list, client.MatchingFields{index.String(): indexValue}); err != nil {
		if !isMissingFieldIndexError(err) || fallback == nil {
			var zero []T
			return zero, false, err
		}
		if logFallback != nil {
			logFallback(semantics, err)
		}
		items, fallbackErr := fallback(ctx)
		return items, false, fallbackErr
	}

	items, err := decode(list)
	return items, true, err
}

func (s *Syncer) logMissingFieldIndexFallbackOnce(semantics indexFallbackSemantics, err error) {
	if s == nil || s.logger == nil {
		return
	}
	if s.markMissingFieldIndexFallback(semantics) {
		logMissingFieldIndexFallback(
			s.logger,
			slog.LevelWarn,
			"missing field index; falling back to configured list path",
			semantics,
			err,
		)
		return
	}
	logMissingFieldIndexFallback(
		s.logger,
		slog.LevelDebug,
		"missing field index; continuing with configured list path",
		semantics,
		err,
	)
}

func (s *Syncer) markMissingFieldIndexFallback(semantics indexFallbackSemantics) bool {
	if s == nil {
		return false
	}
	key := missingFieldIndexFallbackLogKey{
		owner:          semantics.Owner,
		kind:           semantics.Kind,
		index:          semantics.Index,
		requestMapping: semantics.RequestMapping,
		fallbackScope:  semantics.FallbackScope,
	}
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	if s.missingFieldIndexFallbacks == nil {
		s.missingFieldIndexFallbacks = make(map[missingFieldIndexFallbackLogKey]struct{})
	}
	if _, ok := s.missingFieldIndexFallbacks[key]; ok {
		return false
	}
	s.missingFieldIndexFallbacks[key] = struct{}{}
	return true
}

func logMissingFieldIndexFallback(
	logger *slog.Logger,
	level slog.Level,
	message string,
	semantics indexFallbackSemantics,
	err error,
) {
	if logger == nil {
		return
	}
	attrs := []slog.Attr{
		slog.String("owner", semantics.Owner),
		slog.String("kind", semantics.Kind),
		slog.String("field_index", semantics.Index.String()),
		slog.String("request_mapping", semantics.RequestMapping),
		slog.String("fallback_scope", semantics.FallbackScope),
	}
	if err != nil {
		attrs = append(attrs, slog.Any("error", err))
	}
	logger.LogAttrs(
		context.Background(),
		level,
		message,
		attrs...,
	)
}

func controllerReferenceIndexContracts(includeBackendTLSPolicy bool) []fieldIndexContract {
	contracts := []fieldIndexContract{
		{
			Name:           gatewaySecretReferenceIndex,
			Object:         &gatewayv1.Gateway{},
			Extract:        gatewaySecretReferenceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Secret",
			RequestMapping: "secretReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           gatewayConfigMapReferenceIndex,
			Object:         &gatewayv1.Gateway{},
			Extract:        gatewayConfigMapReferenceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ConfigMap",
			RequestMapping: "configMapReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           gatewayReferenceGrantNamespaceIndex,
			Object:         &gatewayv1.Gateway{},
			Extract:        gatewayReferenceGrantNamespaceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ReferenceGrant",
			RequestMapping: "referenceGrantReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           gatewayNamespaceSelectorIndex,
			Object:         &gatewayv1.Gateway{},
			Extract:        gatewayNamespaceSelectorIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Namespace",
			RequestMapping: "namespaceReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           httpRouteConfigMapReferenceIndex,
			Object:         &gatewayv1.HTTPRoute{},
			Extract:        httpRouteConfigMapReferenceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ConfigMap",
			RequestMapping: "configMapReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           httpRouteParentGatewayIndex,
			Object:         &gatewayv1.HTTPRoute{},
			Extract:        httpRouteParentGatewayIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Gateway",
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           httpRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1.HTTPRoute{},
			Extract:        httpRouteReferenceGrantNamespaceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ReferenceGrant",
			RequestMapping: "referenceGrantReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           grpcRouteConfigMapReferenceIndex,
			Object:         &gatewayv1.GRPCRoute{},
			Extract:        grpcRouteConfigMapReferenceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ConfigMap",
			RequestMapping: "configMapReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           grpcRouteParentGatewayIndex,
			Object:         &gatewayv1.GRPCRoute{},
			Extract:        grpcRouteParentGatewayIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Gateway",
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           grpcRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1.GRPCRoute{},
			Extract:        grpcRouteReferenceGrantNamespaceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ReferenceGrant",
			RequestMapping: "referenceGrantReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           tcpRouteParentGatewayIndex,
			Object:         &gatewayv1alpha2.TCPRoute{},
			Extract:        tcpRouteParentGatewayIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Gateway",
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           tcpRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1alpha2.TCPRoute{},
			Extract:        tcpRouteReferenceGrantNamespaceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ReferenceGrant",
			RequestMapping: "referenceGrantReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           udpRouteParentGatewayIndex,
			Object:         &gatewayv1alpha2.UDPRoute{},
			Extract:        udpRouteParentGatewayIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Gateway",
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           udpRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1alpha2.UDPRoute{},
			Extract:        udpRouteReferenceGrantNamespaceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ReferenceGrant",
			RequestMapping: "referenceGrantReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           tlsRouteParentGatewayIndex,
			Object:         &gatewayv1alpha2.TLSRoute{},
			Extract:        tlsRouteParentGatewayIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Gateway",
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           listenerSetParentGatewayIndex,
			Object:         &gatewayv1.ListenerSet{},
			Extract:        listenerSetParentGatewayIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "Gateway",
			RequestMapping: "listenerSetsForParentGateway",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
		{
			Name:           tlsRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1alpha2.TLSRoute{},
			Extract:        tlsRouteReferenceGrantNamespaceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ReferenceGrant",
			RequestMapping: "referenceGrantReconcileRequests",
			Fallback:       "full snapshot rebuild when dependency lookup cannot use the index",
		},
	}

	if includeBackendTLSPolicy {
		contracts = append(contracts, fieldIndexContract{
			Name:           backendTLSPolicyConfigMapRefIndex,
			Object:         gatewayapi.NewBackendTLSPolicyV1Object(),
			Extract:        backendTLSPolicyConfigMapReferenceIndexKeys,
			Owner:          "snapshot-syncer",
			WatchSource:    "ConfigMap",
			RequestMapping: "backendTLSPoliciesForConfigMapIndex",
			Fallback:       "namespace BackendTLSPolicy scan with missing-index log",
		})
	}

	return contracts
}
