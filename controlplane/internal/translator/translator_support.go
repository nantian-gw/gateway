package translator

import (
	"context"
	"sort"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/extensionfilter"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapi"
)

type translatorSupportObjects struct {
	namespaces []corev1.Namespace
	secrets    []corev1.Secret
	configMaps []corev1.ConfigMap
}

func (t *Translator) loadSupportObjects(
	ctx context.Context,
	cl client.Client,
	gateways []gatewayv1.Gateway,
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
	tcpRoutes []gatewayv1alpha2.TCPRoute,
	udpRoutes []gatewayv1alpha2.UDPRoute,
	tlsRoutes []gatewayv1alpha2.TLSRoute,
	backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy,
) (translatorSupportObjects, error) {
	var out translatorSupportObjects

	secretKeys := referencedSecretKeys(gateways)
	configMapKeys := referencedConfigMapKeys(gateways, httpRoutes, grpcRoutes, backendTLSPolicies)
	namespaceKeys := attachmentNamespaceKeys(gateways, httpRoutes, grpcRoutes, tcpRoutes, udpRoutes, tlsRoutes)

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		secrets, err := loadSecrets(groupCtx, cl, secretKeys)
		if err != nil {
			return err
		}
		out.secrets = secrets
		return err
	})
	group.Go(func() error {
		configMaps, err := loadConfigMaps(groupCtx, cl, configMapKeys)
		if err != nil {
			return err
		}
		out.configMaps = configMaps
		return err
	})
	group.Go(func() error {
		namespaces, err := loadNamespaces(groupCtx, cl, namespaceKeys)
		if err != nil {
			return err
		}
		out.namespaces = namespaces
		return err
	})
	if err := group.Wait(); err != nil {
		return translatorSupportObjects{}, err
	}

	return out, nil
}

func referencedSecretKeys(gateways []gatewayv1.Gateway) []client.ObjectKey {
	keys := make(map[string]client.ObjectKey)
	for _, gateway := range gateways {
		for _, listener := range gatewayapi.EffectiveListeners(gateway) {
			if listener.TLS == nil {
				continue
			}
			for _, ref := range listener.TLS.CertificateRefs {
				if refGroup(&ref) != "" || refKind(&ref) != "Secret" || ref.Name == "" {
					continue
				}
				key := client.ObjectKey{
					Namespace: namespaceOrDefault(ref.Namespace, gateway.Namespace),
					Name:      string(ref.Name),
				}
				keys[backendObjectKey(key.Namespace, key.Name)] = key
			}
		}

		backendTLS := gatewayapi.GatewayBackendTLS(gateway)
		if backendTLS == nil || backendTLS.ClientCertificateRef == nil {
			continue
		}

		ref := backendTLS.ClientCertificateRef
		if refGroup(ref) != "" || refKind(ref) != "Secret" || ref.Name == "" {
			continue
		}
		key := client.ObjectKey{
			Namespace: namespaceOrDefault(ref.Namespace, gateway.Namespace),
			Name:      string(ref.Name),
		}
		keys[backendObjectKey(key.Namespace, key.Name)] = key
	}

	return sortedObjectKeys(keys)
}

