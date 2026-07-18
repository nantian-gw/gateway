package translator

import (
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

const defaultConnectTimeout = 5 * time.Second

func translateBackends(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	slices []discoveryv1.EndpointSlice,
	configMaps []corev1.ConfigMap,
	backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy,
	backendLBPolicies []backend.BackendLBPolicy,
	connectTimeout time.Duration,
) []ir.BackendCluster {
	return translateBackendsWithIndexes(
		services,
		serviceImports,
		backendTLSPolicies,
		backendLBPolicies,
		connectTimeout,
		shared.NewTranslatorIndexes(services, serviceImports, slices, nil, configMaps, nil),
	)
}

func translateBackendsWithIndexes(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy,
	backendLBPolicies []backend.BackendLBPolicy,
	connectTimeout time.Duration,
	indexes shared.TranslatorIndexes,
) []ir.BackendCluster {
	backendTLSValidations := backendTLSValidationIndexWithIndexes(backendTLSPolicies, indexes)
	backendLBIndexes := buildBackendLBPolicyIndexesWithIndexes(backendLBPolicies, indexes)
	out := translateEffectiveBackends(
		effectiveBackendServices(services),
		backendTLSValidations,
		backendLBIndexes,
		connectTimeout,
		indexes,
	)
	out = append(out, translateServiceImportBackends(serviceImports, backendTLSValidations, backendLBIndexes, connectTimeout, indexes)...)
	return out
}

func translateEffectiveBackends(
	services []backendService,
	backendTLSValidations map[string]*ir.BackendTLSValidation,
	backendLB backendLBPolicyIndexes,
	connectTimeout time.Duration,
	indexes shared.TranslatorIndexes,
) []ir.BackendCluster {
	out := make([]ir.BackendCluster, 0)

	for _, service := range services {
		for _, port := range service.service.Spec.Ports {
			cluster := ir.BackendCluster{
				Name:           service.logicalName + ":" + strconv.Itoa(int(port.Port)),
				Namespace:      service.namespace,
				Protocol:       backendProtocol(port),
				ConnectTimeout: connectTimeout,
				BackendTLSValidation: backendTLSValidations[backendClusterKey(
					service.namespace,
					service.logicalName,
					port.Port,
				)],
				SessionPersistence: backendLB.sessionPersistence[backendClusterKey(
					service.namespace,
					service.logicalName,
					port.Port,
				)],
			LoadBalancing: backendLB.loadBalancing[backendClusterKey(
				service.namespace,
				service.logicalName,
				port.Port,
			)],
			CircuitBreaker: backendLB.circuitBreaker[backendClusterKey(
				service.namespace,
				service.logicalName,
				port.Port,
			)],
				Metadata: map[string]string{
					"service": service.logicalName,
				},
			}

			for _, slice := range indexes.ServiceEndpointSlices(service.service.Namespace, service.service.Name) {
				for _, endpoint := range slice.Endpoints {
					healthy := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
					zone := ""
					if endpoint.Zone != nil {
						zone = *endpoint.Zone
					}

					matchedPort := uint32(port.Port)
					if portInfo := shared.SelectSlicePort(slice.Ports, port.Name, port.Port); portInfo != nil && portInfo.Port != nil {
						matchedPort = uint32(*portInfo.Port)
					}

					for _, address := range endpoint.Addresses {
						cluster.Endpoints = append(cluster.Endpoints, ir.BackendEndpoint{
							Address: address,
							Port:    matchedPort,
							Healthy: healthy,
							Zone:    zone,
						})
					}
				}
			}

			out = append(out, cluster)
		}
	}

	return out
}

func backendProtocol(port corev1.ServicePort) string {
	return backendProtocolForValues(port.Protocol, port.AppProtocol)
}

func backendProtocolForValues(protocol corev1.Protocol, appProtocol *string) string {
	if appProtocol == nil || *appProtocol == "" {
		return string(protocol)
	}

	switch strings.ToLower(*appProtocol) {
	case "kubernetes.io/h2c", "h2c":
		return "H2C"
	case "kubernetes.io/ws", "ws":
		return "HTTP"
	case "kubernetes.io/wss", "wss":
		return "HTTPS"
	case "grpc":
		return "GRPC"
	case "grpcs":
		return "GRPCS"
	case "http":
		return "HTTP"
	case "https":
		return "HTTPS"
	default:
		return strings.ToUpper(*appProtocol)
	}
}

func translateServiceImportBackends(
	serviceImports []mcsv1alpha1.ServiceImport,
	backendTLSValidations map[string]*ir.BackendTLSValidation,
	backendLB backendLBPolicyIndexes,
	connectTimeout time.Duration,
	indexes shared.TranslatorIndexes,
) []ir.BackendCluster {
	out := make([]ir.BackendCluster, 0)
	for _, serviceImport := range serviceImports {
		for _, port := range serviceImport.Spec.Ports {
			cluster := ir.BackendCluster{
				Name:           serviceImport.Name + ":" + strconv.Itoa(int(port.Port)),
				Namespace:      serviceImport.Namespace,
				Protocol:       backendProtocolForValues(port.Protocol, port.AppProtocol),
				ConnectTimeout: connectTimeout,
				BackendTLSValidation: backendTLSValidations[backendClusterKey(
					serviceImport.Namespace,
					serviceImport.Name,
					port.Port,
				)],
				SessionPersistence: backendLB.sessionPersistence[backendClusterKey(
					serviceImport.Namespace,
					serviceImport.Name,
					port.Port,
				)],
				LoadBalancing: backendLB.loadBalancing[backendClusterKey(
					serviceImport.Namespace,
					serviceImport.Name,
					port.Port,
				)],
				Metadata: map[string]string{
					"service": serviceImport.Name,
				},
			}

			for _, slice := range indexes.ServiceImportEndpointSlices(serviceImport.Namespace, serviceImport.Name) {
				for _, endpoint := range slice.Endpoints {
					healthy := endpoint.Conditions.Ready == nil || *endpoint.Conditions.Ready
					zone := ""
					if endpoint.Zone != nil {
						zone = *endpoint.Zone
					}

					matchedPort := uint32(port.Port)
					if portInfo := shared.SelectSlicePort(slice.Ports, port.Name, port.Port); portInfo != nil && portInfo.Port != nil {
						matchedPort = uint32(*portInfo.Port)
					}

					for _, address := range endpoint.Addresses {
						cluster.Endpoints = append(cluster.Endpoints, ir.BackendEndpoint{
							Address: address,
							Port:    matchedPort,
							Healthy: healthy,
							Zone:    zone,
						})
					}
				}
			}

			out = append(out, cluster)
		}
	}

	return out
}

func translateSecrets(secrets []corev1.Secret) []ir.SecretMaterial {
	out := make([]ir.SecretMaterial, 0)
	for _, secret := range secrets {
		if secret.Type != corev1.SecretTypeTLS {
			continue
		}
		out = append(out, ir.SecretMaterial{
			Namespace: secret.Namespace,
			Name:      secret.Name,
			CertPEM:   string(secret.Data["tls.crt"]),
			KeyPEM:    string(secret.Data["tls.key"]),
		})
	}

	return out
}
