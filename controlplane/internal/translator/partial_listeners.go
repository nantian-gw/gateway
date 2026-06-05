package translator

import (
	"context"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func (t *Translator) BuildGatewayListenersForSnapshot(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
	gatewayKeys []client.ObjectKey,
) (*ir.Snapshot, error) {
	if current == nil {
		return t.Build(ctx, cl)
	}
	if len(gatewayKeys) == 0 {
		return ApplyPartialSnapshot(current, nil, nil), nil
	}

	gateways, err := loadGateways(ctx, cl, gatewayKeys)
	if err != nil {
		return nil, err
	}
	gateways, err = t.filterGatewaysByManagedClasses(ctx, cl, gateways)
	if err != nil {
		return nil, err
	}

	var (
		supportObjects  translatorSupportObjects
		referenceGrants []gatewayv1beta1.ReferenceGrant
	)
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		supportObjects, err = t.loadSupportObjects(groupCtx, cl, gateways, nil, nil, nil, nil, nil, nil)
		return err
	})
	group.Go(func() error {
		var err error
		referenceGrants, err = loadReferenceGrantsForNamespaces(
			groupCtx,
			cl,
			referencedGatewayGrantNamespaces(gateways),
		)
		return err
	})
	if err := group.Wait(); err != nil {
		return nil, err
	}

	indexes := newTranslatorIndexes(nil, nil, nil, supportObjects.secrets, supportObjects.configMaps, referenceGrants)
	listeners := t.rebuildGatewayListenersWithIndexes(current.Listeners, gatewayKeys, gateways, indexes)
	updated := ApplyPartialSnapshot(current, nil, listeners)
	attachmentNamespaces := attachmentRouteNamespacesForGatewayKeys(current, gatewayKeys)
	if len(attachmentNamespaces) != 0 {
		listeners, err = t.RebuildAttachmentsForNamespaces(ctx, cl, updated, attachmentNamespaces)
		if err != nil {
			return nil, err
		}
		updated = ApplyPartialSnapshot(current, nil, listeners)
	}
	secrets := rebuildGatewaySecretMaterials(current.Secrets, current.Listeners, listeners, gatewayKeys, supportObjects.secrets)
	return ApplyPartialSnapshotWithSecrets(updated, nil, listeners, secrets), nil
}

func (t *Translator) filterGatewaysByManagedClasses(
	ctx context.Context,
	cl client.Client,
	gateways []gatewayv1.Gateway,
) ([]gatewayv1.Gateway, error) {
	if len(gateways) == 0 {
		return nil, nil
	}

	gatewayClasses, err := loadGatewayClasses(ctx, cl, gatewayClassNamesFromGateways(gateways))
	if err != nil {
		return nil, err
	}

	managedClassNames := make(map[string]struct{}, len(gatewayClasses))
	for _, gatewayClass := range gatewayClasses {
		if string(gatewayClass.Spec.ControllerName) != t.controllerName {
			continue
		}
		managedClassNames[gatewayClass.Name] = struct{}{}
	}
	if len(managedClassNames) == 0 {
		gatewayClasses, err = listGatewayClassesForController(ctx, cl, t.controllerName)
		if err != nil {
			return nil, err
		}
		if len(gatewayClasses) == 0 {
			return gateways, nil
		}
		for _, gatewayClass := range gatewayClasses {
			managedClassNames[gatewayClass.Name] = struct{}{}
		}
	}

	filtered := make([]gatewayv1.Gateway, 0, len(gateways))
	for _, gateway := range gateways {
		if _, ok := managedClassNames[string(gateway.Spec.GatewayClassName)]; !ok {
			continue
		}
		filtered = append(filtered, gateway)
	}
	return filtered, nil
}