func referencedConfigMapKeys(
	gateways []gatewayv1.Gateway,
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
	backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy,
) []client.ObjectKey {
	keys := make(map[string]client.ObjectKey)

	for _, gateway := range gateways {
		for _, listener := range gatewayapi.EffectiveListeners(gateway) {
			validation := gatewayapi.FrontendValidationForListener(gateway, listener)
			if validation == nil {
				continue
			}
			for _, ref := range validation.CACertificateRefs {
				if key, ok := configMapObjectKeyFromRef(gateway.Namespace, ref.Group, ref.Kind, ref.Namespace, ref.Name); ok {
					keys[backendObjectKey(key.Namespace, key.Name)] = key
				}
			}
		}
	}

	for _, route := range httpRoutes {
		for _, rule := range route.Spec.Rules {
			for _, filter := range rule.Filters {
				if key, ok := configMapObjectKeyFromLocalRef(route.Namespace, filter.ExtensionRef); ok {
					keys[backendObjectKey(key.Namespace, key.Name)] = key
				}
			}
			for _, backendRef := range rule.BackendRefs {
				for _, filter := range backendRef.Filters {
					if key, ok := configMapObjectKeyFromLocalRef(route.Namespace, filter.ExtensionRef); ok {
						keys[backendObjectKey(key.Namespace, key.Name)] = key
					}
				}
			}
		}
	}

	for _, route := range grpcRoutes {
		for _, rule := range route.Spec.Rules {
			for _, filter := range rule.Filters {
				if key, ok := configMapObjectKeyFromLocalRef(route.Namespace, filter.ExtensionRef); ok {
					keys[backendObjectKey(key.Namespace, key.Name)] = key
				}
			}
			for _, backendRef := range rule.BackendRefs {
				for _, filter := range backendRef.Filters {
					if key, ok := configMapObjectKeyFromLocalRef(route.Namespace, filter.ExtensionRef); ok {
						keys[backendObjectKey(key.Namespace, key.Name)] = key
					}
				}
			}
		}
	}

	for _, policy := range backendTLSPolicies {
		for _, ref := range policy.Spec.Validation.CACertificateRefs {
			if key, ok := localConfigMapObjectKey(policy.Namespace, ref.Group, ref.Kind, ref.Name); ok {
				keys[backendObjectKey(key.Namespace, key.Name)] = key
			}
		}
	}

	return sortedObjectKeys(keys)
}

func attachmentNamespaceKeys(
	gateways []gatewayv1.Gateway,
	httpRoutes []gatewayv1.HTTPRoute,
	grpcRoutes []gatewayv1.GRPCRoute,
	tcpRoutes []gatewayv1alpha2.TCPRoute,
	udpRoutes []gatewayv1alpha2.UDPRoute,
	tlsRoutes []gatewayv1alpha2.TLSRoute,
) []client.ObjectKey {
	if !gatewaysUseNamespaceSelectors(gateways) {
		return nil
	}

	keys := make(map[string]client.ObjectKey)
	addNamespace := func(namespace string) {
		if namespace == "" {
			return
		}
		keys[namespace] = client.ObjectKey{Name: namespace}
	}

	for _, route := range httpRoutes {
		addNamespace(route.Namespace)
	}
	for _, route := range grpcRoutes {
		addNamespace(route.Namespace)
	}
	for _, route := range tcpRoutes {
		addNamespace(route.Namespace)
	}
	for _, route := range udpRoutes {
		addNamespace(route.Namespace)
	}
	for _, route := range tlsRoutes {
		addNamespace(route.Namespace)
	}

	return sortedObjectKeys(keys)
}

func gatewaysUseNamespaceSelectors(gateways []gatewayv1.Gateway) bool {
	for _, gateway := range gateways {
		for _, listener := range gatewayapi.EffectiveListeners(gateway) {
			if listener.AllowedRoutes == nil || listener.AllowedRoutes.Namespaces == nil || listener.AllowedRoutes.Namespaces.From == nil {
				continue
			}
			if *listener.AllowedRoutes.Namespaces.From == gatewayv1.NamespacesFromSelector {
				return true
			}
		}
	}
	return false
}

func configMapObjectKeyFromRef(
	defaultNamespace string,
	group gatewayv1.Group,
	kind gatewayv1.Kind,
	namespace *gatewayv1.Namespace,
	name gatewayv1.ObjectName,
) (client.ObjectKey, bool) {
	if name == "" {
		return client.ObjectKey{}, false
	}
	if string(group) != "" {
		return client.ObjectKey{}, false
	}
	targetKind := string(kind)
	if targetKind == "" {
		targetKind = "ConfigMap"
	}
	if targetKind != "ConfigMap" {
		return client.ObjectKey{}, false
	}
	return client.ObjectKey{
		Namespace: namespaceOrDefault(namespace, defaultNamespace),
		Name:      string(name),
	}, true
}

func configMapObjectKeyFromLocalRef(
	defaultNamespace string,
	ref *gatewayv1.LocalObjectReference,
) (client.ObjectKey, bool) {
	if ref == nil || ref.Name == "" {
		return client.ObjectKey{}, false
	}
	if string(ref.Group) != "" {
		return client.ObjectKey{}, false
	}
	if string(ref.Kind) != extensionfilter.ConfigMapKind {
		return client.ObjectKey{}, false
	}
	return client.ObjectKey{
		Namespace: defaultNamespace,
		Name:      string(ref.Name),
	}, true
}

