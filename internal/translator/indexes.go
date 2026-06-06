package translator

import (
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

type translatorIndexes struct {
	servicesByKey                    map[string]corev1.Service
	serviceImportsByKey              map[string]mcsv1alpha1.ServiceImport
	tlsSecretsByKey                  map[string]corev1.Secret
	configMapCAPEMsByKey             map[string]string
	endpointSlicesByServiceKey       map[string][]discoveryv1.EndpointSlice
	endpointSlicesByServiceImportKey map[string][]discoveryv1.EndpointSlice
	referenceGrantsByNamespace       map[string][]gatewayv1beta1.ReferenceGrant
}

func newTranslatorIndexes(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	slices []discoveryv1.EndpointSlice,
	secrets []corev1.Secret,
	configMaps []corev1.ConfigMap,
	referenceGrants []gatewayv1beta1.ReferenceGrant,
) translatorIndexes {
	indexes := translatorIndexes{
		servicesByKey:                    make(map[string]corev1.Service, len(services)),
		serviceImportsByKey:              make(map[string]mcsv1alpha1.ServiceImport, len(serviceImports)),
		tlsSecretsByKey:                  make(map[string]corev1.Secret, len(secrets)),
		configMapCAPEMsByKey:             make(map[string]string, len(configMaps)),
		endpointSlicesByServiceKey:       make(map[string][]discoveryv1.EndpointSlice),
		endpointSlicesByServiceImportKey: make(map[string][]discoveryv1.EndpointSlice),
		referenceGrantsByNamespace:       make(map[string][]gatewayv1beta1.ReferenceGrant),
	}

	for _, service := range services {
		indexes.servicesByKey[backendObjectKey(service.Namespace, service.Name)] = service
	}

	for _, serviceImport := range serviceImports {
		indexes.serviceImportsByKey[backendObjectKey(serviceImport.Namespace, serviceImport.Name)] = serviceImport
	}

	for _, secret := range secrets {
		indexes.tlsSecretsByKey[backendObjectKey(secret.Namespace, secret.Name)] = secret
	}

	for _, configMap := range configMaps {
		indexes.configMapCAPEMsByKey[backendObjectKey(configMap.Namespace, configMap.Name)] = configMap.Data["ca.crt"]
	}

	for _, slice := range slices {
		if serviceName := slice.Labels[discoveryv1.LabelServiceName]; serviceName != "" {
			key := backendObjectKey(slice.Namespace, serviceName)
			indexes.endpointSlicesByServiceKey[key] = append(indexes.endpointSlicesByServiceKey[key], slice)
		}
		if serviceImportName := slice.Labels[mcsv1alpha1.LabelServiceName]; serviceImportName != "" {
			key := backendObjectKey(slice.Namespace, serviceImportName)
			indexes.endpointSlicesByServiceImportKey[key] = append(indexes.endpointSlicesByServiceImportKey[key], slice)
		}
	}

	for _, grant := range referenceGrants {
		indexes.referenceGrantsByNamespace[grant.Namespace] = append(indexes.referenceGrantsByNamespace[grant.Namespace], grant)
	}

	return indexes
}

func (i translatorIndexes) service(namespace, name string) (corev1.Service, bool) {
	item, ok := i.servicesByKey[backendObjectKey(namespace, name)]
	return item, ok
}

func (i translatorIndexes) serviceImport(namespace, name string) (mcsv1alpha1.ServiceImport, bool) {
	item, ok := i.serviceImportsByKey[backendObjectKey(namespace, name)]
	return item, ok
}

func (i translatorIndexes) tlsSecret(namespace, name string) (corev1.Secret, bool) {
	secret, ok := i.tlsSecretsByKey[backendObjectKey(namespace, name)]
	if !ok {
		return corev1.Secret{}, false
	}
	if secret.Type != corev1.SecretTypeTLS {
		return corev1.Secret{}, false
	}
	if len(secret.Data["tls.crt"]) == 0 || len(secret.Data["tls.key"]) == 0 {
		return corev1.Secret{}, false
	}
	return secret, true
}

func (i translatorIndexes) configMapCAPEM(namespace, name string) string {
	return i.configMapCAPEMsByKey[backendObjectKey(namespace, name)]
}

func (i translatorIndexes) serviceEndpointSlices(namespace, name string) []discoveryv1.EndpointSlice {
	return i.endpointSlicesByServiceKey[backendObjectKey(namespace, name)]
}

func (i translatorIndexes) serviceImportEndpointSlices(namespace, name string) []discoveryv1.EndpointSlice {
	return i.endpointSlicesByServiceImportKey[backendObjectKey(namespace, name)]
}
