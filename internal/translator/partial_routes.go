package translator

import (
	"context"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/resources"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func (t *Translator) BuildRoutesForSnapshot(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	httpKeys []client.ObjectKey,
	grpcKeys []client.ObjectKey,
	tcpKeys []client.ObjectKey,
	udpKeys []client.ObjectKey,
	tlsKeys []client.ObjectKey,
) (*ir.Snapshot, error) {
	if current == nil {
		return t.Build(ctx, cl)
	}
	if len(httpKeys) == 0 && len(grpcKeys) == 0 && len(tcpKeys) == 0 && len(udpKeys) == 0 && len(tlsKeys) == 0 {
		return ApplyPartialSnapshot(current, nil, nil), nil
	}

	var (
		filteredGateways []gatewayv1.Gateway
		httpRoutes       []gatewayv1.HTTPRoute
		grpcRoutes       []gatewayv1.GRPCRoute
		tcpRoutes        []gatewayv1alpha2.TCPRoute
		udpRoutes        []gatewayv1alpha2.UDPRoute
		tlsRoutes        []gatewayv1alpha2.TLSRoute
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		httpRoutes, err = loadHTTPRoutes(groupCtx, cl, httpKeys)
		return err
	})
	group.Go(func() error {
		var err error
		grpcRoutes, err = loadGRPCRoutes(groupCtx, cl, grpcKeys)
		return err
	})
	group.Go(func() error {
		var err error
		tcpRoutes, err = loadTCPRoutes(groupCtx, cl, tcpKeys)
		return err
	})
	group.Go(func() error {
		var err error
		udpRoutes, err = loadUDPRoutes(groupCtx, cl, udpKeys)
		return err
	})
	group.Go(func() error {
		var err error
		tlsRoutes, err = loadTLSRoutes(groupCtx, cl, tlsKeys)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	if routesUseDefaultGateways(httpRoutes, grpcRoutes, tcpRoutes, udpRoutes, tlsRoutes) || len(tlsRoutes) > 0 {
		var err error
		filteredGateways, err = t.loadFilteredGateways(ctx, cl)
		if err != nil {
			return nil, err
		}
	}

	httpRouteRawFilters, err := loadHTTPRouteRawFilterConfigs(ctx, cl, httpRoutes)
	if err != nil {
		return nil, err
	}

	configMaps, err := loadConfigMaps(
		ctx,
		cl,
		referencedConfigMapKeys(nil, httpRoutes, grpcRoutes, nil),
	)
	if err != nil {
		return nil, err
	}
	extensionResolver := extfilter.NewResolver(configMaps)

	updatedHTTPRoutes := make([]ir.HTTPRoute, 0, len(httpRoutes))
	for _, route := range httpRoutes {
		updatedHTTPRoutes = append(updatedHTTPRoutes, translateHTTPRouteWithDefaultGateways(
			route,
			extensionResolver,
			httpRouteRawFilters[client.ObjectKeyFromObject(&route)],
			filteredGateways,
		))
	}
	updatedGRPCRoutes := make([]ir.GRPCRoute, 0, len(grpcRoutes))
	for _, route := range grpcRoutes {
		updatedGRPCRoutes = append(updatedGRPCRoutes, translateGRPCRouteWithDefaultGateways(route, extensionResolver, filteredGateways))
	}
	updatedStreamRoutes := make([]ir.StreamRoute, 0, len(tcpRoutes)+len(udpRoutes)+len(tlsRoutes))
	for _, route := range tcpRoutes {
		updatedStreamRoutes = append(updatedStreamRoutes, translateTCPRouteWithDefaultGateways(route, filteredGateways))
	}
	for _, route := range udpRoutes {
		updatedStreamRoutes = append(updatedStreamRoutes, translateUDPRouteWithDefaultGateways(route, filteredGateways))
	}
	for _, route := range tlsRoutes {
		updatedStreamRoutes = append(updatedStreamRoutes, translateTLSRouteWithDefaultGateways(route, filteredGateways))
	}

	partialRoutes := &ir.Snapshot{
		HTTPRoutes:   updatedHTTPRoutes,
		GRPCRoutes:   updatedGRPCRoutes,
		StreamRoutes: updatedStreamRoutes,
	}
	backendKeys := referencedBackendObjectKeysFromSnapshot(partialRoutes)
	referenceGrantNamespaces := referencedBackendGrantNamespacesFromSnapshot(partialRoutes)

	var (
		services        []corev1.Service
		serviceImports  []mcsv1alpha1.ServiceImport
		referenceGrants []gatewayv1beta1.ReferenceGrant
	)
	group, groupCtx = errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		services, err = loadServices(groupCtx, cl, backendKeys.services)
		return err
	})
	group.Go(func() error {
		var err error
		serviceImports, err = loadServiceImports(groupCtx, cl, backendKeys.serviceImports)
		return err
	})
	group.Go(func() error {
		var err error
		referenceGrants, err = loadReferenceGrantsForNamespaces(groupCtx, cl, referenceGrantNamespaces)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	annotator := backends.NewBackendRefTranslator(
		resources.FilterServices(services),
		serviceImports,
		referenceGrants,
		extensionResolver,
		func(filters []gatewayv1.HTTPRouteFilter, ns string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter {
			return filtersFromHTTPWithResolver(filters, ns, resolver, target, nil, 0)
		},
		func(filters []gatewayv1.GRPCRouteFilter, ns string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter {
			return filtersFromGRPCWithResolver(filters, ns, resolver, target)
		},
	)
	for idx := range httpRoutes {
		annotator.AnnotateHTTPRoute(&updatedHTTPRoutes[idx], httpRoutes[idx])
	}
	for idx := range updatedGRPCRoutes {
		annotator.AnnotateGRPCRoute(&updatedGRPCRoutes[idx], grpcRoutes[idx])
	}
	streamIndex := 0
	for idx := range tcpRoutes {
		annotator.AnnotateTCPRoute(&updatedStreamRoutes[streamIndex], tcpRoutes[idx])
		streamIndex++
	}
	for idx := range udpRoutes {
		annotator.AnnotateUDPRoute(&updatedStreamRoutes[streamIndex], udpRoutes[idx])
		streamIndex++
	}
	for idx := range tlsRoutes {
		annotator.AnnotateTLSRoute(&updatedStreamRoutes[streamIndex], tlsRoutes[idx])
		streamIndex++
	}

	next := ApplyPartialSnapshot(current, nil, nil)
	next.HTTPRoutes = mergePartialHTTPRoutes(current.HTTPRoutes, objectKeyMap(httpKeys), updatedHTTPRoutes)
	next.GRPCRoutes = mergePartialGRPCRoutes(current.GRPCRoutes, objectKeyMap(grpcKeys), updatedGRPCRoutes)
	next.StreamRoutes = mergePartialStreamRoutes(
		current.StreamRoutes,
		streamRouteReplacementKeyMap(tcpKeys, "TCP"),
		streamRouteReplacementKeyMap(udpKeys, "UDP"),
		streamRouteReplacementKeyMap(tlsKeys, "TLS"),
		updatedStreamRoutes,
	)
	replacementBackendKeys := referencedBackendReplacementKeysForRouteChanges(
		current,
		updatedHTTPRoutes,
		updatedGRPCRoutes,
		updatedStreamRoutes,
	)
	if len(replacementBackendKeys) != 0 {
		referencedKeys := referencedBackendObjectKeysFromSnapshot(next)
		serviceKeys, serviceImportKeys := filterReferencedBackendKeysByReplacementSet(
			referencedKeys,
			replacementBackendKeys,
		)
		backends, err := t.buildBackendsForKeyMaps(
			ctx,
			cl,
			next,
			replacementBackendKeys,
			serviceKeys,
			serviceImportKeys,
		)
		if err != nil {
			return nil, err
		}
		next = ApplyPartialSnapshot(next, backends, nil)
	}

	currentWorkloadNamespaces := meshWorkloadNamespacesFromSnapshot(current)
	nextWorkloadNamespaces := meshWorkloadNamespacesFromSnapshot(next)
	if !sortedStringSlicesEqual(currentWorkloadNamespaces, nextWorkloadNamespaces) {
		pods, err := loadPodsForNamespaces(ctx, cl, nextWorkloadNamespaces)
		if err != nil {
			return nil, err
		}
		next.Workloads = translateWorkloads(pods)
	}

	if partialRouteChangesAffectMesh(current, httpKeys, grpcKeys, tcpKeys, udpKeys, tlsKeys, updatedHTTPRoutes, updatedGRPCRoutes, updatedStreamRoutes) {
		listeners, err := t.RebuildMeshServiceListeners(ctx, cl, next)
		if err != nil {
			return nil, err
		}
		next = ApplyPartialSnapshot(next, nil, listeners)
	}

	attachmentNamespaces := routeChangeNamespaces(httpKeys, grpcKeys, tcpKeys, udpKeys, tlsKeys)
	if len(attachmentNamespaces) != 0 {
		targetSet := make(map[string]struct{}, len(attachmentNamespaces))
		for _, namespace := range attachmentNamespaces {
			if namespace == "" {
				continue
			}
			targetSet[namespace] = struct{}{}
		}

		missingGatewayKeyMap := objectKeyMap(missingParentGatewayListenerObjectKeys(next, attachmentNamespaces))
		if len(targetSet) != 0 {
			listenerSets, loadErr := loadAttachmentParentListenerSets(ctx, cl, next, targetSet)
			if loadErr != nil {
				return nil, loadErr
			}
			for _, key := range attachmentParentGatewayObjectKeys(next, targetSet, listenerSets) {
				missingGatewayKeyMap[shared.BackendObjectKey(key.Namespace, key.Name)] = key
			}
		}

		if len(missingGatewayKeyMap) != 0 {
			next, err = t.BuildGatewayListenersForSnapshot(ctx, cl, next, sortedObjectKeys(missingGatewayKeyMap))
			if err != nil {
				return nil, err
			}
		}
		listeners, err := t.RebuildAttachmentsForNamespaces(ctx, cl, next, attachmentNamespaces)
		if err != nil {
			return nil, err
		}
		next = ApplyPartialSnapshot(next, nil, listeners)
	}

	return next, nil
}

func loadHTTPRoutes(ctx context.Context, cl client.Client, keys []client.ObjectKey) ([]gatewayv1.HTTPRoute, error) {
	out := make([]gatewayv1.HTTPRoute, 0, len(keys))
	for _, key := range keys {
		item := &gatewayv1.HTTPRoute{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadGRPCRoutes(ctx context.Context, cl client.Client, keys []client.ObjectKey) ([]gatewayv1.GRPCRoute, error) {
	out := make([]gatewayv1.GRPCRoute, 0, len(keys))
	for _, key := range keys {
		item := &gatewayv1.GRPCRoute{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadTCPRoutes(ctx context.Context, cl client.Client, keys []client.ObjectKey) ([]gatewayv1alpha2.TCPRoute, error) {
	out := make([]gatewayv1alpha2.TCPRoute, 0, len(keys))
	for _, key := range keys {
		item := &gatewayv1alpha2.TCPRoute{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadUDPRoutes(ctx context.Context, cl client.Client, keys []client.ObjectKey) ([]gatewayv1alpha2.UDPRoute, error) {
	out := make([]gatewayv1alpha2.UDPRoute, 0, len(keys))
	for _, key := range keys {
		item := &gatewayv1alpha2.UDPRoute{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadTLSRoutes(ctx context.Context, cl client.Client, keys []client.ObjectKey) ([]gatewayv1alpha2.TLSRoute, error) {
	out := make([]gatewayv1alpha2.TLSRoute, 0, len(keys))
	for _, key := range keys {
		item := &gatewayv1alpha2.TLSRoute{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func routesUseDefaultGateways(
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
	tcpRoutes []gatewayv1alpha2.TCPRoute,
	udpRoutes []gatewayv1alpha2.UDPRoute,
	tlsRoutes []gatewayv1alpha2.TLSRoute,
) bool {
	for _, route := range httpRoutes {
		if gatewayapi.UsesDefaultGateways(route.Spec.UseDefaultGateways) {
			return true
		}
	}
	for _, route := range grpcRoutes {
		if gatewayapi.UsesDefaultGateways(route.Spec.UseDefaultGateways) {
			return true
		}
	}
	for _, route := range tcpRoutes {
		if gatewayapi.UsesDefaultGateways(route.Spec.UseDefaultGateways) {
			return true
		}
	}
	for _, route := range udpRoutes {
		if gatewayapi.UsesDefaultGateways(route.Spec.UseDefaultGateways) {
			return true
		}
	}
	for _, route := range tlsRoutes {
		if gatewayapi.UsesDefaultGateways(route.Spec.UseDefaultGateways) {
			return true
		}
	}
	return false
}

func mergePartialHTTPRoutes(
	current []ir.HTTPRoute,
	replacementKeys map[string]client.ObjectKey,
	updated []ir.HTTPRoute,
) []ir.HTTPRoute {
	out := make([]ir.HTTPRoute, 0, len(current)+len(updated))
	for _, route := range current {
		if _, replace := replacementKeys[shared.BackendObjectKey(route.Namespace, route.Name)]; replace {
			continue
		}
		out = append(out, route)
	}
	out = append(out, updated...)
	return out
}

func mergePartialGRPCRoutes(
	current []ir.GRPCRoute,
	replacementKeys map[string]client.ObjectKey,
	updated []ir.GRPCRoute,
) []ir.GRPCRoute {
	out := make([]ir.GRPCRoute, 0, len(current)+len(updated))
	for _, route := range current {
		if _, replace := replacementKeys[shared.BackendObjectKey(route.Namespace, route.Name)]; replace {
			continue
		}
		out = append(out, route)
	}
	out = append(out, updated...)
	return out
}

func mergePartialStreamRoutes(
	current []ir.StreamRoute,
	tcpKeys map[string]struct{},
	udpKeys map[string]struct{},
	tlsKeys map[string]struct{},
	updated []ir.StreamRoute,
) []ir.StreamRoute {
	out := make([]ir.StreamRoute, 0, len(current)+len(updated))
	for _, route := range current {
		replacementKeys := streamRouteReplacementKeys(route.Kind, tcpKeys, udpKeys, tlsKeys)
		if replacementKeys != nil {
			if _, replace := replacementKeys[streamRouteReplacementKey(route.Kind, route.Namespace, route.Name)]; replace {
				continue
			}
		}
		out = append(out, route)
	}
	out = append(out, updated...)
	return out
}

func streamRouteReplacementKeys(
	kind string,
	tcpKeys map[string]struct{},
	udpKeys map[string]struct{},
	tlsKeys map[string]struct{},
) map[string]struct{} {
	switch kind {
	case "TCP":
		return tcpKeys
	case "UDP":
		return udpKeys
	case "TLS":
		return tlsKeys
	default:
		return nil
	}
}

func streamRouteReplacementKeyMap(keys []client.ObjectKey, kind string) map[string]struct{} {
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key.Name == "" {
			continue
		}
		out[streamRouteReplacementKey(kind, key.Namespace, key.Name)] = struct{}{}
	}
	return out
}

func streamRouteReplacementKey(kind, namespace, name string) string {
	return kind + "/" + namespace + "/" + name
}

func partialRouteChangesAffectMesh(
	current *ir.Snapshot,
	httpKeys []client.ObjectKey,
	grpcKeys []client.ObjectKey,
	tcpKeys []client.ObjectKey,
	udpKeys []client.ObjectKey,
	tlsKeys []client.ObjectKey,
	updatedHTTPRoutes []ir.HTTPRoute,
	updatedGRPCRoutes []ir.GRPCRoute,
	updatedStreamRoutes []ir.StreamRoute,
) bool {
	if routeSlicesUseServiceParents(updatedHTTPRoutes, updatedGRPCRoutes, updatedStreamRoutes) {
		return true
	}
	if current == nil {
		return false
	}

	httpKeySet := objectKeyMap(httpKeys)
	for _, route := range current.HTTPRoutes {
		if _, ok := httpKeySet[shared.BackendObjectKey(route.Namespace, route.Name)]; ok && routeHasServiceParentRefs(route.ParentRefs) {
			return true
		}
	}
	grpcKeySet := objectKeyMap(grpcKeys)
	for _, route := range current.GRPCRoutes {
		if _, ok := grpcKeySet[shared.BackendObjectKey(route.Namespace, route.Name)]; ok && routeHasServiceParentRefs(route.ParentRefs) {
			return true
		}
	}

	tcpKeySet := streamRouteReplacementKeyMap(tcpKeys, "TCP")
	udpKeySet := streamRouteReplacementKeyMap(udpKeys, "UDP")
	tlsKeySet := streamRouteReplacementKeyMap(tlsKeys, "TLS")
	for _, route := range current.StreamRoutes {
		replacementKeys := streamRouteReplacementKeys(route.Kind, tcpKeySet, udpKeySet, tlsKeySet)
		if replacementKeys == nil {
			continue
		}
		if _, ok := replacementKeys[streamRouteReplacementKey(route.Kind, route.Namespace, route.Name)]; ok && routeHasServiceParentRefs(route.ParentRefs) {
			return true
		}
	}
	return false
}

func routeSlicesUseServiceParents(
	httpRoutes []ir.HTTPRoute,
	grpcRoutes []ir.GRPCRoute,
	streamRoutes []ir.StreamRoute,
) bool {
	for _, route := range httpRoutes {
		if routeHasServiceParentRefs(route.ParentRefs) {
			return true
		}
	}
	for _, route := range grpcRoutes {
		if routeHasServiceParentRefs(route.ParentRefs) {
			return true
		}
	}
	for _, route := range streamRoutes {
		if routeHasServiceParentRefs(route.ParentRefs) {
			return true
		}
	}
	return false
}

func routeHasServiceParentRefs(parentRefs []ir.ParentRef) bool {
	for _, parentRef := range parentRefs {
		if isServiceParentRef(parentRef) {
			return true
		}
	}
	return false
}

func referencedBackendReplacementKeysForRouteChanges(
	current *ir.Snapshot,
	updatedHTTPRoutes []ir.HTTPRoute,
	updatedGRPCRoutes []ir.GRPCRoute,
	updatedStreamRoutes []ir.StreamRoute,
) map[string]client.ObjectKey {
	keys := make(map[string]client.ObjectKey)
	currentBackendKeys := objectKeyMap(backendCatalogObjectKeysFromSnapshot(current))

	add := func(ref ir.BackendRef) {
		if backendRefMarkedInvalid(ref) || ref.Name == "" || ref.Namespace == "" {
			return
		}

		if _, ok := backends.BackendKindForRef(ref.Group, ref.Kind); !ok {
			return
		}

		key := client.ObjectKey{Namespace: ref.Namespace, Name: ref.Name}
		lookupKey := shared.BackendObjectKey(key.Namespace, key.Name)
		if _, exists := currentBackendKeys[lookupKey]; exists {
			return
		}
		keys[lookupKey] = key
	}

	for _, route := range updatedHTTPRoutes {
		for _, rule := range route.Rules {
			for _, ref := range rule.BackendRefs {
				add(ref)
			}
		}
	}
	for _, route := range updatedGRPCRoutes {
		for _, rule := range route.Rules {
			for _, ref := range rule.BackendRefs {
				add(ref)
			}
		}
	}
	for _, route := range updatedStreamRoutes {
		for _, rule := range route.Rules {
			for _, ref := range rule.BackendRefs {
				add(ref)
			}
		}
	}

	return keys
}

func filterReferencedBackendKeysByReplacementSet(
	keys referencedBackendObjectKeys,
	allowed map[string]client.ObjectKey,
) (map[string]client.ObjectKey, map[string]client.ObjectKey) {
	serviceKeys := make(map[string]client.ObjectKey)
	serviceImportKeys := make(map[string]client.ObjectKey)

	for _, key := range keys.services {
		lookupKey := shared.BackendObjectKey(key.Namespace, key.Name)
		if _, ok := allowed[lookupKey]; ok {
			serviceKeys[lookupKey] = key
		}
	}
	for _, key := range keys.serviceImports {
		lookupKey := shared.BackendObjectKey(key.Namespace, key.Name)
		if _, ok := allowed[lookupKey]; ok {
			serviceImportKeys[lookupKey] = key
		}
	}

	return serviceKeys, serviceImportKeys
}

func backendRefMarkedInvalid(ref ir.BackendRef) bool {
	if len(ref.Metadata) == 0 {
		return false
	}
	return ref.Metadata[backends.BackendRefMetaValid] == "false"
}

func routeChangeNamespaces(groups ...[]client.ObjectKey) []string {
	namespaces := make(map[string]struct{})
	for _, group := range groups {
		for _, key := range group {
			if key.Namespace == "" {
				continue
			}
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

func missingParentGatewayListenerObjectKeys(
	snapshot *ir.Snapshot,
	namespaces []string,
) []client.ObjectKey {
	if snapshot == nil || len(namespaces) == 0 {
		return nil
	}

	targetSet := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		if namespace == "" {
			continue
		}
		targetSet[namespace] = struct{}{}
	}
	if len(targetSet) == 0 {
		return nil
	}

	gatewayKeys := attachmentParentGatewayObjectKeys(snapshot, targetSet, nil)
	if len(gatewayKeys) == 0 {
		return nil
	}

	presentPrefixes := make(map[string]struct{}, len(snapshot.Listeners))
	for _, listener := range snapshot.Listeners {
		parts := strings.SplitN(listener.Name, "/", 3)
		if len(parts) < 3 {
			continue
		}
		presentPrefixes[parts[0]+"/"+parts[1]+"/"] = struct{}{}
	}

	out := make([]client.ObjectKey, 0, len(gatewayKeys))
	for _, key := range gatewayKeys {
		prefix := key.Namespace + "/" + key.Name + "/"
		if _, ok := presentPrefixes[prefix]; ok {
			continue
		}
		out = append(out, key)
	}
	return out
}

func sortedStringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}