func localConfigMapObjectKey(
	defaultNamespace string,
	group gatewayv1.Group,
	kind gatewayv1.Kind,
	name gatewayv1.ObjectName,
) (client.ObjectKey, bool) {
	if name == "" || string(group) != "" {
		return client.ObjectKey{}, false
	}
	targetKind := string(kind)
	if targetKind == "" {
		targetKind = "ConfigMap"
	}
	if targetKind != "ConfigMap" {
		return client.ObjectKey{}, false
	}
	return client.ObjectKey{
		Namespace: defaultNamespace,
		Name:      string(name),
	}, true
}

func loadSecrets(
	ctx context.Context,
	cl client.Client,
	keys []client.ObjectKey,
) ([]corev1.Secret, error) {
	out := make([]corev1.Secret, 0, len(keys))
	for _, key := range keys {
		item := &corev1.Secret{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadConfigMaps(
	ctx context.Context,
	cl client.Client,
	keys []client.ObjectKey,
) ([]corev1.ConfigMap, error) {
	out := make([]corev1.ConfigMap, 0, len(keys))
	for _, key := range keys {
		item := &corev1.ConfigMap{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadNamespaces(
	ctx context.Context,
	cl client.Client,
	keys []client.ObjectKey,
) ([]corev1.Namespace, error) {
	out := make([]corev1.Namespace, 0, len(keys))
	for _, key := range keys {
		item := &corev1.Namespace{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadServices(
	ctx context.Context,
	cl client.Client,
	keys []client.ObjectKey,
) ([]corev1.Service, error) {
	out := make([]corev1.Service, 0, len(keys))
	for _, key := range keys {
		item := &corev1.Service{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadServiceImports(
	ctx context.Context,
	cl client.Client,
	keys []client.ObjectKey,
) ([]mcsv1alpha1.ServiceImport, error) {
	out := make([]mcsv1alpha1.ServiceImport, 0, len(keys))
	for _, key := range keys {
		item := &mcsv1alpha1.ServiceImport{}
		err := cl.Get(ctx, key, item)
		switch {
		case meta.IsNoMatchError(err), runtime.IsNotRegisteredError(err):
			return nil, nil
		case client.IgnoreNotFound(err) != nil:
			return nil, err
		case item.Name != "":
			out = append(out, *item)
		}
	}
	return out, nil
}

func loadPodsForNamespaces(
	ctx context.Context,
	cl client.Client,
	namespaces []string,
) ([]corev1.Pod, error) {
	if len(namespaces) == 0 {
		return nil, nil
	}

	pods := make([]corev1.Pod, 0)
	for _, namespace := range namespaces {
		if namespace == "" {
			continue
		}

		var list corev1.PodList
		if err := cl.List(ctx, &list, client.InNamespace(namespace)); err != nil {
			return nil, err
		}
		pods = append(pods, list.Items...)
	}

	sort.Slice(pods, func(i, j int) bool {
		left := pods[i].Namespace + "/" + pods[i].Name
		right := pods[j].Namespace + "/" + pods[j].Name
		return left < right
	})
	return pods, nil
}

func mergeConfigMaps(groups ...[]corev1.ConfigMap) []corev1.ConfigMap {
	items := make(map[string]corev1.ConfigMap)
	for _, group := range groups {
		for _, item := range group {
			if item.Name == "" {
				continue
			}
			items[item.Namespace+"/"+item.Name] = item
		}
	}

	out := make([]corev1.ConfigMap, 0, len(items))
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

func mergeSortedStrings(groups ...[]string) []string {
	items := make(map[string]struct{})
	for _, group := range groups {
		for _, item := range group {
			if item == "" {
				continue
			}
			items[item] = struct{}{}
		}
	}

	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sortedObjectKeys(keys map[string]client.ObjectKey) []client.ObjectKey {
	out := make([]client.ObjectKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Namespace + "/" + out[i].Name
		right := out[j].Namespace + "/" + out[j].Name
		return left < right
	})
	return out
}