func loadGateways(
	ctx context.Context,
	cl client.Client,
	keys []client.ObjectKey,
) ([]gatewayv1.Gateway, error) {
	out := make([]gatewayv1.Gateway, 0, len(keys))
	for _, key := range keys {
		item := &gatewayv1.Gateway{}
		if err := cl.Get(ctx, key, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func attachmentRouteNamespacesForGatewayKeys(
	snapshot *ir.Snapshot,
	gatewayKeys []client.ObjectKey,
) []string {
	if snapshot == nil || len(gatewayKeys) == 0 {
		return nil
	}

	targetGateways := make(map[string]struct{}, len(gatewayKeys))
	for _, key := range gatewayKeys {
		if key.Name == "" {
			continue
		}
		targetGateways[key.Namespace+"/"+key.Name] = struct{}{}
	}

	namespaces := make(map[string]struct{})
	add := func(routeNamespace string, parentRefs []ir.ParentRef) {
		for _, parentRef := range parentRefs {
			if isServiceParentRef(parentRef) || parentRef.Name == "" {
				continue
			}

			namespace := parentRef.Namespace
			if namespace == "" {
				namespace = routeNamespace
			}
			if _, ok := targetGateways[namespace+"/"+parentRef.Name]; ok {
				namespaces[routeNamespace] = struct{}{}
				return
			}
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

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func gatewayClassNamesFromGateways(gateways []gatewayv1.Gateway) []string {
	if len(gateways) == 0 {
		return nil
	}

	names := make(map[string]struct{}, len(gateways))
	for _, gateway := range gateways {
		name := strings.TrimSpace(string(gateway.Spec.GatewayClassName))
		if name == "" {
			continue
		}
		names[name] = struct{}{}
	}

	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func loadGatewayClasses(
	ctx context.Context,
	cl client.Client,
	names []string,
) ([]gatewayv1.GatewayClass, error) {
	out := make([]gatewayv1.GatewayClass, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}

		item := &gatewayv1.GatewayClass{}
		if err := cl.Get(ctx, client.ObjectKey{Name: name}, item); client.IgnoreNotFound(err) != nil {
			return nil, err
		}
		if item.Name != "" {
			out = append(out, *item)
		}
	}
	return out, nil
}

func (t *Translator) rebuildGatewayListenersWithIndexes(
	current []ir.Listener,
	gatewayKeys []client.ObjectKey,
	gateways []gatewayv1.Gateway,
	indexes translatorIndexes,
) []ir.Listener {
	currentByName := make(map[string]ir.Listener, len(current))
	for _, listener := range current {
		currentByName[listener.Name] = listener
	}

	replacementPrefixes := gatewayListenerPrefixes(gatewayKeys)
	out := make([]ir.Listener, 0, len(current))
	for _, listener := range current {
		if gatewayListenerMatchesPrefixes(listener.Name, replacementPrefixes) {
			continue
		}
		out = append(out, listener)
	}

	for _, gateway := range gateways {
		for _, listener := range t.translateGatewayListenersWithIndexes(gateway, indexes, nil, nil) {
			if existing, ok := currentByName[listener.Name]; ok {
				listener.AttachedRoutes = append([]string(nil), existing.AttachedRoutes...)
			}
			out = append(out, listener)
		}
	}
	return out
}

func rebuildGatewaySecretMaterials(
	currentSecrets []ir.SecretMaterial,
	currentListeners []ir.Listener,
	updatedListeners []ir.Listener,
	gatewayKeys []client.ObjectKey,
	secrets []corev1.Secret,
) []ir.SecretMaterial {
	updated := gatewaySecretMaterialKeysFromListeners(updatedListeners, gatewayKeys)
	referenced := listenerSecretMaterialKeys(updatedListeners)
	replacements := filterSecretMaterialsByKeys(translateSecrets(secrets), updated)
	replacementKeys := make(map[string]struct{}, len(replacements))
	for _, secret := range replacements {
		replacementKeys[backendObjectKey(secret.Namespace, secret.Name)] = struct{}{}
	}

	out := make([]ir.SecretMaterial, 0, len(currentSecrets)+len(secrets))
	for _, secret := range currentSecrets {
		key := backendObjectKey(secret.Namespace, secret.Name)
		if _, stillReferenced := referenced[key]; !stillReferenced {
			continue
		}
		if _, replace := replacementKeys[key]; replace {
			continue
		}
		out = append(out, secret)
	}
	out = append(out, replacements...)
	return out
}

func listenerSecretMaterialKeys(listeners []ir.Listener) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, listener := range listeners {
		if listener.TLS != nil {
			for _, secretRef := range listener.TLS.SecretRefs {
				namespace, name, ok := splitNamespacedRef(secretRef)
				if !ok {
					continue
				}
				keys[backendObjectKey(namespace, name)] = struct{}{}
			}
		}
		if listener.BackendTLS != nil && listener.BackendTLS.ClientCertificateRef != "" {
			namespace, name, ok := splitNamespacedRef(listener.BackendTLS.ClientCertificateRef)
			if ok {
				keys[backendObjectKey(namespace, name)] = struct{}{}
			}
		}
	}
	return keys
}

func gatewayListenerPrefixes(keys []client.ObjectKey) map[string]struct{} {
	prefixes := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key.Name == "" {
			continue
		}
		prefixes[key.Namespace+"/"+key.Name+"/"] = struct{}{}
	}
	return prefixes
}

func gatewayListenerMatchesPrefixes(name string, prefixes map[string]struct{}) bool {
	for prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func gatewaySecretMaterialKeysFromListeners(
	listeners []ir.Listener,
	gatewayKeys []client.ObjectKey,
) map[string]struct{} {
	prefixes := gatewayListenerPrefixes(gatewayKeys)
	keys := make(map[string]struct{})

	for _, listener := range listeners {
		if !gatewayListenerMatchesPrefixes(listener.Name, prefixes) {
			continue
		}
		for key := range listenerSecretMaterialKeys([]ir.Listener{listener}) {
			keys[key] = struct{}{}
		}
	}
	return keys
}

func splitNamespacedRef(ref string) (string, string, bool) {
	namespace, name, ok := strings.Cut(ref, "/")
	if !ok || namespace == "" || name == "" {
		return "", "", false
	}
	return namespace, name, true
}

func referencedGatewayGrantNamespaces(gateways []gatewayv1.Gateway) []string {
	namespaces := make(map[string]struct{})
	for _, gateway := range gateways {
		for _, listener := range gatewayapi.EffectiveListeners(gateway) {
			if listener.TLS != nil {
				for _, ref := range listener.TLS.CertificateRefs {
					targetNamespace := namespaceOrDefault(ref.Namespace, gateway.Namespace)
					if targetNamespace != gateway.Namespace && refGroup(&ref) == "" && refKind(&ref) == "Secret" {
						namespaces[targetNamespace] = struct{}{}
					}
				}
			}

			if validation := gatewayapi.FrontendValidationForListener(gateway, listener); validation != nil {
				for _, ref := range validation.CACertificateRefs {
					targetNamespace := namespaceOrDefault(ref.Namespace, gateway.Namespace)
					targetKind := string(ref.Kind)
					if targetKind == "" {
						targetKind = "ConfigMap"
					}
					if targetNamespace != gateway.Namespace && string(ref.Group) == "" && targetKind == "ConfigMap" {
						namespaces[targetNamespace] = struct{}{}
					}
				}
			}
		}

		backendTLS := gatewayapi.GatewayBackendTLS(gateway)
		if backendTLS == nil || backendTLS.ClientCertificateRef == nil {
			continue
		}
		ref := backendTLS.ClientCertificateRef
		targetNamespace := namespaceOrDefault(ref.Namespace, gateway.Namespace)
		if targetNamespace != gateway.Namespace && refGroup(ref) == "" && refKind(ref) == "Secret" {
			namespaces[targetNamespace] = struct{}{}
		}
	}

	out := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}

func filterSecretMaterialsByKeys(
	materials []ir.SecretMaterial,
	keys map[string]struct{},
) []ir.SecretMaterial {
	if len(materials) == 0 || len(keys) == 0 {
		return nil
	}
	out := make([]ir.SecretMaterial, 0, len(materials))
	for _, material := range materials {
		if _, ok := keys[backendObjectKey(material.Namespace, material.Name)]; !ok {
			continue
		}
		out = append(out, material)
	}
	return out
}
