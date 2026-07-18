package translator

import (
	"context"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/resources"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func (t *Translator) BuildBackendsForSnapshot(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	changedServiceKeys []client.ObjectKey,
	changedServiceImportKeys []client.ObjectKey,
) ([]ir.BackendCluster, error) {
	if current == nil {
		return t.BuildBackends(ctx, cl)
	}

	serviceKeys := objectKeyMap(changedServiceKeys)
	serviceImportKeys := objectKeyMap(changedServiceImportKeys)
	if len(serviceKeys) == 0 && len(serviceImportKeys) == 0 {
		currentBackendKeys := backendCatalogObjectKeysFromSnapshot(current)
		referencedKeys := referencedBackendObjectKeysFromSnapshot(current)

		serviceKeys = objectKeyMap(referencedKeys.services)
		serviceImportKeys = objectKeyMap(referencedKeys.serviceImports)
		for _, key := range currentBackendKeys {
			lookupKey := shared.BackendObjectKey(key.Namespace, key.Name)
			_, serviceKnown := serviceKeys[lookupKey]
			_, serviceImportKnown := serviceImportKeys[lookupKey]
			if serviceKnown || serviceImportKnown {
				continue
			}
			serviceKeys[lookupKey] = key
			serviceImportKeys[lookupKey] = key
		}
	}

	replacementKeys := objectKeyMap(changedServiceKeys, changedServiceImportKeys)
	if len(replacementKeys) == 0 {
		replacementKeys = objectKeyMap(sortedObjectKeys(serviceKeys), sortedObjectKeys(serviceImportKeys))
	}

	return t.buildBackendsForKeyMaps(ctx, cl, current, replacementKeys, serviceKeys, serviceImportKeys)
}

func (t *Translator) BuildBackendsForNamespaces(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	namespaces []string,
) ([]ir.BackendCluster, error) {
	if current == nil {
		return t.BuildBackends(ctx, cl)
	}

	namespaceSet := make(map[string]struct{}, len(namespaces))
	for _, namespace := range namespaces {
		if namespace == "" {
			continue
		}
		namespaceSet[namespace] = struct{}{}
	}
	if len(namespaceSet) == 0 {
		return t.BuildBackends(ctx, cl)
	}

	currentBackendKeys := filterObjectKeysByNamespaceSet(backendCatalogObjectKeysFromSnapshot(current), namespaceSet)
	referencedKeys := referencedBackendObjectKeysFromSnapshot(current)

	serviceKeys := objectKeyMap(filterObjectKeysByNamespaceSet(referencedKeys.services, namespaceSet))
	serviceImportKeys := objectKeyMap(filterObjectKeysByNamespaceSet(referencedKeys.serviceImports, namespaceSet))
	for _, key := range currentBackendKeys {
		lookupKey := shared.BackendObjectKey(key.Namespace, key.Name)
		_, serviceKnown := serviceKeys[lookupKey]
		_, serviceImportKnown := serviceImportKeys[lookupKey]
		if serviceKnown || serviceImportKnown {
			continue
		}
		serviceKeys[lookupKey] = key
		serviceImportKeys[lookupKey] = key
	}

	replacementKeys := objectKeyMap(currentBackendKeys, sortedObjectKeys(serviceKeys), sortedObjectKeys(serviceImportKeys))
	return t.buildBackendsForKeyMaps(ctx, cl, current, replacementKeys, serviceKeys, serviceImportKeys)
}

