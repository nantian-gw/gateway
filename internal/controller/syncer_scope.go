package controller

import (
	"context"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator"
)

type snapshotBuildScope uint16

const (
	snapshotBuildScopeNone     snapshotBuildScope = 0
	snapshotBuildScopeBackends snapshotBuildScope = 1 << iota
	snapshotBuildScopeAttachments
	snapshotBuildScopeGatewayListeners
	snapshotBuildScopeRoutes
	snapshotBuildScopeRouteBackendRefs
	snapshotBuildScopeMeshListeners
	snapshotBuildScopeWorkloads
	snapshotBuildScopeFull
)

func (s snapshotBuildScope) String() string {
	switch s {
	case snapshotBuildScopeNone:
		return "none"
	case snapshotBuildScopeFull:
		return "full"
	}

	parts := make([]string, 0, 7)
	if s&snapshotBuildScopeBackends != 0 {
		parts = append(parts, "backends")
	}
	if s&snapshotBuildScopeAttachments != 0 {
		parts = append(parts, "attachments")
	}
	if s&snapshotBuildScopeGatewayListeners != 0 {
		parts = append(parts, "gateway_listeners")
	}
	if s&snapshotBuildScopeRoutes != 0 {
		parts = append(parts, "routes")
	}
	if s&snapshotBuildScopeRouteBackendRefs != 0 {
		parts = append(parts, "route_backend_refs")
	}
	if s&snapshotBuildScopeMeshListeners != 0 {
		parts = append(parts, "mesh_listeners")
	}
	if s&snapshotBuildScopeWorkloads != 0 {
		parts = append(parts, "workloads")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, "+")
}

const (
	snapshotReconcileRequestName                 = "snapshot"
	snapshotBackendsReconcileRequestName         = "snapshot-backends"
	snapshotBackendsNamespaceRequestName         = "snapshot-backends-namespace"
	snapshotBackendsServiceRequestName           = "snapshot-backends-service"
	snapshotBackendsServiceImportRequestName     = "snapshot-backends-serviceimport"
	snapshotBackendDependenciesRequestName       = "snapshot-backend-deps"
	snapshotServiceDependenciesRequestName       = "snapshot-service-deps"
	snapshotAttachmentsReconcileRequestName      = "snapshot-attachments"
	snapshotGatewayListenersReconcileRequestName = "snapshot-gateway-listeners"
	snapshotWorkloadsReconcileRequestName        = "snapshot-workloads"
	snapshotHTTPRoutesReconcileRequestName       = "snapshot-routes-http"
	snapshotGRPCRoutesReconcileRequestName       = "snapshot-routes-grpc"
	snapshotTCPRoutesReconcileRequestName        = "snapshot-routes-tcp"
	snapshotUDPRoutesReconcileRequestName        = "snapshot-routes-udp"
	snapshotTLSRoutesReconcileRequestName        = "snapshot-routes-tls"
)

type snapshotRouteObjectKeys struct {
	http []client.ObjectKey
	grpc []client.ObjectKey
	tcp  []client.ObjectKey
	udp  []client.ObjectKey
	tls  []client.ObjectKey
}

type snapshotPendingBuild struct {
	scope                snapshotBuildScope
	attachmentNamespaces map[string]struct{}
	backendNamespaces    map[string]struct{}
	gatewayKeys          map[string]client.ObjectKey
	serviceKeys          map[string]client.ObjectKey
	serviceImportKeys    map[string]client.ObjectKey
	httpRouteKeys        map[string]client.ObjectKey
	grpcRouteKeys        map[string]client.ObjectKey
	tcpRouteKeys         map[string]client.ObjectKey
	udpRouteKeys         map[string]client.ObjectKey
	tlsRouteKeys         map[string]client.ObjectKey
}

