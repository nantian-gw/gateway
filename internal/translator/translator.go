package translator

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	gwroutepolicy "github.com/nantian-gw/gateway/internal/gatewayexp/routepolicy"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/resources"
	aiservicetranslator "github.com/nantian-gw/gateway/internal/translator/aiservice"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/listeners"
	"github.com/nantian-gw/gateway/internal/translator/policies"
	"github.com/nantian-gw/gateway/internal/translator/routepolicy"
	"github.com/nantian-gw/gateway/internal/translator/routes"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

type Translator struct {
	controllerName string
	logger         *slog.Logger
	limits         shared.Limits
}

const (
	listenerAddressesMetadataKey        = "nantian.dev/listener-addresses"
	listenerDisplayAddressesMetadataKey = "nantian.dev/display-addresses"
)

func New(controllerName string, logger *slog.Logger) *Translator {
	return NewWithOptions(controllerName, logger, shared.Options{})
}

func NewWithOptions(controllerName string, logger *slog.Logger, options shared.Options) *Translator {
	return &Translator{
		controllerName: controllerName,
		logger:         logger,
		limits:         shared.NormalizeLimits(options.Limits),
	}
}

func (t *Translator) ControllerName() string {
	if t == nil {
		return ""
	}
	return t.controllerName
}

func (t *Translator) loadRoutesAndServices(
	ctx context.Context,
	cl client.Client,
	filteredGateways *[]gatewayv1.Gateway,
	listenerSets *gatewayv1.ListenerSetList,
	httpRoutes *gatewayv1.HTTPRouteList,
	grpcRoutes *gatewayv1.GRPCRouteList,
	tcpRoutes *gatewayv1alpha2.TCPRouteList,
	udpRoutes *gatewayv1alpha2.UDPRouteList,
	tlsRoutes *gatewayv1alpha2.TLSRouteList,
	services *[]corev1.Service,
	serviceImports *[]mcsv1alpha1.ServiceImport,
	endpointSlices *[]discoveryv1.EndpointSlice,
) error {
	group, ctx := errgroup.WithContext(ctx)
	var svcList corev1.ServiceList
	var siList mcsv1alpha1.ServiceImportList
	var esList discoveryv1.EndpointSliceList

	group.Go(func() error {
		var err error
		*filteredGateways, err = t.loadFilteredGateways(ctx, cl)
		return err
	})
	group.Go(func() error { return cl.List(ctx, httpRoutes) })
	group.Go(func() error { return cl.List(ctx, grpcRoutes) })
	group.Go(func() error {
		return listOptional(ctx, cl, tcpRoutes)
	})
	group.Go(func() error {
		return listOptional(ctx, cl, udpRoutes)
	})
	group.Go(func() error {
		return listOptional(ctx, cl, tlsRoutes)
	})
	group.Go(func() error {
		return listOptional(ctx, cl, listenerSets)
	})
	group.Go(func() error {
		if err := cl.List(ctx, &svcList); err != nil {
			return err
		}
		*services = svcList.Items
		return nil
	})
	group.Go(func() error {
		if err := cl.List(ctx, &siList); err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
			return err
		}
		*serviceImports = siList.Items
		return nil
	})
	group.Go(func() error {
		if err := cl.List(ctx, &esList); err != nil {
			return err
		}
		*endpointSlices = esList.Items
		return nil
	})
	return group.Wait()
}

func listOptional(ctx context.Context, cl client.Client, list client.ObjectList) error {
	if err := cl.List(ctx, list); err != nil && !isOptionalResourceMissing(err) {
		return err
	}
	return nil
}

