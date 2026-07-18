package translator

import (
	"context"
	"sort"
	"time"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/resources"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func (t *Translator) BuildBackends(ctx context.Context, cl client.Client) ([]ir.BackendCluster, error) {
	var (
		services           corev1.ServiceList
		serviceImports     mcsv1alpha1.ServiceImportList
		endpointSlices     discoveryv1.EndpointSliceList
		backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy
		backendLBPolicies  backend.BackendLBPolicyList
	)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		return cl.List(groupCtx, &services)
	})
	group.Go(func() error {
		if err := cl.List(groupCtx, &serviceImports); err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		return cl.List(groupCtx, &endpointSlices)
	})
	group.Go(func() error {
		var err error
		backendTLSPolicies, err = gatewayapi.ListBackendTLSPoliciesV1(groupCtx, cl)
		if err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		if err := cl.List(groupCtx, &backendLBPolicies); err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
			return err
		}
		return nil
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	configMaps, err := loadConfigMaps(
		ctx,
		cl,
		referencedConfigMapKeys(nil, nil, nil, backendTLSPolicies),
	)
	if err != nil {
		return nil, err
	}

	filteredServices := resources.FilterServices(services.Items)
	filteredEndpointSlices := resources.FilterEndpointSlices(endpointSlices.Items)
	indexes := shared.NewTranslatorIndexes(
		filteredServices,
		serviceImports.Items,
		filteredEndpointSlices,
		nil,
		configMaps,
		nil,
	)

	return backends.TranslateBackendsWithIndexes(
		filteredServices,
		serviceImports.Items,
		backendTLSPolicies,
		backendLBPolicies.Items,
		t.limits.DefaultConnectTimeout,
		indexes,
	), nil
}