var (
	snapshotReconcileRequest = reconcile.Request{
		NamespacedName: types.NamespacedName{Name: snapshotReconcileRequestName},
	}
	snapshotBackendsReconcileRequest = reconcile.Request{
		NamespacedName: types.NamespacedName{Name: snapshotBackendsReconcileRequestName},
	}
	snapshotBackendDependenciesReconcileRequest = reconcile.Request{
		NamespacedName: types.NamespacedName{Name: snapshotBackendDependenciesRequestName},
	}
	snapshotServiceDependenciesReconcileRequest = reconcile.Request{
		NamespacedName: types.NamespacedName{Name: snapshotServiceDependenciesRequestName},
	}
	snapshotWorkloadsReconcileRequest = reconcile.Request{
		NamespacedName: types.NamespacedName{Name: snapshotWorkloadsReconcileRequestName},
	}
)

func snapshotAttachmentsReconcileRequest(namespace string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      snapshotAttachmentsReconcileRequestName,
		},
	}
}

func snapshotBackendsReconcileRequestForNamespace(namespace string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: namespace,
			Name:      snapshotBackendsNamespaceRequestName,
		},
	}
}

func snapshotBackendDependenciesReconcileRequestForServiceImport(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotBackendDependenciesRequestName, key)
}

func snapshotBackendsReconcileRequestForService(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotBackendsServiceRequestName, key)
}

func snapshotBackendsReconcileRequestForServiceImport(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotBackendsServiceImportRequestName, key)
}

func snapshotServiceDependenciesReconcileRequestForService(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotServiceDependenciesRequestName, key)
}

func snapshotGatewayListenersReconcileRequestForKey(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotGatewayListenersReconcileRequestName, key)
}

func snapshotHTTPRoutesReconcileRequestForKey(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotHTTPRoutesReconcileRequestName, key)
}

func snapshotGRPCRoutesReconcileRequestForKey(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotGRPCRoutesReconcileRequestName, key)
}

func snapshotTCPRoutesReconcileRequestForKey(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotTCPRoutesReconcileRequestName, key)
}

func snapshotUDPRoutesReconcileRequestForKey(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotUDPRoutesReconcileRequestName, key)
}

func snapshotTLSRoutesReconcileRequestForKey(key client.ObjectKey) reconcile.Request {
	return snapshotScopedObjectReconcileRequest(snapshotTLSRoutesReconcileRequestName, key)
}

func snapshotScopedObjectReconcileRequest(baseName string, key client.ObjectKey) reconcile.Request {
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: key.Namespace,
			Name:      baseName,
		},
	}
	if key.Name != "" {
		request.Name = baseName + "/" + key.Name
	}
	return request
}

func (s *Syncer) queueScopedSettleRun(
	fallbackCtx context.Context,
	scope snapshotBuildScope,
	attachmentNamespace string,
	backendNamespace string,
	gatewayKeys []client.ObjectKey,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	routeKeys snapshotRouteObjectKeys,
) {
	if s.settleDelay <= 0 {
		return
	}

	s.settleMu.Lock()
	defer s.settleMu.Unlock()

	s.mergePendingBuildLocked(scope, attachmentNamespace, backendNamespace, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys)
	if s.settleTimer != nil {
		s.settleTimer.Stop()
	}

	delay := s.settleDelay
	run := s.settleRun
	settleCtx := fallbackCtx
	if s.lifecycleCtx != nil {
		settleCtx = s.lifecycleCtx
	}
	if run == nil {
		run = func(ctx context.Context) {
			if ctx.Err() != nil {
				return
			}
			scope, attachmentNamespaces, backendNamespaces, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys := s.consumePendingBuild()
			if scope == snapshotBuildScopeNone {
				return
			}
			published, err := s.publishSnapshotWithScope(
				ctx,
				scope,
				attachmentNamespaces,
				backendNamespaces,
				gatewayKeys,
				serviceKeys,
				serviceImportKeys,
				routeKeys,
			)
			if err != nil {
				s.mergeRetryPendingBuild(
					scope,
					attachmentNamespaces,
					backendNamespaces,
					gatewayKeys,
					serviceKeys,
					serviceImportKeys,
					routeKeys,
				)
				return
			}
			if published {
				s.queueLeaderRun(scope)
			}
		}
	}
	s.settleTimer = time.AfterFunc(delay, func() {
		run(settleCtx)
	})
}

