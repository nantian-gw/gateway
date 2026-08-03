package backends

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
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

// DefaultConnectTimeout is the exported version for external callers.
const DefaultConnectTimeout = 5 * time.Second

// BackendService bundles a Kubernetes Service with its logical name and
// namespace for backend cluster construction.
type BackendService struct {
	namespace   string
	logicalName string
	service     corev1.Service
}

// EffectiveBackendServices resolves shadow service substitution and returns
// the effective backend services to use for cluster construction.
func EffectiveBackendServices(services []corev1.Service) []BackendService {
	shadowByName := make(map[string]corev1.Service, len(services))
	shadowByOriginal := make(map[string]corev1.Service, len(services))
	for _, svc := range services {
		if svc.Labels[mesh.ShadowServiceRoleLabel] != mesh.ShadowServiceRoleValue {
			continue
		}

		shadowByName[svc.Namespace+"/"+svc.Name] = svc
		originalNamespace := svc.Labels[mesh.OriginalServiceNamespaceLabel]
		originalName := svc.Labels[mesh.OriginalServiceNameLabel]
		if originalNamespace != "" && originalName != "" {
			shadowByOriginal[originalNamespace+"/"+originalName] = svc
		}
	}

	out := make([]BackendService, 0, len(services))
	for _, svc := range services {
		if svc.Labels[mesh.ShadowServiceRoleLabel] == mesh.ShadowServiceRoleValue {
			continue
		}

		actual := svc
		if svc.Annotations[mesh.ManagedServiceAnnotation] == "true" {
			shadowName := svc.Annotations[mesh.ShadowServiceAnnotation]
			if shadow, ok := shadowByName[svc.Namespace+"/"+shadowName]; ok {
				actual = shadow
			} else if shadow, ok := shadowByOriginal[svc.Namespace+"/"+svc.Name]; ok {
				actual = shadow
			}
		}

		out = append(out, BackendService{
			namespace:   svc.Namespace,
			logicalName: svc.Name,
			service:     actual,
		})
	}

	return out
}

func TranslateBackends(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	slices []discoveryv1.EndpointSlice,
	configMaps []corev1.ConfigMap,
	backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy,
	backendLBPolicies []backend.BackendLBPolicy,
	connectTimeout time.Duration,
) []ir.BackendCluster {
	return TranslateBackendsWithIndexes(
		services,
		serviceImports,
		backendTLSPolicies,
		backendLBPolicies,
		connectTimeout,
		shared.NewTranslatorIndexes(services, serviceImports, slices, nil, configMaps, nil),
	)
}

func TranslateBackendsWithIndexes(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy,
	backendLBPolicies []backend.BackendLBPolicy,
	connectTimeout time.Duration,
	indexes shared.TranslatorIndexes,
) []ir.BackendCluster {
	backendTLSValidations := BackendTLSValidationIndexWithIndexes(backendTLSPolicies, indexes)
	backendLBIndexes := BuildBackendLBPolicyIndexesWithIndexes(backendLBPolicies, indexes)
	out := TranslateEffectiveBackends(
		EffectiveBackendServices(services),
		backendTLSValidations,
		backendLBIndexes,
		connectTimeout,
		indexes,
	)
	out = append(out, TranslateServiceImportBackends(serviceImports, backendTLSValidations, backendLBIndexes, connectTimeout, indexes)...)
	return out
}

func TranslateEffectiveBackends(
	services []BackendService,
	backendTLSValidations map[string]*ir.BackendTLSValidation,
	backendLB BackendLBPolicyIndexes,
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

					matchedPort := uint32(port.Port) //nolint:gosec
					if portInfo := shared.SelectSlicePort(slice.Ports, port.Name, port.Port); portInfo != nil && portInfo.Port != nil {
						matchedPort = uint32(*portInfo.Port) //nolint:gosec
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

func TranslateServiceImportBackends(
	serviceImports []mcsv1alpha1.ServiceImport,
	backendTLSValidations map[string]*ir.BackendTLSValidation,
	backendLB BackendLBPolicyIndexes,
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

					matchedPort := uint32(port.Port) //nolint:gosec
					if portInfo := shared.SelectSlicePort(slice.Ports, port.Name, port.Port); portInfo != nil && portInfo.Port != nil {
						matchedPort = uint32(*portInfo.Port) //nolint:gosec
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

func TranslateSecrets(secrets []corev1.Secret) []ir.SecretMaterial {
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