func (t *Translator) RebuildAttachmentsForNamespaces(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	namespaces []string,
) ([]ir.Listener, error) {
	if current == nil || len(current.Listeners) == 0 || len(namespaces) == 0 {
		return cloneListeners(current.Listeners), nil
	}

	targetSet := make(map[string]struct{}, len(namespaces))
	namespaceByName := make(map[string]corev1.Namespace, len(namespaces))
	for _, name := range namespaces {
		if name == "" {
			continue
		}
		targetSet[name] = struct{}{}
		item := &corev1.Namespace{}
		if err := cl.Get(ctx, client.ObjectKey{Name: name}, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			namespaceByName[item.Name] = *item
		}
	}

	if len(targetSet) == 0 {
		return cloneListeners(current.Listeners), nil
	}

	listenerSets, err := loadAttachmentParentListenerSets(ctx, cl, current, targetSet)
	if err != nil {
		return nil, err
	}
	if err := loadAttachmentListenerSetNamespaces(ctx, cl, namespaceByName, listenerSets); err != nil {
		return nil, err
	}

	filteredGateways, err := t.loadAttachmentParentGateways(ctx, cl, current, targetSet, listenerSets)
	if err != nil {
		return nil, err
	}

	listeners := cloneListeners(current.Listeners)
	listenerIndexByName := make(map[string]int, len(listeners))
	serviceListeners := make(map[string][]ir.Listener)
	for idx, listener := range listeners {
		listenerIndexByName[listener.Name] = idx
		if listener.Metadata[mesh.FrontendKindMetadataKey] != mesh.FrontendKindService {
			continue
		}
		namespace := listener.Metadata[mesh.FrontendNamespaceMetadataKey]
		name := listener.Metadata[mesh.FrontendNameMetadataKey]
		if namespace == "" || name == "" {
			continue
		}
		serviceListeners[namespace+"/"+name] = append(serviceListeners[namespace+"/"+name], listener)
	}

	routeKeysToReplace := attachmentRouteKeysForNamespaces(current, targetSet)
	for idx := range listeners {
		listeners[idx].AttachedRoutes = filterAttachedRoutes(listeners[idx].AttachedRoutes, routeKeysToReplace)
	}

	attachments := make(map[string]map[string]struct{})
	gatewayByKey := make(map[string]gatewayv1.Gateway, len(filteredGateways))
	for _, gateway := range filteredGateways {
		gatewayByKey[gateway.Namespace+"/"+gateway.Name] = gateway
	}
	listenerSetByKey, listenerSetGateway := listenerSetAttachmentMaps(listenerSets)

	for _, route := range current.HTTPRoutes {
		if _, ok := targetSet[route.Namespace]; !ok {
			continue
		}
		recordRouteAttachments(
			attachments,
			gatewayByKey,
			listenerSetByKey,
			listenerSetGateway,
			namespaceByName,
			route.Namespace,
			route.Name,
			attachmentRouteKindHTTP,
			route.Hostnames,
			route.ParentRefs,
			serviceListeners,
		)
	}
	for _, route := range current.GRPCRoutes {
		if _, ok := targetSet[route.Namespace]; !ok {
			continue
		}
		recordRouteAttachments(
			attachments,
			gatewayByKey,
			listenerSetByKey,
			listenerSetGateway,
			namespaceByName,
			route.Namespace,
			route.Name,
			attachmentRouteKindGRPC,
			route.Hostnames,
			route.ParentRefs,
			serviceListeners,
		)
	}
	for _, route := range current.StreamRoutes {
		if _, ok := targetSet[route.Namespace]; !ok {
			continue
		}
		recordRouteAttachments(
			attachments,
			gatewayByKey,
			listenerSetByKey,
			listenerSetGateway,
			namespaceByName,
			route.Namespace,
			route.Name,
			attachmentKindForStreamRoute(route.Kind),
			streamRouteHostnames(route),
			route.ParentRefs,
			serviceListeners,
		)
	}

	for listenerName, routeKeys := range attachments {
		index, ok := listenerIndexByName[listenerName]
		if !ok {
			continue
		}
		existing := make(map[string]struct{}, len(listeners[index].AttachedRoutes)+len(routeKeys))
		for _, routeKey := range listeners[index].AttachedRoutes {
			existing[routeKey] = struct{}{}
		}
		for routeKey := range routeKeys {
			existing[routeKey] = struct{}{}
		}
		listeners[index].AttachedRoutes = sortedKeys(existing)
	}

	return listeners, nil
}

func (t *Translator) loadAttachmentParentGateways(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	targetSet map[string]struct{},
	listenerSets []gatewayv1.ListenerSet,
) ([]gatewayv1.Gateway, error) {
	gatewayKeys := attachmentParentGatewayObjectKeys(current, targetSet, listenerSets)
	if len(gatewayKeys) == 0 {
		return nil, nil
	}

	gateways, err := loadGateways(ctx, cl, gatewayKeys)
	if err != nil {
		return nil, err
	}
	return t.filterGatewaysByManagedClasses(ctx, cl, gateways)
}

func loadAttachmentParentListenerSets(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	targetSet map[string]struct{},
) ([]gatewayv1.ListenerSet, error) {
	keys := attachmentParentListenerSetObjectKeys(current, targetSet)
	if len(keys) == 0 {
		return nil, nil
	}

	out := make([]gatewayv1.ListenerSet, 0, len(keys))
	for _, key := range keys {
		var listenerSet gatewayv1.ListenerSet
		if err := cl.Get(ctx, key, &listenerSet); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if listenerSet.Name != "" {
			out = append(out, listenerSet)
		}
	}
	return out, nil
}

func loadAttachmentListenerSetNamespaces(
	ctx context.Context,
	cl client.Client,
	namespaceByName map[string]corev1.Namespace,
	listenerSets []gatewayv1.ListenerSet,
) error {
	for _, listenerSet := range listenerSets {
		if listenerSet.Namespace == "" {
			continue
		}
		if _, ok := namespaceByName[listenerSet.Namespace]; ok {
			continue
		}
		var namespace corev1.Namespace
		if err := cl.Get(ctx, client.ObjectKey{Name: listenerSet.Namespace}, &namespace); client.IgnoreNotFound(err) != nil {
			return err
		}
		if namespace.Name != "" {
			namespaceByName[namespace.Name] = namespace
		}
	}
	return nil
}