func (s *Syncer) buildScopeForRequest(
	request ctrl.Request,
) (snapshotBuildScope, string, string, []client.ObjectKey, []client.ObjectKey, []client.ObjectKey, snapshotRouteObjectKeys) {
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotBackendsServiceRequestName); ok {
		return snapshotBuildScopeBackends, "", "", nil, []client.ObjectKey{key}, nil, snapshotRouteObjectKeys{}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotBackendsServiceImportRequestName); ok {
		return snapshotBuildScopeBackends, "", "", nil, nil, []client.ObjectKey{key}, snapshotRouteObjectKeys{}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotBackendDependenciesRequestName); ok {
		return snapshotBuildScopeBackends | snapshotBuildScopeRouteBackendRefs, "", "", nil, nil, []client.ObjectKey{key}, snapshotRouteObjectKeys{}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotServiceDependenciesRequestName); ok {
		return snapshotBuildScopeBackends | snapshotBuildScopeRouteBackendRefs | snapshotBuildScopeMeshListeners, "", "", nil, []client.ObjectKey{key}, nil, snapshotRouteObjectKeys{}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotGatewayListenersReconcileRequestName); ok {
		return snapshotBuildScopeGatewayListeners, "", "", []client.ObjectKey{key}, nil, nil, snapshotRouteObjectKeys{}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotHTTPRoutesReconcileRequestName); ok {
		return snapshotBuildScopeRoutes, "", "", nil, nil, nil, snapshotRouteObjectKeys{http: []client.ObjectKey{key}}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotGRPCRoutesReconcileRequestName); ok {
		return snapshotBuildScopeRoutes, "", "", nil, nil, nil, snapshotRouteObjectKeys{grpc: []client.ObjectKey{key}}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotTCPRoutesReconcileRequestName); ok {
		return snapshotBuildScopeRoutes, "", "", nil, nil, nil, snapshotRouteObjectKeys{tcp: []client.ObjectKey{key}}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotUDPRoutesReconcileRequestName); ok {
		return snapshotBuildScopeRoutes, "", "", nil, nil, nil, snapshotRouteObjectKeys{udp: []client.ObjectKey{key}}
	}
	if key, ok := snapshotScopedObjectKeyForRequest(request, snapshotTLSRoutesReconcileRequestName); ok {
		return snapshotBuildScopeRoutes, "", "", nil, nil, nil, snapshotRouteObjectKeys{tls: []client.ObjectKey{key}}
	}

	switch request.Name {
	case snapshotBackendsReconcileRequestName:
		return snapshotBuildScopeBackends, "", "", nil, nil, nil, snapshotRouteObjectKeys{}
	case snapshotBackendsNamespaceRequestName:
		return snapshotBuildScopeBackends, "", request.Namespace, nil, nil, nil, snapshotRouteObjectKeys{}
	case snapshotBackendDependenciesRequestName:
		return snapshotBuildScopeBackends | snapshotBuildScopeRouteBackendRefs, "", "", nil, nil, nil, snapshotRouteObjectKeys{}
	case snapshotServiceDependenciesRequestName:
		return snapshotBuildScopeBackends | snapshotBuildScopeRouteBackendRefs | snapshotBuildScopeMeshListeners, "", "", nil, nil, nil, snapshotRouteObjectKeys{}
	case snapshotAttachmentsReconcileRequestName:
		return snapshotBuildScopeAttachments, request.Namespace, "", nil, nil, nil, snapshotRouteObjectKeys{}
	case snapshotWorkloadsReconcileRequestName:
		return snapshotBuildScopeWorkloads, "", "", nil, nil, nil, snapshotRouteObjectKeys{}
	default:
		return snapshotBuildScopeFull, "", "", nil, nil, nil, snapshotRouteObjectKeys{}
	}
}

