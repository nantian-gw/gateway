package infrastructure

import (
	"crypto/sha256"
	"encoding/hex"
	jsoniter "github.com/json-iterator/go"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	derivedResourceOwnerKindAnnotation                   = "nantian.dev/owner-kind"
	derivedResourceOwnerNamespaceAnnotation              = "nantian.dev/owner-namespace"
	derivedResourceOwnerNameAnnotation                   = "nantian.dev/owner-name"
	derivedResourceOwnerUIDAnnotation                    = "nantian.dev/owner-uid"
	derivedResourceOwnerGenerationAnnotation             = "nantian.dev/owner-generation"
	derivedResourceGatewayClassNameAnnotation            = "nantian.dev/gatewayclass-name"
	derivedResourceGatewayClassParametersRefAnnotation   = "nantian.dev/gatewayclass-parameters-ref"
	derivedResourceInfrastructureParametersRefAnnotation = "nantian.dev/infrastructure-parameters-ref"
	derivedResourceServiceParametersHashAnnotation       = "nantian.dev/service-parameters-hash"
)

func desiredGatewayServiceAnnotations(
	gateway gatewayv1.Gateway,
	params gatewayServiceParameters,
	gatewayClassParametersRef string,
) map[string]string {
	annotationCapacity := 6
	if gateway.Spec.Infrastructure != nil {
		annotationCapacity += len(gateway.Spec.Infrastructure.Annotations)
		if gateway.Spec.Infrastructure.ParametersRef != nil {
			annotationCapacity++
		}
	}
	if gatewayClassParametersRef != "" {
		annotationCapacity++
	}

	annotations := make(map[string]string, annotationCapacity)
	if gateway.Spec.Infrastructure != nil {
		for key, value := range gateway.Spec.Infrastructure.Annotations {
			annotations[string(key)] = string(value)
		}
		if gateway.Spec.Infrastructure.ParametersRef != nil {
			annotations[derivedResourceInfrastructureParametersRefAnnotation] = formatNamespacedName(
				gateway.Namespace,
				gateway.Spec.Infrastructure.ParametersRef.Name,
			)
		}
	}
	if gatewayClassParametersRef != "" {
		annotations[derivedResourceGatewayClassParametersRefAnnotation] = gatewayClassParametersRef
	}

	annotations[derivedResourceOwnerKindAnnotation] = "Gateway"
	annotations[derivedResourceOwnerNamespaceAnnotation] = gateway.Namespace
	annotations[derivedResourceOwnerNameAnnotation] = gateway.Name
	annotations[derivedResourceOwnerGenerationAnnotation] = strconv.FormatInt(gateway.Generation, 10)
	annotations[derivedResourceGatewayClassNameAnnotation] = string(gateway.Spec.GatewayClassName)
	annotations[derivedResourceServiceParametersHashAnnotation] = gatewayServiceParametersHash(params)
	if gateway.UID != "" {
		annotations[derivedResourceOwnerUIDAnnotation] = string(gateway.UID)
	}

	return annotations
}

func desiredGatewayServiceConvergenceAnnotations(
	gateway gatewayv1.Gateway,
	gatewayClassParametersRef string,
) map[string]string {
	annotationCapacity := 5
	if gateway.UID != "" {
		annotationCapacity++
	}
	if gateway.Spec.Infrastructure != nil && gateway.Spec.Infrastructure.ParametersRef != nil {
		annotationCapacity++
	}
	if gatewayClassParametersRef != "" {
		annotationCapacity++
	}

	annotations := make(map[string]string, annotationCapacity)
	annotations[derivedResourceOwnerKindAnnotation] = "Gateway"
	annotations[derivedResourceOwnerNamespaceAnnotation] = gateway.Namespace
	annotations[derivedResourceOwnerNameAnnotation] = gateway.Name
	annotations[derivedResourceOwnerGenerationAnnotation] = strconv.FormatInt(gateway.Generation, 10)
	annotations[derivedResourceGatewayClassNameAnnotation] = string(gateway.Spec.GatewayClassName)
	if gateway.UID != "" {
		annotations[derivedResourceOwnerUIDAnnotation] = string(gateway.UID)
	}
	if gateway.Spec.Infrastructure != nil && gateway.Spec.Infrastructure.ParametersRef != nil {
		annotations[derivedResourceInfrastructureParametersRefAnnotation] = formatNamespacedName(
			gateway.Namespace,
			gateway.Spec.Infrastructure.ParametersRef.Name,
		)
	}
	if gatewayClassParametersRef != "" {
		annotations[derivedResourceGatewayClassParametersRefAnnotation] = gatewayClassParametersRef
	}

	return annotations
}

func desiredGatewayServiceOwnerReferences(gateway gatewayv1.Gateway) []metav1.OwnerReference {
	if gateway.UID == "" {
		return nil
	}

	controller := true
	blockOwnerDeletion := true
	return []metav1.OwnerReference{{
		APIVersion:         gatewayv1.GroupVersion.String(),
		Kind:               "Gateway",
		Name:               gateway.Name,
		UID:                gateway.UID,
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}}
}

func desiredEndpointSliceOwnerReferences(service corev1.Service) []metav1.OwnerReference {
	if service.UID == "" {
		return nil
	}

	controller := true
	blockOwnerDeletion := true
	return []metav1.OwnerReference{{
		APIVersion:         "v1",
		Kind:               "Service",
		Name:               service.Name,
		UID:                service.UID,
		Controller:         &controller,
		BlockOwnerDeletion: &blockOwnerDeletion,
	}}
}

func filterGatewayServiceConvergenceAnnotations(
	annotations map[string]string,
	includeOwnerUID bool,
) map[string]string {
	if len(annotations) == 0 {
		return nil
	}

	out := make(map[string]string)
	for key, value := range annotations {
		if !isGatewayServiceConvergenceAnnotation(key) {
			continue
		}
		if key == derivedResourceOwnerUIDAnnotation && !includeOwnerUID {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isGatewayServiceConvergenceAnnotation(key string) bool {
	switch key {
	case derivedResourceOwnerKindAnnotation,
		derivedResourceOwnerNamespaceAnnotation,
		derivedResourceOwnerNameAnnotation,
		derivedResourceOwnerUIDAnnotation,
		derivedResourceOwnerGenerationAnnotation,
		derivedResourceGatewayClassNameAnnotation,
		derivedResourceGatewayClassParametersRefAnnotation,
		derivedResourceInfrastructureParametersRefAnnotation:
		return true
	default:
		return false
	}
}

func gatewayServiceParametersHash(params gatewayServiceParameters) string {
	normalized := cloneGatewayServiceParameters(params)
	normalized.normalize()

	raw, err := jsoniter.Marshal(normalized)
	if err != nil {
		return ""
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
