package translator

import (
	"context"
	"log/slog"
	"time"

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

	"github.com/nantian-gw/gateway/internal/extensionfilter"
	aiservicev1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/aiservicev1alpha1"
	backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"
	tokenpolicyv1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/tokenpolicyv1alpha1"
	wasmpluginv1alpha1 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/wasmpluginv1alpha1"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/managedresources"
)

type Translator struct {
	controllerName string
	logger         *slog.Logger
	limits         Limits
}

const (
	listenerAddressesMetadataKey        = "nantian.dev/listener-addresses"
	listenerDisplayAddressesMetadataKey = "nantian.dev/display-addresses"
)

func New(controllerName string, logger *slog.Logger) *Translator {
	return NewWithOptions(controllerName, logger, Options{})
}

func NewWithOptions(controllerName string, logger *slog.Logger, options Options) *Translator {
	return &Translator{
		controllerName: controllerName,
		logger:         logger,
		limits:         normalizeLimits(options.Limits),
	}
}

func (t *Translator) ControllerName() string {
	if t == nil {
		return ""
	}
	return t.controllerName
}

func (t *Translator) Build(ctx context.Context, cl client.Client) (*ir.Snapshot, error) {
	snapshot := &ir.Snapshot{
		GeneratedAt: time.Now().UTC(),
	}

	var (
		filteredGateways    []gatewayv1.Gateway
		listenerSets        gatewayv1.ListenerSetList
		httpRoutes          gatewayv1.HTTPRouteList
		httpRouteRawFilters map[client.ObjectKey]rawHTTPRouteFilterConfigs
		grpcRoutes          gatewayv1.GRPCRouteList
		tcpRoutes           gatewayv1alpha2.TCPRouteList
		udpRoutes           gatewayv1alpha2.UDPRouteList
		tlsRoutes           gatewayv1alpha2.TLSRouteList
		referenceGrants     []gatewayv1beta1.ReferenceGrant
		backendTLSPolicies  []gatewayv1alpha3.BackendTLSPolicy
		backendLBPolicies   []backendlbv1alpha2.BackendLBPolicy
		aiServices          []aiservicev1alpha1.AIService
		tokenPolicies       []tokenpolicyv1alpha1.TokenPolicy
		wasmPlugins         []wasmpluginv1alpha1.WasmPlugin
		services            []corev1.Service
		serviceImports      []mcsv1alpha1.ServiceImport
		pods                []corev1.Pod
		endpointSlices      []discoveryv1.EndpointSlice
		supportObjects      translatorSupportObjects
		backendConfigMaps   []corev1.ConfigMap
		wasmConfigMaps      []corev1.ConfigMap
	)

	group, groupCtx := errgroup.WithContext(ctx)

	group.Go(func() error {
		var err error
		filteredGateways, err = t.loadFilteredGateways(groupCtx, cl)
		return err
	})
	group.Go(func() error {
		return cl.List(groupCtx, &httpRoutes)
	})
	group.Go(func() error {
		return cl.List(groupCtx, &grpcRoutes)
	})
	group.Go(func() error {
		if err := cl.List(groupCtx, &tcpRoutes); err != nil && !isOptionalResourceMissing(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		if err := cl.List(groupCtx, &udpRoutes); err != nil && !isOptionalResourceMissing(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		if err := cl.List(groupCtx, &tlsRoutes); err != nil && !isOptionalResourceMissing(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		if err := cl.List(groupCtx, &listenerSets); err != nil && !isOptionalResourceMissing(err) {
			return err
		}
		return nil
	})
	group.Go(func() error {
		var list corev1.ServiceList
		if err := cl.List(groupCtx, &list); err != nil {
			return err
		}
		services = list.Items
		return nil
	})
	group.Go(func() error {
		var list mcsv1alpha1.ServiceImportList
		if err := cl.List(groupCtx, &list); err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
			return err
		}
		serviceImports = list.Items
		return nil
	})
	group.Go(func() error {
		var list discoveryv1.EndpointSliceList
		if err := cl.List(groupCtx, &list); err != nil {
			return err
		}
		endpointSlices = list.Items
		return nil
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	group, groupCtx = errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		httpRouteRawFilters, err = loadHTTPRouteRawFilterConfigs(groupCtx, cl, httpRoutes.Items)
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

	extensionResolver := extensionfilter.NewResolver(supportObjects.configMaps)

	// Translate all routes in parallel — each route's translation is independent.
	transGroup, _ := errgroup.WithContext(ctx)
	snapshot.HTTPRoutes = make([]ir.HTTPRoute, len(httpRoutes.Items))
	for i := range httpRoutes.Items {
		i := i
		transGroup.Go(func() error {
			snapshot.HTTPRoutes[i] = translateHTTPRouteWithDefaultGateways(
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
		i := i
		transGroup.Go(func() error {
			snapshot.GRPCRoutes[i] = translateGRPCRouteWithDefaultGateways(grpcRoutes.Items[i], extensionResolver, filteredGateways)
			return nil
		})
	}
	snapshot.StreamRoutes = make([]ir.StreamRoute, 0, len(tcpRoutes.Items)+len(udpRoutes.Items)+len(tlsRoutes.Items))
	tcpResults := make([]ir.StreamRoute, len(tcpRoutes.Items))
	for i := range tcpRoutes.Items {
		i := i
		transGroup.Go(func() error {
			tcpResults[i] = translateTCPRouteWithDefaultGateways(tcpRoutes.Items[i], filteredGateways)
			return nil
		})
	}
	udpResults := make([]ir.StreamRoute, len(udpRoutes.Items))
	for i := range udpRoutes.Items {
		i := i
		transGroup.Go(func() error {
			udpResults[i] = translateUDPRouteWithDefaultGateways(udpRoutes.Items[i], filteredGateways)
			return nil
		})
	}
	tlsStreamResults := make([]ir.StreamRoute, len(tlsRoutes.Items))
	for i := range tlsRoutes.Items {
		i := i
		transGroup.Go(func() error {
			tlsStreamResults[i] = translateTLSRouteWithDefaultGateways(tlsRoutes.Items[i], filteredGateways)
			return nil
		})
	}
	if err := transGroup.Wait(); err != nil {
		return nil, err
	}
	for i := range tcpResults {
		snapshot.StreamRoutes = append(snapshot.StreamRoutes, tcpResults[i])
	}
	for i := range udpResults {
		snapshot.StreamRoutes = append(snapshot.StreamRoutes, udpResults[i])
	}
	for i := range tlsStreamResults {
		snapshot.StreamRoutes = append(snapshot.StreamRoutes, tlsStreamResults[i])
	}

	filteredServices := managedresources.FilterServices(services)
	serviceKeyMap := make(map[string]client.ObjectKey, len(filteredServices))
	for _, service := range filteredServices {
		key := client.ObjectKey{Namespace: service.Namespace, Name: service.Name}
		serviceKeyMap[backendObjectKey(key.Namespace, key.Name)] = key
	}
	serviceImportKeyMap := make(map[string]client.ObjectKey, len(serviceImports))
	for _, serviceImport := range serviceImports {
		key := client.ObjectKey{Namespace: serviceImport.Namespace, Name: serviceImport.Name}
		serviceImportKeyMap[backendObjectKey(key.Namespace, key.Name)] = key
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
		var list aiservicev1alpha1.AIServiceList
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
		var list tokenpolicyv1alpha1.TokenPolicyList
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
		var list wasmpluginv1alpha1.WasmPluginList
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
			referencedConfigMapKeysForWasmPlugins(wasmPlugins),
		)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	if err := t.limits.validateInputObjects(
		len(filteredGateways) +
			len(httpRoutes.Items) +
			len(grpcRoutes.Items) +
			len(tcpRoutes.Items) +
			len(udpRoutes.Items) +
			len(tlsRoutes.Items) +
			len(referenceGrants) +
			len(backendTLSPolicies) +
			len(backendLBPolicies) +
			len(services) +
			len(serviceImports) +
			len(pods) +
			len(endpointSlices),
	); err != nil {
		return nil, err
	}

	filteredEndpointSlices := managedresources.FilterEndpointSlices(endpointSlices)
	mergedConfigMaps := mergeConfigMaps(supportObjects.configMaps, backendConfigMaps, wasmConfigMaps)
	indexes := newTranslatorIndexes(
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
		translateMeshServiceListeners(
			collectMeshServiceFrontends(
				filteredServices,
				httpRoutes.Items,
				grpcRoutes.Items,
				tcpRoutes.Items,
				udpRoutes.Items,
				tlsRoutes.Items,
			),
		)...,
	)

	backendRefs := newBackendRefTranslator(
		filteredServices,
		serviceImports,
		referenceGrants,
		extensionfilter.NewResolver(mergedConfigMaps),
	)

	annotGroup, _ := errgroup.WithContext(ctx)
	for idx := range httpRoutes.Items {
		idx := idx
		annotGroup.Go(func() error {
			backendRefs.annotateHTTPRoute(&snapshot.HTTPRoutes[idx], httpRoutes.Items[idx])
			return nil
		})
	}
	for idx := range snapshot.GRPCRoutes {
		idx := idx
		annotGroup.Go(func() error {
			backendRefs.annotateGRPCRoute(&snapshot.GRPCRoutes[idx], grpcRoutes.Items[idx])
			return nil
		})
	}
	streamIdx := 0
	for idx := range tcpRoutes.Items {
		idx := idx
		sIdx := streamIdx
		annotGroup.Go(func() error {
			backendRefs.annotateTCPRoute(&snapshot.StreamRoutes[sIdx], tcpRoutes.Items[idx])
			return nil
		})
		streamIdx++
	}
	for idx := range udpRoutes.Items {
		idx := idx
		sIdx := streamIdx
		annotGroup.Go(func() error {
			backendRefs.annotateUDPRoute(&snapshot.StreamRoutes[sIdx], udpRoutes.Items[idx])
			return nil
		})
		streamIdx++
	}
	for idx := range tlsRoutes.Items {
		idx := idx
		sIdx := streamIdx
		annotGroup.Go(func() error {
			backendRefs.annotateTLSRoute(&snapshot.StreamRoutes[sIdx], tlsRoutes.Items[idx])
			return nil
		})
		streamIdx++
	}
	_ = annotGroup.Wait()

	snapshot.Backends = translateBackendsWithIndexes(
		filteredServices,
		serviceImports,
		backendTLSPolicies,
		backendLBPolicies,
		t.limits.DefaultConnectTimeout,
		indexes,
	)
	aiServiceConfigs := translateAIServices(aiServices)
	for i := range snapshot.Backends {
		key := backendObjectKey(snapshot.Backends[i].Namespace, snapshot.Backends[i].Name)
		if cfg, ok := aiServiceConfigs[key]; ok {
			cfgCopy := cfg
			snapshot.Backends[i].AIService = &cfgCopy
		}
	}
	routeBackends := buildRouteBackendServices(httpRoutes.Items)
	tokenPolicyConfigs := translateTokenPolicies(tokenPolicies, serviceKeySet(filteredServices), serviceImportKeySet(serviceImports), routeBackends)
	for i := range snapshot.Backends {
		key := backendObjectKey(snapshot.Backends[i].Namespace, snapshot.Backends[i].Name)
		if cfg, ok := tokenPolicyConfigs[key]; ok {
			cfgCopy := cfg
			snapshot.Backends[i].TokenPolicy = &cfgCopy
		}
	}
	wasmPluginConfigs := translateWasmPlugins(wasmPlugins, mergedConfigMaps)
	for i := range snapshot.Backends {
		key := backendObjectKey(snapshot.Backends[i].Namespace, snapshot.Backends[i].Name)
		if cfg, ok := wasmPluginConfigs[key]; ok {
			cfgCopy := cfg
			snapshot.Backends[i].WasmPlugin = &cfgCopy
		}
	}
	snapshot.Workloads = translateWorkloads(pods)
	attachRoutes(snapshot, filteredGateways, supportObjects.namespaces, listenerSets.Items)
	snapshot.Secrets = filterSecretMaterialsByKeys(
		translateSecrets(supportObjects.secrets),
		listenerSecretMaterialKeys(snapshot.Listeners),
	)
	if err := t.limits.validateSnapshot(snapshot); err != nil {
		return nil, err
	}

	return snapshot, nil
}

func isOptionalResourceMissing(err error) bool {
	return meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err)
}