func reconcilerRunnerScopesForSnapshotBuildScope(scope snapshotBuildScope) []ReconcilerRunnerScope {
	if scope == snapshotBuildScopeNone || scope == snapshotBuildScopeFull {
		return []ReconcilerRunnerScope{ReconcilerRunnerScopeFull}
	}

	scopes := runnerScopeSet{}
	if scope&snapshotBuildScopeGatewayListeners != 0 {
		scopes.merge(ReconcilerRunnerScopeInfra, ReconcilerRunnerScopeGatewayStatus, ReconcilerRunnerScopeRouteStatus)
	}
	if scope&snapshotBuildScopeAttachments != 0 {
		scopes.merge(ReconcilerRunnerScopeGatewayStatus, ReconcilerRunnerScopeRouteStatus)
	}
	if scope&snapshotBuildScopeRoutes != 0 {
		scopes.merge(
			ReconcilerRunnerScopeInfra,
			ReconcilerRunnerScopeGatewayStatus,
			ReconcilerRunnerScopeRouteStatus,
			ReconcilerRunnerScopePolicyStatus,
		)
	}
	if scope&snapshotBuildScopeBackends != 0 || scope&snapshotBuildScopeRouteBackendRefs != 0 {
		scopes.merge(ReconcilerRunnerScopeRouteStatus, ReconcilerRunnerScopePolicyStatus)
	}
	if scope&snapshotBuildScopeMeshListeners != 0 || scope&snapshotBuildScopeWorkloads != 0 {
		scopes.merge(ReconcilerRunnerScopeInfra, ReconcilerRunnerScopeGatewayStatus, ReconcilerRunnerScopeRouteStatus)
	}

	return scopes.sortedOrFull()
}

func (s *Syncer) buildSnapshot(
	ctx context.Context,
	scope snapshotBuildScope,
	attachmentNamespaces []string,
	backendNamespaces []string,
	gatewayKeys []client.ObjectKey,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	routeKeys snapshotRouteObjectKeys,
) (*ir.Snapshot, error) {
	if scope == snapshotBuildScopeNone || scope == snapshotBuildScopeFull {
		return s.translator.Build(ctx, s.client)
	}

	current := s.store.Current()
	if current == nil {
		return s.translator.Build(ctx, s.client)
	}

	next := current
	var err error
	if scope&snapshotBuildScopeRoutes != 0 {
		next, err = s.translator.BuildRoutesForSnapshot(
			ctx,
			s.client,
			next,
			routeKeys.http,
			routeKeys.grpc,
			routeKeys.tcp,
			routeKeys.udp,
			routeKeys.tls,
		)
		if err != nil {
			return nil, err
		}
	}
	if scope&snapshotBuildScopeBackends != 0 {
		var backends []ir.BackendCluster
		if len(serviceKeys) != 0 || len(serviceImportKeys) != 0 {
			backends, err = s.translator.BuildBackendsForSnapshot(ctx, s.client, next, serviceKeys, serviceImportKeys)
		} else if len(backendNamespaces) != 0 {
			backends, err = s.translator.BuildBackendsForNamespaces(ctx, s.client, next, backendNamespaces)
		} else {
			backends, err = s.translator.BuildBackends(ctx, s.client)
		}
		if err != nil {
			return nil, err
		}
		next = translator.ApplyPartialSnapshot(next, backends, nil)
	}
	if scope&snapshotBuildScopeRouteBackendRefs != 0 {
		httpRoutes, grpcRoutes, streamRoutes, err := s.translator.RefreshBackendRefMetadataForBackends(
			ctx,
			s.client,
			next,
			serviceKeys,
			serviceImportKeys,
			backendNamespaces,
		)
		if err != nil {
			return nil, err
		}
		next = translator.ApplyPartialSnapshot(next, nil, nil)
		next.HTTPRoutes = httpRoutes
		next.GRPCRoutes = grpcRoutes
		next.StreamRoutes = streamRoutes
	}
	if scope&snapshotBuildScopeMeshListeners != 0 {
		listeners, err := s.translator.RebuildMeshServiceListeners(ctx, s.client, next)
		if err != nil {
			return nil, err
		}
		next = translator.ApplyPartialSnapshot(next, nil, listeners)
	}
	if scope&snapshotBuildScopeWorkloads != 0 {
		workloads, err := s.translator.BuildWorkloadsForSnapshot(ctx, s.client, next)
		if err != nil {
			return nil, err
		}
		next = translator.ApplyPartialSnapshot(next, nil, nil)
		next.Workloads = workloads
	}
	if scope&snapshotBuildScopeGatewayListeners != 0 {
		next, err = s.translator.BuildGatewayListenersForSnapshot(ctx, s.client, next, gatewayKeys)
		if err != nil {
			return nil, err
		}
	}
	if scope&snapshotBuildScopeAttachments != 0 {
		listeners, err := s.translator.RebuildAttachmentsForNamespaces(ctx, s.client, next, attachmentNamespaces)
		if err != nil {
			return nil, err
		}
		next = translator.ApplyPartialSnapshot(next, nil, listeners)
	}
	return next, nil
}