func (t *Translator) buildBackendsForKeyMaps(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	replacementKeys map[string]client.ObjectKey,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) ([]ir.BackendCluster, error) {
	if current == nil {
		return t.BuildBackends(ctx, cl)
	}

	var err error
	serviceKeys, replacementKeys, err = expandMeshShadowBackendKeys(ctx, cl, serviceKeys, replacementKeys)
	if err != nil {
		return nil, err
	}

	var (
		services           []corev1.Service
		serviceImports     []mcsv1alpha1.ServiceImport
		endpointSlices     []discoveryv1.EndpointSlice
		backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy
		backendLBPolicies  []backend.BackendLBPolicy
	)
	orderedServiceKeys := sortedObjectKeys(serviceKeys)
	orderedServiceImportKeys := sortedObjectKeys(serviceImportKeys)
	policyNamespaces := sortedBackendPolicyNamespaces(serviceKeys, serviceImportKeys)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		services, err = loadServices(groupCtx, cl, orderedServiceKeys)
		return err
	})
	group.Go(func() error {
		var err error
		serviceImports, err = loadServiceImports(groupCtx, cl, orderedServiceImportKeys)
		return err
	})
	group.Go(func() error {
		var err error
		endpointSlices, err = loadEndpointSlicesForBackendKeys(
			groupCtx,
			cl,
			orderedServiceKeys,
			orderedServiceImportKeys,
		)
		return err
	})
	group.Go(func() error {
		var err error
		backendTLSPolicies, err = loadBackendTLSPoliciesForNamespaces(
			groupCtx,
			cl,
			policyNamespaces,
			serviceKeys,
			serviceImportKeys,
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
			policyNamespaces,
			serviceKeys,
			serviceImportKeys,
		)
		if err != nil && !meta.IsNoMatchError(err) && !runtime.IsNotRegisteredError(err) {
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

	filteredServices := resources.FilterServices(services)
	indexes := shared.NewTranslatorIndexes(
		filteredServices,
		serviceImports,
		endpointSlices,
		nil,
		configMaps,
		nil,
	)

	return mergePartialBackends(current, replacementKeys, translateBackendsWithIndexes(
		filteredServices,
		serviceImports,
		backendTLSPolicies,
		backendLBPolicies,
		defaultConnectTimeout,
		indexes,
	)), nil
}

func expandMeshShadowBackendKeys(
	ctx context.Context,
	cl client.Client,
	serviceKeys map[string]client.ObjectKey,
	replacementKeys map[string]client.ObjectKey,
) (map[string]client.ObjectKey, map[string]client.ObjectKey, error) {
	if len(serviceKeys) == 0 {
		return serviceKeys, replacementKeys, nil
	}

	expandedServiceKeys := cloneObjectKeyMap(serviceKeys)
	expandedReplacementKeys := cloneObjectKeyMap(replacementKeys)
	loadedKeys := make(map[string]struct{}, len(serviceKeys))
	pendingKeys := cloneObjectKeyMap(serviceKeys)

	for len(pendingKeys) != 0 {
		services, err := loadServices(ctx, cl, sortedObjectKeys(pendingKeys))
		if err != nil {
			return nil, nil, err
		}
		for key := range pendingKeys {
			loadedKeys[key] = struct{}{}
		}
		pendingKeys = make(map[string]client.ObjectKey)

		for _, service := range services {
			recordMeshShadowBackendKeyExpansion(
				service,
				expandedServiceKeys,
				expandedReplacementKeys,
				pendingKeys,
				loadedKeys,
			)
		}
	}

	return expandedServiceKeys, expandedReplacementKeys, nil
}

func recordMeshShadowBackendKeyExpansion(
	service corev1.Service,
	serviceKeys map[string]client.ObjectKey,
	replacementKeys map[string]client.ObjectKey,
	pendingKeys map[string]client.ObjectKey,
	loadedKeys map[string]struct{},
) {
	addServiceKey := func(key client.ObjectKey) {
		if key.Namespace == "" || key.Name == "" {
			return
		}
		lookupKey := shared.BackendObjectKey(key.Namespace, key.Name)
		if _, ok := serviceKeys[lookupKey]; ok {
			return
		}
		serviceKeys[lookupKey] = key
		if _, loaded := loadedKeys[lookupKey]; !loaded {
			pendingKeys[lookupKey] = key
		}
	}
	addReplacementKey := func(key client.ObjectKey) {
		if key.Namespace == "" || key.Name == "" {
			return
		}
		replacementKeys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
	}

	if service.Labels[mesh.ShadowServiceRoleLabel] == mesh.ShadowServiceRoleValue {
		originalKey := client.ObjectKey{
			Namespace: strings.TrimSpace(service.Labels[mesh.OriginalServiceNamespaceLabel]),
			Name:      strings.TrimSpace(service.Labels[mesh.OriginalServiceNameLabel]),
		}
		addServiceKey(originalKey)
		addReplacementKey(originalKey)
		return
	}

	shadowName := strings.TrimSpace(service.Annotations[mesh.ShadowServiceAnnotation])
	if service.Annotations[mesh.ManagedServiceAnnotation] == "true" && shadowName != "" {
		addServiceKey(client.ObjectKey{
			Namespace: service.Namespace,
			Name:      shadowName,
		})
	}
}

func mergePartialBackends(
	current *ir.Snapshot,
	replacementKeys map[string]client.ObjectKey,
	updated []ir.BackendCluster,
) []ir.BackendCluster {
	if current == nil {
		return updated
	}

	existing := current.Clone().Backends
	out := make([]ir.BackendCluster, 0, len(existing)+len(updated))
	for _, backend := range existing {
		key, ok := backendObjectKeyForCluster(backend)
		if ok {
			if _, replace := replacementKeys[shared.BackendObjectKey(key.Namespace, key.Name)]; replace {
				continue
			}
		}
		out = append(out, backend)
	}
	out = append(out, updated...)
	return out
}

func filterObjectKeysByNamespaceSet(
	keys []client.ObjectKey,
	namespaces map[string]struct{},
) []client.ObjectKey {
	if len(keys) == 0 || len(namespaces) == 0 {
		return nil
	}

	out := make([]client.ObjectKey, 0, len(keys))
	for _, key := range keys {
		if _, ok := namespaces[key.Namespace]; !ok {
			continue
		}
		out = append(out, key)
	}
	return out
}

func sortedBackendPolicyNamespaces(
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) []string {
	namespaces := make(map[string]struct{}, len(serviceKeys)+len(serviceImportKeys))
	for _, key := range serviceKeys {
		if key.Namespace != "" {
			namespaces[key.Namespace] = struct{}{}
		}
	}
	for _, key := range serviceImportKeys {
		if key.Namespace != "" {
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

func loadBackendTLSPoliciesForNamespaces(
	ctx context.Context,
	cl client.Client,
	namespaces []string,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	targetValuesByNamespace := backendPolicyTargetRefIndexValuesByNamespace(serviceKeys, serviceImportKeys)
	policies := make([]gatewayv1alpha3.BackendTLSPolicy, 0)
	seen := make(map[string]struct{})
	for _, namespace := range namespaces {
		items, err := listBackendTLSPoliciesForNamespaceTargets(
			ctx,
			cl,
			namespace,
			targetValuesByNamespace[namespace],
		)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !backendTLSPolicyTouchesKeys(item.Namespace, item.Spec.TargetRefs, serviceKeys, serviceImportKeys) {
				continue
			}
			key := item.Namespace + "/" + item.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}
	sort.Slice(policies, func(i, j int) bool {
		left := policies[i].Namespace + "/" + policies[i].Name
		right := policies[j].Namespace + "/" + policies[j].Name
		return left < right
	})
	return policies, nil
}

func loadBackendLBPoliciesForNamespaces(
	ctx context.Context,
	cl client.Client,
	namespaces []string,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) ([]backend.BackendLBPolicy, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	targetValuesByNamespace := backendPolicyTargetRefIndexValuesByNamespace(serviceKeys, serviceImportKeys)
	policies := make([]backend.BackendLBPolicy, 0)
	seen := make(map[string]struct{})
	for _, namespace := range namespaces {
		items, err := listBackendLBPoliciesForNamespaceTargets(
			ctx,
			cl,
			namespace,
			targetValuesByNamespace[namespace],
		)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if !backendLBPolicyTouchesKeys(item.Namespace, item.Spec.TargetRefs, serviceKeys, serviceImportKeys) {
				continue
			}
			key := item.Namespace + "/" + item.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}
	sort.Slice(policies, func(i, j int) bool {
		left := policies[i].Namespace + "/" + policies[i].Name
		right := policies[j].Namespace + "/" + policies[j].Name
		return left < right
	})
	return policies, nil
}

func listBackendTLSPoliciesForNamespaceTargets(
	ctx context.Context,
	cl client.Client,
	namespace string,
	targetValues []string,
) ([]gatewayv1alpha3.BackendTLSPolicy, error) {
	if len(targetValues) == 0 {
		return nil, nil
	}

	items, usedIndex, err := listBackendTLSPoliciesByTargetRefIndex(ctx, cl, namespace, targetValues)
	if err != nil {
		return nil, err
	}
	if usedIndex {
		return items, nil
	}
	return gatewayapi.ListBackendTLSPoliciesV1WithOptions(ctx, cl, client.InNamespace(namespace))
}

func listBackendTLSPoliciesByTargetRefIndex(
	ctx context.Context,
	cl client.Client,
	namespace string,
	targetValues []string,
) ([]gatewayv1alpha3.BackendTLSPolicy, bool, error) {
	policies := make([]gatewayv1alpha3.BackendTLSPolicy, 0)
	seen := make(map[string]struct{})

	for _, targetValue := range targetValues {
		items, err := gatewayapi.ListBackendTLSPoliciesV1WithOptions(
			ctx,
			cl,
			client.InNamespace(namespace),
			client.MatchingFields{backendTLSPolicyTargetRefIndex: targetValue},
		)
		if err != nil {
			if isMissingFieldIndexError(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		for _, item := range items {
			key := item.Namespace + "/" + item.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}

	return policies, true, nil
}

func listBackendLBPoliciesForNamespaceTargets(
	ctx context.Context,
	cl client.Client,
	namespace string,
	targetValues []string,
) ([]backend.BackendLBPolicy, error) {
	if len(targetValues) == 0 {
		return nil, nil
	}

	items, usedIndex, err := listBackendLBPoliciesByTargetRefIndex(ctx, cl, namespace, targetValues)
	if err != nil {
		return nil, err
	}
	if usedIndex {
		return items, nil
	}

	var list backend.BackendLBPolicyList
	if err := cl.List(ctx, &list, client.InNamespace(namespace)); err != nil {
		return nil, err
	}
	return list.Items, nil
}

func listBackendLBPoliciesByTargetRefIndex(
	ctx context.Context,
	cl client.Client,
	namespace string,
	targetValues []string,
) ([]backend.BackendLBPolicy, bool, error) {
	policies := make([]backend.BackendLBPolicy, 0)
	seen := make(map[string]struct{})

	for _, targetValue := range targetValues {
		var list backend.BackendLBPolicyList
		if err := cl.List(
			ctx,
			&list,
			client.InNamespace(namespace),
			client.MatchingFields{backendLBPolicyTargetRefIndex: targetValue},
		); err != nil {
			if isMissingFieldIndexError(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		for _, item := range list.Items {
			key := item.Namespace + "/" + item.Name
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			policies = append(policies, item)
		}
	}

	return policies, true, nil
}

func backendTLSPolicyTouchesKeys(
	namespace string,
	targetRefs []gatewayv1.LocalPolicyTargetReferenceWithSectionName,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) bool {
	for _, targetRef := range targetRefs {
		key := shared.BackendObjectKey(namespace, string(targetRef.Name))
		switch {
		case string(targetRef.Group) == "" && string(targetRef.Kind) == "Service":
			if _, ok := serviceKeys[key]; ok {
				return true
			}
		case string(targetRef.Group) == mcsv1alpha1.GroupName &&
			string(targetRef.Kind) == "ServiceImport":
			if _, ok := serviceImportKeys[key]; ok {
				return true
			}
		}
	}
	return false
}

func backendLBPolicyTouchesKeys(
	namespace string,
	targetRefs []backend.LocalPolicyTargetReference,
	serviceKeys map[string]client.ObjectKey,
	serviceImportKeys map[string]client.ObjectKey,
) bool {
	for _, targetRef := range targetRefs {
		key := shared.BackendObjectKey(namespace, string(targetRef.Name))
		switch {
		case string(targetRef.Group) == "" && string(targetRef.Kind) == "Service":
			if _, ok := serviceKeys[key]; ok {
				return true
			}
		case string(targetRef.Group) == mcsv1alpha1.GroupName &&
			string(targetRef.Kind) == "ServiceImport":
			if _, ok := serviceImportKeys[key]; ok {
				return true
			}
		}
	}
	return false
}

func backendCatalogObjectKeysFromSnapshot(current *ir.Snapshot) []client.ObjectKey {
	if current == nil {
		return nil
	}

	keys := make(map[string]client.ObjectKey, len(current.Backends))
	for _, backend := range current.Backends {
		key, ok := backendObjectKeyForCluster(backend)
		if !ok {
			continue
		}
		keys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
	}
	return sortedObjectKeys(keys)
}

func backendObjectKeyForCluster(backend ir.BackendCluster) (client.ObjectKey, bool) {
	if backend.Namespace == "" {
		return client.ObjectKey{}, false
	}

	name := ""
	if backend.Metadata != nil {
		name = backend.Metadata["service"]
	}
	if name == "" {
		name, _, _ = strings.Cut(backend.Name, ":")
	}
	if name == "" {
		return client.ObjectKey{}, false
	}
	return client.ObjectKey{Namespace: backend.Namespace, Name: name}, true
}

func objectKeyMap(groups ...[]client.ObjectKey) map[string]client.ObjectKey {
	keys := make(map[string]client.ObjectKey)
	for _, group := range groups {
		for _, key := range group {
			if key.Name == "" {
				continue
			}
			keys[shared.BackendObjectKey(key.Namespace, key.Name)] = key
		}
	}
	return keys
}

func cloneObjectKeyMap(items map[string]client.ObjectKey) map[string]client.ObjectKey {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]client.ObjectKey, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func loadEndpointSlicesForBackendKeys(
	ctx context.Context,
	cl client.Client,
	serviceKeys []client.ObjectKey,
	serviceImportKeys []client.ObjectKey,
) ([]discoveryv1.EndpointSlice, error) {
	slicesByKey := make(map[string]discoveryv1.EndpointSlice)

	for _, key := range serviceKeys {
		items, err := loadEndpointSlicesWithLabel(ctx, cl, key.Namespace, discoveryv1.LabelServiceName, key.Name)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			slicesByKey[item.Namespace+"/"+item.Name] = item
		}
	}
	for _, key := range serviceImportKeys {
		items, err := loadEndpointSlicesWithLabel(ctx, cl, key.Namespace, mcsv1alpha1.LabelServiceName, key.Name)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			slicesByKey[item.Namespace+"/"+item.Name] = item
		}
	}

	if len(slicesByKey) == 0 {
		return nil, nil
	}

	out := make([]discoveryv1.EndpointSlice, 0, len(slicesByKey))
	for _, item := range slicesByKey {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Namespace + "/" + out[i].Name
		right := out[j].Namespace + "/" + out[j].Name
		return left < right
	})
	return resources.FilterEndpointSlices(out), nil
}

func loadEndpointSlicesWithLabel(
	ctx context.Context,
	cl client.Client,
	namespace string,
	labelKey string,
	labelValue string,
) ([]discoveryv1.EndpointSlice, error) {
	var endpointSlices discoveryv1.EndpointSliceList
	if err := cl.List(
		ctx,
		&endpointSlices,
		client.InNamespace(namespace),
		client.MatchingLabels{labelKey: labelValue},
	); err != nil {
		return nil, err
	}
	return endpointSlices.Items, nil
}