func listenerSetAttachmentMaps(
	listenerSets []gatewayv1.ListenerSet,
) (map[string]gatewayv1.ListenerSet, map[string]string) {
	listenerSetByKey := make(map[string]gatewayv1.ListenerSet, len(listenerSets))
	listenerSetGateway := make(map[string]string, len(listenerSets))
	for _, listenerSet := range listenerSets {
		key := listenerSet.Namespace + "/" + listenerSet.Name
		listenerSetByKey[key] = listenerSet
		parentNamespace := shared.NamespaceOrDefault(listenerSet.Spec.ParentRef.Namespace, listenerSet.Namespace)
		listenerSetGateway[key] = parentNamespace + "/" + string(listenerSet.Spec.ParentRef.Name)
	}
	return listenerSetByKey, listenerSetGateway
}

func (t *Translator) RefreshBackendRefMetadata(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
) ([]ir.HTTPRoute, []ir.GRPCRoute, []ir.StreamRoute, error) {
	return t.refreshBackendRefMetadataForSnapshot(ctx, cl, current)
}

func (t *Translator) RefreshBackendRefMetadataForBackends(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
	backendNamespaces []string,
) ([]ir.HTTPRoute, []ir.GRPCRoute, []ir.StreamRoute, error) {
	if current == nil {
		return nil, nil, nil, nil
	}

	if len(serviceKeys) == 0 && len(serviceImportKeys) == 0 && len(backendNamespaces) == 0 {
		return t.refreshBackendRefMetadataForSnapshot(ctx, cl, current)
	}

	routeKeys, httpRoutes, grpcRoutes, streamRoutes := affectedBackendRefRoutes(
		current,
		serviceKeys,
		serviceImportKeys,
		backendNamespaces,
	)
	if len(routeKeys.http) == 0 && len(routeKeys.grpc) == 0 && len(routeKeys.tcp) == 0 && len(routeKeys.udp) == 0 && len(routeKeys.tls) == 0 {
		next := current.Clone()
		return next.HTTPRoutes, next.GRPCRoutes, next.StreamRoutes, nil
	}

	refreshedHTTPRoutes, refreshedGRPCRoutes, refreshedStreamRoutes, err := t.refreshBackendRefMetadataForSnapshot(
		ctx,
		cl,
		&ir.Snapshot{
			HTTPRoutes:   httpRoutes,
			GRPCRoutes:   grpcRoutes,
			StreamRoutes: streamRoutes,
		},
	)
	if err != nil {
		return nil, nil, nil, err
	}

	next := current.Clone()
	next.HTTPRoutes = mergePartialHTTPRoutes(current.HTTPRoutes, objectKeyMap(routeKeys.http), refreshedHTTPRoutes)
	next.GRPCRoutes = mergePartialGRPCRoutes(current.GRPCRoutes, objectKeyMap(routeKeys.grpc), refreshedGRPCRoutes)
	next.StreamRoutes = mergePartialStreamRoutes(
		current.StreamRoutes,
		streamRouteReplacementKeyMap(routeKeys.tcp, "TCP"),
		streamRouteReplacementKeyMap(routeKeys.udp, "UDP"),
		streamRouteReplacementKeyMap(routeKeys.tls, "TLS"),
		refreshedStreamRoutes,
	)
	return next.HTTPRoutes, next.GRPCRoutes, next.StreamRoutes, nil
}