func (s *Syncer) mergePendingBuildLocked(
	scope snapshotBuildScope,
	attachmentNamespace string,
	backendNamespace string,
	gatewayKeys []client.ObjectKey,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	routeKeys snapshotRouteObjectKeys,
) {
	s.settlePending.merge(
		scope,
		attachmentNamespaces(attachmentNamespace),
		backendNamespaces(backendNamespace),
		gatewayKeys,
		serviceKeys,
		serviceImportKeys,
		routeKeys,
	)
}

func (s *Syncer) consumePendingBuild() (snapshotBuildScope, []string, []string, []client.ObjectKey, []client.ObjectKey, []client.ObjectKey, snapshotRouteObjectKeys) {
	s.settleMu.Lock()
	defer s.settleMu.Unlock()

	return s.settlePending.consume()
}

func (s *Syncer) clearPendingBuild() {
	s.settleMu.Lock()
	defer s.settleMu.Unlock()
	s.clearPendingBuildLocked()
}

func (s *Syncer) clearPendingBuildLocked() {
	s.settlePending.clear()
}

func (s *Syncer) mergeRetryPendingBuild(
	scope snapshotBuildScope,
	attachmentNamespaces []string,
	backendNamespaces []string,
	gatewayKeys []client.ObjectKey,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	routeKeys snapshotRouteObjectKeys,
) {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()

	s.retryPending.merge(
		scope,
		attachmentNamespaces,
		backendNamespaces,
		gatewayKeys,
		serviceKeys,
		serviceImportKeys,
		routeKeys,
	)
}

func (s *Syncer) consumeRetryPendingBuild() (snapshotBuildScope, []string, []string, []client.ObjectKey, []client.ObjectKey, []client.ObjectKey, snapshotRouteObjectKeys) {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()

	return s.retryPending.consume()
}

func (p *snapshotPendingBuild) merge(
	scope snapshotBuildScope,
	attachmentNamespaces []string,
	backendNamespaces []string,
	gatewayKeys []client.ObjectKey,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	routeKeys snapshotRouteObjectKeys,
) {
	if scope == snapshotBuildScopeNone {
		return
	}
	if scope == snapshotBuildScopeFull || p.scope == snapshotBuildScopeFull {
		p.clear()
		p.scope = snapshotBuildScopeFull
		return
	}

	p.scope |= scope
	if scope&snapshotBuildScopeAttachments != 0 {
		mergePendingNamespaces(&p.attachmentNamespaces, attachmentNamespaces)
	}
	if scope&snapshotBuildScopeBackends != 0 {
		mergePendingNamespaces(&p.backendNamespaces, backendNamespaces)
	}
	mergePendingObjectKeys(&p.gatewayKeys, gatewayKeys)
	mergePendingObjectKeys(&p.serviceKeys, serviceKeys)
	mergePendingObjectKeys(&p.serviceImportKeys, serviceImportKeys)
	mergePendingObjectKeys(&p.httpRouteKeys, routeKeys.http)
	mergePendingObjectKeys(&p.grpcRouteKeys, routeKeys.grpc)
	mergePendingObjectKeys(&p.tcpRouteKeys, routeKeys.tcp)
	mergePendingObjectKeys(&p.udpRouteKeys, routeKeys.udp)
	mergePendingObjectKeys(&p.tlsRouteKeys, routeKeys.tls)
}