func (t *Translator) Build(ctx context.Context, cl client.Client) (snapshot *ir.Snapshot, err error) {
	tracer := otel.Tracer("github.com/nantian-gw/gateway/internal/translator")
	ctx, span := tracer.Start(ctx, "translator.build_snapshot")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		if snapshot != nil {
			span.SetAttributes(
				attribute.Int("translator.listener_count", len(snapshot.Listeners)),
				attribute.Int("translator.http_route_count", len(snapshot.HTTPRoutes)),
				attribute.Int("translator.grpc_route_count", len(snapshot.GRPCRoutes)),
				attribute.Int("translator.stream_route_count", len(snapshot.StreamRoutes)),
				attribute.Int("translator.backend_count", len(snapshot.Backends)),
				attribute.Int("translator.secret_count", len(snapshot.Secrets)),
			)
		}
		span.End()
	}()

	snapshot = &ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
	}

	var (
		filteredGateways    []gatewayv1.Gateway
		listenerSets        gatewayv1.ListenerSetList
		httpRoutes          gatewayv1.HTTPRouteList
		httpRouteRawFilters map[client.ObjectKey]routes.RawHTTPRouteFilterConfigs
		grpcRoutes          gatewayv1.GRPCRouteList
		tcpRoutes           gatewayv1alpha2.TCPRouteList
		udpRoutes           gatewayv1alpha2.UDPRouteList
		tlsRoutes           gatewayv1alpha2.TLSRouteList
		referenceGrants     []gatewayv1beta1.ReferenceGrant
		backendTLSPolicies  []gatewayv1alpha3.BackendTLSPolicy
		backendLBPolicies   []backend.BackendLBPolicy
		routePolicies       []gwroutepolicy.RoutePolicy
		aiServices          []aiservice.AIService
		tokenPolicies       []tokenpolicy.TokenPolicy
		wasmPlugins         []wasmplugin.WasmPlugin
		services            []corev1.Service
		serviceImports      []mcsv1alpha1.ServiceImport
		pods                []corev1.Pod
		endpointSlices      []discoveryv1.EndpointSlice
		supportObjects      translatorSupportObjects
		backendConfigMaps   []corev1.ConfigMap
		wasmConfigMaps      []corev1.ConfigMap
	)

	if err := t.loadRoutesAndServices(ctx, cl,
		&filteredGateways, &listenerSets, &httpRoutes, &grpcRoutes,
		&tcpRoutes, &udpRoutes, &tlsRoutes, &services, &serviceImports, &endpointSlices,
	); err != nil {
		shared.MetricTranslationErrors.WithLabelValues("routes").Inc()
		return nil, err
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		httpRouteRawFilters, err = routes.LoadHTTPRouteRawFilterConfigs(groupCtx, cl, httpRoutes.Items)
		return err
	})
	group.Go(func() error {
		var err error
		supportObjects, err = t.loadSupportObjects(
			groupCtx,
			cl,
			filteredGateways,
			listenerSets.Items,
			httpRoutes.Items,
			grpcRoutes.Items,
			tcpRoutes.Items,
			udpRoutes.Items,
			tlsRoutes.Items,
			nil,
		)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	extensionResolver := extfilter.NewResolver(supportObjects.configMaps)

	// Translate all routes in parallel — each route's translation is independent.
	transGroup, _ := errgroup.WithContext(ctx)
	snapshot.HTTPRoutes = make([]ir.HTTPRoute, len(httpRoutes.Items))
	for i := range httpRoutes.Items {
		transGroup.Go(func() error {
			snapshot.HTTPRoutes[i] = routes.TranslateHTTPRouteWithDefaultGateways(
				httpRoutes.Items[i],
				extensionResolver,
				httpRouteRawFilters[client.ObjectKeyFromObject(&httpRoutes.Items[i])],
				filteredGateways,
			)
			return nil
		})
	}
	snapshot.GRPCRoutes = make([]ir.GRPCRoute, len(grpcRoutes.Items))
	for i := range grpcRoutes.Items {
		transGroup.Go(func() error {
			snapshot.GRPCRoutes[i] = routes.TranslateGRPCRouteWithDefaultGateways(grpcRoutes.Items[i], extensionResolver, filteredGateways)
			return nil
		})
	}
	snapshot.StreamRoutes = make([]ir.StreamRoute, 0, len(tcpRoutes.Items)+len(udpRoutes.Items)+len(tlsRoutes.Items))
	tcpResults := make([]ir.StreamRoute, len(tcpRoutes.Items))
	for i := range tcpRoutes.Items {
		transGroup.Go(func() error {
			tcpResults[i] = routes.TranslateTCPRouteWithDefaultGateways(tcpRoutes.Items[i], filteredGateways)
			return nil
		})
	}
	udpResults := make([]ir.StreamRoute, len(udpRoutes.Items))
	for i := range udpRoutes.Items {
		transGroup.Go(func() error {
			udpResults[i] = routes.TranslateUDPRouteWithDefaultGateways(udpRoutes.Items[i], filteredGateways)
			return nil
		})
	}
	tlsStreamResults := make([]ir.StreamRoute, len(tlsRoutes.Items))
	for i := range tlsRoutes.Items {
		transGroup.Go(func() error {
			tlsStreamResults[i] = routes.TranslateTLSRouteWithDefaultGateways(tlsRoutes.Items[i], filteredGateways)
			return nil
		})
	}
	if err := transGroup.Wait(); err != nil {
		shared.MetricTranslationErrors.WithLabelValues("routes").Inc()
		return nil, err
	}
	snapshot.StreamRoutes = append(snapshot.StreamRoutes, tcpResults...)
	snapshot.StreamRoutes = append(snapshot.StreamRoutes, udpResults...)
	snapshot.StreamRoutes = append(snapshot.StreamRoutes, tlsStreamResults...)

	filteredServices := resources.FilterServices(services)
	serviceKeyMap := make(map[string]client.ObjectKey, len(filteredServices))
	for _, service := range filteredServices {
		key := client.ObjectKey{Namespace: service.Namespace, Name: service.Name}
		serviceKeyMap[shared.BackendObjectKey(key.Namespace, key.Name)] = key
	}
	serviceImportKeyMap := make(map[string]client.ObjectKey, len(serviceImports))
	for _, serviceImport := range serviceImports {
		key := client.ObjectKey{Namespace: serviceImport.Namespace, Name: serviceImport.Name}
		serviceImportKeyMap[shared.BackendObjectKey(key.Namespace, key.Name)] = key
	}
	backendNamespaces := sortedBackendPolicyNamespaces(serviceKeyMap, serviceImportKeyMap)
	referenceGrantNamespaces := mergeSortedStrings(
		referencedGatewayGrantNamespaces(filteredGateways),
		referencedBackendGrantNamespacesFromSnapshot(snapshot),
	)
	workloadNamespaces := meshWorkloadNamespacesFromSnapshot(snapshot)

	group, groupCtx = errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		referenceGrants, err = loadReferenceGrantsForNamespaces(groupCtx, cl, referenceGrantNamespaces)
		return err
	})
	group.Go(func() error {
		var err error
		backendTLSPolicies, err = loadBackendTLSPoliciesForNamespaces(
			groupCtx,
			cl,
			backendNamespaces,
			serviceKeyMap,
			serviceImportKeyMap,
		)
		if err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		var err error
		backendLBPolicies, err = loadBackendLBPoliciesForNamespaces(
			groupCtx,
			cl,
			backendNamespaces,
			serviceKeyMap,
			serviceImportKeyMap,
		)
		if err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		var list gwroutepolicy.RoutePolicyList
		if err := cl.List(groupCtx, &list); err != nil {
			if !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
				return err
			}
			return nil
		}
		routePolicies = list.Items
		return nil
	})
	group.Go(func() error {
		var list aiservice.AIServiceList
		if err := cl.List(groupCtx, &list); err != nil {
			if !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
				return err
			}
			return nil
		}
		aiServices = list.Items
		return nil
	})
	group.Go(func() error {
		var list tokenpolicy.TokenPolicyList
		if err := cl.List(groupCtx, &list); err != nil {
			if !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
				return err
			}
			return nil
		}
		tokenPolicies = list.Items
		return nil
	})
	group.Go(func() error {
		var list wasmplugin.WasmPluginList
		if err := cl.List(groupCtx, &list); err != nil {
			if !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
				return err
			}
			return nil
		}
		wasmPlugins = list.Items
		return nil
	})
	group.Go(func() error {
		var err error
		pods, err = loadPodsForNamespaces(groupCtx, cl, workloadNamespaces)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	group, groupCtx = errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		backendConfigMaps, err = loadConfigMaps(
			groupCtx,
			cl,
			referencedConfigMapKeys(nil, nil, nil, backendTLSPolicies),
		)
		return err
	})
	group.Go(func() error {
		var err error
		wasmConfigMaps, err = loadConfigMaps(
			groupCtx,
			cl,
			policies.ReferencedConfigMapKeysForWasmPlugins(wasmPlugins),
		)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	if err := t.limits.ValidateInputObjects(
		len(filteredGateways) +
			len(httpRoutes.Items) +
			len(grpcRoutes.Items) +
			len(tcpRoutes.Items) +
			len(udpRoutes.Items) +
			len(tlsRoutes.Items) +
			len(referenceGrants) +
			len(backendTLSPolicies) +
			len(backendLBPolicies) +
			len(routePolicies) +
			len(services) +
			len(serviceImports) +
			len(pods) +
			len(endpointSlices),
	); err != nil {
		return nil, err
	}

	filteredEndpointSlices := resources.FilterEndpointSlices(endpointSlices)
	mergedConfigMaps := mergeConfigMaps(supportObjects.configMaps, backendConfigMaps, wasmConfigMaps)
	indexes := shared.NewTranslatorIndexes(
		filteredServices,
		serviceImports,
		filteredEndpointSlices,
		supportObjects.secrets,
		mergedConfigMaps,
		referenceGrants,
	)

	namespaceByName := make(map[string]corev1.Namespace, len(supportObjects.namespaces))
	for _, ns := range supportObjects.namespaces {
		namespaceByName[ns.Name] = ns
	}
	for _, gateway := range filteredGateways {
		snapshot.Listeners = append(
			snapshot.Listeners,
			t.translateGatewayListenersWithIndexes(gateway, indexes, listenerSets.Items, namespaceByName)...,
		)
	}
	snapshot.Listeners = append(
		snapshot.Listeners,
		listeners.TranslateMeshServiceListeners(
			listeners.CollectMeshServiceFrontends(
				filteredServices,
				httpRoutes.Items,
				grpcRoutes.Items,
				tcpRoutes.Items,
				udpRoutes.Items,
				tlsRoutes.Items,
			),
		)...,
	)

	backendRefs := backends.NewBackendRefTranslator(
		filteredServices,
		serviceImports,
		referenceGrants,
		extfilter.NewResolver(mergedConfigMaps),
		func(filters []gatewayv1.HTTPRouteFilter, ns string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter {
			return routes.FiltersFromHTTPWithResolver(filters, ns, resolver, target, nil, 0)
		},
		func(filters []gatewayv1.GRPCRouteFilter, ns string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter {
			return routes.FiltersFromGRPCWithResolver(filters, ns, resolver, target)
		},
	)

	annotGroup, _ := errgroup.WithContext(ctx)
	for idx := range httpRoutes.Items {
		annotGroup.Go(func() error {
			backendRefs.AnnotateHTTPRoute(&snapshot.HTTPRoutes[idx], httpRoutes.Items[idx])
			return nil
		})
	}
	for idx := range snapshot.GRPCRoutes {
		annotGroup.Go(func() error {
			backendRefs.AnnotateGRPCRoute(&snapshot.GRPCRoutes[idx], grpcRoutes.Items[idx])
			return nil
		})
	}
	streamIdx := 0
	for idx := range tcpRoutes.Items {
		sIdx := streamIdx
		annotGroup.Go(func() error {
			backendRefs.AnnotateTCPRoute(&snapshot.StreamRoutes[sIdx], tcpRoutes.Items[idx])
			return nil
		})
		streamIdx++
	}
	for idx := range udpRoutes.Items {
		sIdx := streamIdx
		annotGroup.Go(func() error {
			backendRefs.AnnotateUDPRoute(&snapshot.StreamRoutes[sIdx], udpRoutes.Items[idx])
			return nil
		})
		streamIdx++
	}
	for idx := range tlsRoutes.Items {
		sIdx := streamIdx
		annotGroup.Go(func() error {
			backendRefs.AnnotateTLSRoute(&snapshot.StreamRoutes[sIdx], tlsRoutes.Items[idx])
			return nil
		})
		streamIdx++
	}
	_ = annotGroup.Wait()

	if len(routePolicies) > 0 {
		routePolicyIndexes := routepolicy.BuildRoutePolicyIndexes(routePolicies, snapshot.HTTPRoutes, filteredGateways)
		for i := range snapshot.HTTPRoutes {
			key := snapshot.HTTPRoutes[i].Namespace + "/" + snapshot.HTTPRoutes[i].Name
			if cfg, ok := routePolicyIndexes[key]; ok {
				snapshot.HTTPRoutes[i].RoutePolicy = cfg
			}
		}
		grpcRoutePolicyIndexes := routepolicy.BuildRoutePolicyIndexes(routePolicies, routepolicy.GrpcRoutesToHTTP(snapshot.GRPCRoutes), filteredGateways)
		for i := range snapshot.GRPCRoutes {
			key := snapshot.GRPCRoutes[i].Namespace + "/" + snapshot.GRPCRoutes[i].Name
			if cfg, ok := grpcRoutePolicyIndexes[key]; ok {
				snapshot.GRPCRoutes[i].RoutePolicy = cfg
			}
		}
	}

	snapshot.Backends = backends.TranslateBackendsWithIndexes(
		filteredServices,
		serviceImports,
		backendTLSPolicies,
		backendLBPolicies,
		t.limits.DefaultConnectTimeout,
		indexes,
	)
	aiServiceConfigs := aiservicetranslator.TranslateAll(aiServices)
	for i := range snapshot.Backends {
		key := shared.BackendObjectKey(snapshot.Backends[i].Namespace, snapshot.Backends[i].Name)
		if cfg, ok := aiServiceConfigs[key]; ok {
			cfgCopy := cfg
			snapshot.Backends[i].AIService = &cfgCopy
		}
	}
	routeBackends := policies.BuildRouteBackendServices(httpRoutes.Items)
	tokenPolicyConfigs := policies.TranslateTokenPolicies(tokenPolicies, policies.ServiceKeySet(filteredServices), policies.ServiceImportKeySet(serviceImports), routeBackends)
	for i := range snapshot.Backends {
		key := shared.BackendObjectKey(snapshot.Backends[i].Namespace, snapshot.Backends[i].Name)
		if cfg, ok := tokenPolicyConfigs[key]; ok {
			cfgCopy := cfg
			snapshot.Backends[i].TokenPolicy = &cfgCopy
		}
	}
	wasmPluginConfigs := policies.TranslateWasmPlugins(wasmPlugins, mergedConfigMaps, t.logger)
	for i := range snapshot.Backends {
		key := shared.BackendObjectKey(snapshot.Backends[i].Namespace, snapshot.Backends[i].Name)
		if cfg, ok := wasmPluginConfigs[key]; ok {
			cfgCopy := cfg
			snapshot.Backends[i].WasmPlugin = &cfgCopy
		}
	}
	snapshot.Workloads = translateWorkloads(pods)
	policies.AttachRoutes(snapshot, filteredGateways, supportObjects.namespaces, listenerSets.Items, gatewayAllowsListenerSet, mergeListenerSetListeners)
	snapshot.Secrets = filterSecretMaterialsByKeys(
		backends.TranslateSecrets(supportObjects.secrets),
		listenerSecretMaterialKeys(snapshot.Listeners),
	)
	if err := t.limits.ValidateSnapshot(snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func isOptionalResourceMissing(err error) bool {
	return meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)
}