func (t *Translator) refreshBackendRefMetadataForSnapshot(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
) ([]ir.HTTPRoute, []ir.GRPCRoute, []ir.StreamRoute, error) {
	if current == nil {
		return nil, nil, nil, nil
	}

	var (
		services        []corev1.Service
		serviceImports  []mcsv1alpha1.ServiceImport
		referenceGrants []gatewayv1beta1.ReferenceGrant
	)
	backendKeys := referencedBackendObjectKeysFromSnapshot(current)
	referenceGrantNamespaces := referencedBackendGrantNamespacesFromSnapshot(current)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		services, err = loadServices(groupCtx, cl, backendKeys.services)
		return err
	})
	group.Go(func() error {
		var err error
		serviceImports, err = loadServiceImports(groupCtx, cl, backendKeys.serviceImports)
		if err != nil {
			return err
		}
		return nil
	})
	group.Go(func() error {
		var err error
		referenceGrants, err = loadReferenceGrantsForNamespaces(
			groupCtx,
			cl,
			referenceGrantNamespaces,
		)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, nil, nil, err
	}

	annotator := backends.NewBackendRefTranslator(
		services,
		serviceImports,
		referenceGrants,
		extfilter.Resolver{},
		func(filters []gatewayv1.HTTPRouteFilter, ns string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter {
			return filtersFromHTTPWithResolver(filters, ns, resolver, target, nil, 0)
		},
		func(filters []gatewayv1.GRPCRouteFilter, ns string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter {
			return filtersFromGRPCWithResolver(filters, ns, resolver, target)
		},
	)
	next := current.Clone()
	refreshHTTPRouteBackendRefs(next.HTTPRoutes, annotator)
	refreshGRPCRouteBackendRefs(next.GRPCRoutes, annotator)
	refreshStreamRouteBackendRefs(next.StreamRoutes, annotator)
	return next.HTTPRoutes, next.GRPCRoutes, next.StreamRoutes, nil
}

type affectedBackendRefRouteKeys struct {
	http []client.ObjectKey
	grpc []client.ObjectKey
	tcp  []client.ObjectKey
	udp  []client.ObjectKey
	tls  []client.ObjectKey
}

func loadReferenceGrantsForNamespaces(
	ctx context.Context,
	cl client.Client,
	namespaces []string,
) ([]gatewayv1beta1.ReferenceGrant, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	grants := make([]gatewayv1beta1.ReferenceGrant, 0)
	for _, namespace := range namespaces {
		var list gatewayv1beta1.ReferenceGrantList
		if err := cl.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		grants = append(grants, list.Items...)
	}
	sort.Slice(grants, func(i, j int) bool {
		left := grants[i].Namespace + "/" + grants[i].Name
		right := grants[j].Namespace + "/" + grants[j].Name
		return left < right
	})
	return grants, nil
}

func (t *Translator) RebuildMeshServiceListeners(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
) ([]ir.Listener, error) {
	if current == nil {
		return nil, nil
	}
	services, err := loadServices(ctx, cl, meshParentServiceObjectKeysFromSnapshot(current))
	if err != nil {
		return nil, err
	}

	meshListeners := translateMeshServiceListeners(
		collectMeshServiceFrontendsFromSnapshot(
			resources.FilterServices(services),
			current,
		),
	)
	serviceListeners := make(map[string][]ir.Listener, len(meshListeners))
	for _, listener := range meshListeners {
		namespace := listener.Metadata[mesh.FrontendNamespaceMetadataKey]
		name := listener.Metadata[mesh.FrontendNameMetadataKey]
		if namespace == "" || name == "" {
			continue
		}
		serviceListeners[namespace+"/"+name] = append(serviceListeners[namespace+"/"+name], listener)
	}

	attachments := make(map[string]map[string]struct{})
	for _, route := range current.HTTPRoutes {
		recordRouteAttachments(
			attachments,
			nil,
			nil,
			nil,
			nil,
			route.Namespace,
			route.Name,
			attachmentRouteKindHTTP,
			route.Hostnames,
			route.ParentRefs,
			serviceListeners,
		)
	}
	for _, route := range current.GRPCRoutes {
		recordRouteAttachments(
			attachments,
			nil,
			nil,
			nil,
			nil,
			route.Namespace,
			route.Name,
			attachmentRouteKindGRPC,
			route.Hostnames,
			route.ParentRefs,
			serviceListeners,
		)
	}
	for _, route := range current.StreamRoutes {
		recordRouteAttachments(
			attachments,
			nil,
			nil,
			nil,
			nil,
			route.Namespace,
			route.Name,
			attachmentKindForStreamRoute(route.Kind),
			streamRouteHostnames(route),
			route.ParentRefs,
			serviceListeners,
		)
	}
	for idx := range meshListeners {
		meshListeners[idx].AttachedRoutes = sortedKeys(attachments[meshListeners[idx].Name])
	}

	listeners := make([]ir.Listener, 0, len(current.Listeners))
	for _, listener := range cloneListeners(current.Listeners) {
		if listener.Metadata[mesh.FrontendKindMetadataKey] == mesh.FrontendKindService {
			continue
		}
		listeners = append(listeners, listener)
	}
	return append(listeners, meshListeners...), nil
}

func (t *Translator) loadFilteredGateways(ctx context.Context, cl client.Client) ([]gatewayv1.Gateway, error) {
	gatewayClasses, err := listGatewayClassesForController(ctx, cl, t.controllerName)
	if err != nil {
		return nil, err
	}
	if len(gatewayClasses) == 0 {
		return nil, nil
	}

	filteredGateways := make([]gatewayv1.Gateway, 0)
	for _, gatewayClass := range gatewayClasses {
		gateways, err := listGatewaysForGatewayClass(ctx, cl, gatewayClass.Name)
		if err != nil {
			return nil, err
		}
		filteredGateways = append(filteredGateways, gateways...)
	}

	return filteredGateways, nil
}

func attachmentRouteKeysForNamespaces(snapshot *ir.Snapshot, targetSet map[string]struct{}) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, route := range snapshot.HTTPRoutes {
		if _, ok := targetSet[route.Namespace]; ok {
			keys[route.Namespace+"/"+route.Name] = struct{}{}
		}
	}
	for _, route := range snapshot.GRPCRoutes {
		if _, ok := targetSet[route.Namespace]; ok {
			keys[route.Namespace+"/"+route.Name] = struct{}{}
		}
	}
	for _, route := range snapshot.StreamRoutes {
		if _, ok := targetSet[route.Namespace]; ok {
			keys[route.Namespace+"/"+route.Name] = struct{}{}
		}
	}
	return keys
}

func attachmentParentGatewayObjectKeys(
	snapshot *ir.Snapshot,
	targetSet map[string]struct{},
	listenerSets []gatewayv1.ListenerSet,
) []client.ObjectKey {
	if snapshot == nil || len(targetSet) == 0 {
		return nil
	}

	keys := make(map[string]client.ObjectKey)
	add := func(routeNamespace string, parentRefs []ir.ParentRef) {
		if _, ok := targetSet[routeNamespace]; !ok {
			return
		}
		for _, parentRef := range parentRefs {
			if isServiceParentRef(parentRef) || parentRef.Name == "" {
				continue
			}
			if isListenerSetParentRef(parentRef) {
				continue
			}

			namespace := parentRef.Namespace
			if namespace == "" {
				namespace = routeNamespace
			}
			key := client.ObjectKey{
				Namespace: namespace,
				Name:      parentRef.Name,
			}
			keys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
		}
	}

	for _, route := range snapshot.HTTPRoutes {
		add(route.Namespace, route.ParentRefs)
	}
	for _, route := range snapshot.GRPCRoutes {
		add(route.Namespace, route.ParentRefs)
	}
	for _, route := range snapshot.StreamRoutes {
		add(route.Namespace, route.ParentRefs)
	}
	for _, listenerSet := range listenerSets {
		if string(listenerSet.Spec.ParentRef.Name) == "" {
			continue
		}
		key := client.ObjectKey{
			Namespace: shared.NamespaceOrDefault(listenerSet.Spec.ParentRef.Namespace, listenerSet.Namespace),
			Name:      string(listenerSet.Spec.ParentRef.Name),
		}
		keys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
	}

	return sortedObjectKeys(keys)
}

func attachmentParentListenerSetObjectKeys(
	snapshot *ir.Snapshot,
	targetSet map[string]struct{},
) []client.ObjectKey {
	if snapshot == nil || len(targetSet) == 0 {
		return nil
	}

	keys := make(map[string]client.ObjectKey)
	add := func(routeNamespace string, parentRefs []ir.ParentRef) {
		if _, ok := targetSet[routeNamespace]; !ok {
			return
		}
		for _, parentRef := range parentRefs {
			if !isListenerSetParentRef(parentRef) || parentRef.Name == "" {
				continue
			}
			namespace := parentRef.Namespace
			if namespace == "" {
				namespace = routeNamespace
			}
			key := client.ObjectKey{
				Namespace: namespace,
				Name:      parentRef.Name,
			}
			keys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
		}
	}

	for _, route := range snapshot.HTTPRoutes {
		add(route.Namespace, route.ParentRefs)
	}
	for _, route := range snapshot.GRPCRoutes {
		add(route.Namespace, route.ParentRefs)
	}
	for _, route := range snapshot.StreamRoutes {
		add(route.Namespace, route.ParentRefs)
	}

	return sortedObjectKeys(keys)
}

func filterAttachedRoutes(attached []string, routeKeysToReplace map[string]struct{}) []string {
	if len(attached) == 0 || len(routeKeysToReplace) == 0 {
		return append([]string(nil), attached...)
	}
	out := make([]string, 0, len(attached))
	for _, routeKey := range attached {
		if _, replace := routeKeysToReplace[routeKey]; replace {
			continue
		}
		out = append(out, routeKey)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedKeys(values map[string]struct{}) []string {
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

func copyStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func cloneListeners(items []ir.Listener) []ir.Listener {
	out := make([]ir.Listener, len(items))
	for i, l := range items {
		out[i] = l
		if l.Hostnames != nil {
			out[i].Hostnames = append([]string(nil), l.Hostnames...)
		}
		if l.AttachedRoutes != nil {
			out[i].AttachedRoutes = append([]string(nil), l.AttachedRoutes...)
		}
		if l.Addresses != nil {
			out[i].Addresses = append([]string(nil), l.Addresses...)
		}
		if l.Metadata != nil {
			m := make(map[string]string, len(l.Metadata))
			for k, v := range l.Metadata {
				m[k] = v
			}
			out[i].Metadata = m
		}
	}
	return out
}

func ApplyPartialSnapshot(
	current *ir.Snapshot,
	backends []ir.BackendCluster,
	listeners []ir.Listener,
) *ir.Snapshot {
	return ApplyPartialSnapshotWithSecrets(current, backends, listeners, nil)
}

func ApplyPartialSnapshotWithSecrets(
	current *ir.Snapshot,
	backends []ir.BackendCluster,
	listeners []ir.Listener,
	secrets []ir.SecretMaterial,
) *ir.Snapshot {
	if current == nil {
		return &ir.Snapshot{
			GeneratedAt: time.Now().UTC(),
			Backends:    backends,
			Listeners:   listeners,
			Secrets:     secrets,
		}
	}

	next := &ir.Snapshot{
		ID:           current.ID,
		GeneratedAt:  time.Now().UTC(),
		Listeners:    current.Listeners,
		HTTPRoutes:   current.HTTPRoutes,
		GRPCRoutes:   current.GRPCRoutes,
		StreamRoutes: current.StreamRoutes,
		Backends:     current.Backends,
		Secrets:      current.Secrets,
		Workloads:    current.Workloads,
	}
	if backends != nil {
		next.Backends = backends
	}
	if listeners != nil {
		next.Listeners = listeners
	}
	if secrets != nil {
		next.Secrets = secrets
	}
	return next
}