func (p *snapshotPendingBuild) consume() (snapshotBuildScope, []string, []string, []client.ObjectKey, []client.ObjectKey, []client.ObjectKey, snapshotRouteObjectKeys) {
	scope := p.scope
	attachmentNamespaces := sortedPendingNamespaces(p.attachmentNamespaces)
	backendNamespaces := sortedPendingNamespaces(p.backendNamespaces)
	gatewayKeys := sortedPendingObjectKeys(p.gatewayKeys)
	serviceKeys := sortedPendingObjectKeys(p.serviceKeys)
	serviceImportKeys := sortedPendingObjectKeys(p.serviceImportKeys)
	routeKeys := snapshotRouteObjectKeys{
		http: sortedPendingObjectKeys(p.httpRouteKeys),
		grpc: sortedPendingObjectKeys(p.grpcRouteKeys),
		tcp:  sortedPendingObjectKeys(p.tcpRouteKeys),
		udp:  sortedPendingObjectKeys(p.udpRouteKeys),
		tls:  sortedPendingObjectKeys(p.tlsRouteKeys),
	}
	p.clear()
	return scope, attachmentNamespaces, backendNamespaces, gatewayKeys, serviceKeys, serviceImportKeys, routeKeys
}

func (p *snapshotPendingBuild) clear() {
	p.scope = snapshotBuildScopeNone
	p.attachmentNamespaces = nil
	p.backendNamespaces = nil
	p.gatewayKeys = nil
	p.serviceKeys = nil
	p.serviceImportKeys = nil
	p.httpRouteKeys = nil
	p.grpcRouteKeys = nil
	p.tcpRouteKeys = nil
	p.udpRouteKeys = nil
	p.tlsRouteKeys = nil
}

func sortedPendingNamespaces(items map[string]struct{}) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func attachmentNamespaces(namespace string) []string {
	if namespace == "" {
		return nil
	}
	return []string{namespace}
}

func backendNamespaces(namespace string) []string {
	if namespace == "" {
		return nil
	}
	return []string{namespace}
}

func snapshotScopedObjectKeyForRequest(request ctrl.Request, baseName string) (client.ObjectKey, bool) {
	prefix := baseName + "/"
	if !strings.HasPrefix(request.Name, prefix) {
		return client.ObjectKey{}, false
	}
	name := strings.TrimPrefix(request.Name, prefix)
	if name == "" {
		return client.ObjectKey{}, false
	}
	return client.ObjectKey{Namespace: request.Namespace, Name: name}, true
}

func sortedPendingObjectKeys(items map[string]client.ObjectKey) []client.ObjectKey {
	if len(items) == 0 {
		return nil
	}
	out := make([]client.ObjectKey, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Namespace + "/" + out[i].Name
		right := out[j].Namespace + "/" + out[j].Name
		return left < right
	})
	return out
}

func mergePendingNamespaces(target *map[string]struct{}, items []string) {
	for _, item := range items {
		if item == "" {
			continue
		}
		if *target == nil {
			*target = make(map[string]struct{})
		}
		(*target)[item] = struct{}{}
	}
}

func mergePendingObjectKeys(target *map[string]client.ObjectKey, keys []client.ObjectKey) {
	for _, key := range keys {
		if key.Name == "" {
			continue
		}
		if *target == nil {
			*target = make(map[string]client.ObjectKey)
		}
		(*target)[key.Namespace+"/"+key.Name] = key
	}
}
