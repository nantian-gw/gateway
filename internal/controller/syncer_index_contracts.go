package controller

import (
	"context"
	"fmt"
	"log/slog"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/constants"
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

type (
	indexedListDecoder[T any]       func(client.ObjectList) ([]T, error)
	indexedListFallback[T any]      func(context.Context) ([]T, error)
	missingFieldIndexFallbackLogger func(indexFallbackSemantics, error)
)

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
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "Secret",
			RequestMapping: "secretReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           gatewayConfigMapReferenceIndex,
			Object:         &gatewayv1.Gateway{},
			Extract:        gatewayConfigMapReferenceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeConfigMap,
			RequestMapping: "configMapReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           gatewayReferenceGrantNamespaceIndex,
			Object:         &gatewayv1.Gateway{},
			Extract:        gatewayReferenceGrantNamespaceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "ReferenceGrant",
			RequestMapping: constants.StrName + "ReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           gatewayNamespaceSelectorIndex,
			Object:         &gatewayv1.Gateway{},
			Extract:        gatewayNamespaceSelectorIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "Namespace",
			RequestMapping: "namespaceReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           httpRouteConfigMapReferenceIndex,
			Object:         &gatewayv1.HTTPRoute{},
			Extract:        httpRouteConfigMapReferenceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeConfigMap,
			RequestMapping: "configMapReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           httpRouteParentGatewayIndex,
			Object:         &gatewayv1.HTTPRoute{},
			Extract:        httpRouteParentGatewayIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeGateway,
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           httpRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1.HTTPRoute{},
			Extract:        httpRouteReferenceGrantNamespaceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "ReferenceGrant",
			RequestMapping: constants.StrName + "ReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           grpcRouteConfigMapReferenceIndex,
			Object:         &gatewayv1.GRPCRoute{},
			Extract:        grpcRouteConfigMapReferenceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeConfigMap,
			RequestMapping: "configMapReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           grpcRouteParentGatewayIndex,
			Object:         &gatewayv1.GRPCRoute{},
			Extract:        grpcRouteParentGatewayIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeGateway,
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           grpcRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1.GRPCRoute{},
			Extract:        grpcRouteReferenceGrantNamespaceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "ReferenceGrant",
			RequestMapping: constants.StrName + "ReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           tcpRouteParentGatewayIndex,
			Object:         &gatewayv1alpha2.TCPRoute{},
			Extract:        tcpRouteParentGatewayIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeGateway,
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           tcpRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1alpha2.TCPRoute{},
			Extract:        tcpRouteReferenceGrantNamespaceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "ReferenceGrant",
			RequestMapping: constants.StrName + "ReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           udpRouteParentGatewayIndex,
			Object:         &gatewayv1alpha2.UDPRoute{},
			Extract:        udpRouteParentGatewayIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeGateway,
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           udpRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1alpha2.UDPRoute{},
			Extract:        udpRouteReferenceGrantNamespaceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "ReferenceGrant",
			RequestMapping: constants.StrName + "ReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           tlsRouteParentGatewayIndex,
			Object:         &gatewayv1alpha2.TLSRoute{},
			Extract:        tlsRouteParentGatewayIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeGateway,
			RequestMapping: "routeAttachmentNamespacesForGateway",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           listenerSetParentGatewayIndex,
			Object:         &gatewayv1.ListenerSet{},
			Extract:        listenerSetParentGatewayIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeGateway,
			RequestMapping: "listenerSetsForParentGateway",
			Fallback:       constants.FullSnapshotRebuild,
		},
		{
			Name:           tlsRouteReferenceGrantNamespaceIndex,
			Object:         &gatewayv1alpha2.TLSRoute{},
			Extract:        tlsRouteReferenceGrantNamespaceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    "ReferenceGrant",
			RequestMapping: constants.StrName + "ReconcileRequests",
			Fallback:       constants.FullSnapshotRebuild,
		},
	}

	if includeBackendTLSPolicy {
		contracts = append(contracts, fieldIndexContract{
			Name:           backendTLSPolicyConfigMapRefIndex,
			Object:         gatewayapi.NewBackendTLSPolicyV1Object(),
			Extract:        backendTLSPolicyConfigMapReferenceIndexKeys,
			Owner:          constants.NameSnapshotSyncer,
			WatchSource:    constants.KubeConfigMap,
			RequestMapping: "backendTLSPoliciesForConfigMapIndex",
			Fallback:       "namespace BackendTLSPolicy scan with missing-index log",
		})
	}

	return contracts
}
