package infrastructure

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

type GatewayInfrastructureParameterIssue struct {
	Path      string
	Reference string
	Cause     string
}

func (i GatewayInfrastructureParameterIssue) Error() string {
	return fmt.Sprintf("%s %s is invalid: %s", i.Path, i.Reference, i.Cause)
}

type GatewayInfrastructureParameterValidation struct {
	Issues []GatewayInfrastructureParameterIssue
}

func (v GatewayInfrastructureParameterValidation) HasIssues() bool {
	return len(v.Issues) > 0
}

func (v GatewayInfrastructureParameterValidation) Error() string {
	if len(v.Issues) == 0 {
		return ""
	}

	messages := make([]string, 0, len(v.Issues))
	for _, issue := range v.Issues {
		messages = append(messages, issue.Error())
	}
	return strings.Join(messages, "; ")
}

// ValidateGatewayInfrastructureParameters checks whether the controller's
// supported GatewayClass/Gateway service-parameter references are present and
// semantically valid. It is intended for status surfacing and does not mutate
// the current fallback behavior used by the infrastructure reconciler.
func ValidateGatewayInfrastructureParameters(
	gateway gatewayv1.Gateway,
	gatewayClasses map[string]gatewayv1.GatewayClass,
	configMaps map[string]corev1.ConfigMap,
) GatewayInfrastructureParameterValidation {
	validation := GatewayInfrastructureParameterValidation{}

	if gateway.Spec.Infrastructure != nil && gateway.Spec.Infrastructure.ParametersRef != nil {
		if _, err := loadGatewayServiceParametersFromMap(
			configMaps,
			gateway.Namespace,
			gateway.Spec.Infrastructure.ParametersRef,
		); err != nil {
			validation.Issues = append(validation.Issues, GatewayInfrastructureParameterIssue{
				Path:      "Gateway.spec.infrastructure.parametersRef",
				Reference: formatNamespacedName(gateway.Namespace, gateway.Spec.Infrastructure.ParametersRef.Name),
				Cause:     err.Error(),
			})
		}
	}

	gatewayClass, ok := gatewayClasses[string(gateway.Spec.GatewayClassName)]
	if !ok || gatewayClass.Spec.ParametersRef == nil {
		return validation
	}

	if _, err := loadGatewayClassServiceParametersFromMap(configMaps, &gatewayClass); err != nil {
		validation.Issues = append(validation.Issues, GatewayInfrastructureParameterIssue{
			Path:      "GatewayClass.spec.parametersRef",
			Reference: formatGatewayClassParametersRef(gatewayClass.Spec.ParametersRef),
			Cause:     err.Error(),
		})
	}

	return validation
}

func loadGatewayServiceParametersFromMap(
	configMaps map[string]corev1.ConfigMap,
	namespace string,
	ref *gatewayv1.LocalParametersReference,
) (gatewayServiceParameters, error) {
	if ref == nil {
		return gatewayServiceParameters{}, nil
	}
	if group := strings.TrimSpace(string(ref.Group)); group != "" {
		return gatewayServiceParameters{}, fmt.Errorf("unsupported parametersRef group %q", group)
	}
	if kind := strings.TrimSpace(string(ref.Kind)); !strings.EqualFold(kind, infrastructureParametersConfigMapKind) {
		return gatewayServiceParameters{}, fmt.Errorf("unsupported parametersRef kind %q", ref.Kind)
	}

	cm, ok := configMaps[formatNamespacedName(namespace, ref.Name)]
	if !ok {
		return gatewayServiceParameters{}, fmt.Errorf("configmaps %q not found", ref.Name)
	}

	return decodeGatewayServiceParameters(cm)
}

func loadGatewayClassServiceParametersFromMap(
	configMaps map[string]corev1.ConfigMap,
	gatewayClass *gatewayv1.GatewayClass,
) (gatewayServiceParameters, error) {
	if gatewayClass == nil || gatewayClass.Spec.ParametersRef == nil {
		return gatewayServiceParameters{}, nil
	}

	ref := gatewayClass.Spec.ParametersRef
	if group := strings.TrimSpace(string(ref.Group)); group != "" {
		return gatewayServiceParameters{}, fmt.Errorf("unsupported gatewayClass parametersRef group %q", group)
	}
	if kind := strings.TrimSpace(string(ref.Kind)); !strings.EqualFold(kind, infrastructureParametersConfigMapKind) {
		return gatewayServiceParameters{}, fmt.Errorf("unsupported gatewayClass parametersRef kind %q", ref.Kind)
	}
	if ref.Namespace == nil || strings.TrimSpace(string(*ref.Namespace)) == "" {
		return gatewayServiceParameters{}, fmt.Errorf("gatewayClass ConfigMap parametersRef namespace is required")
	}

	cm, ok := configMaps[formatNamespacedName(string(*ref.Namespace), ref.Name)]
	if !ok {
		return gatewayServiceParameters{}, fmt.Errorf("configmaps %q not found", ref.Name)
	}

	return decodeGatewayServiceParameters(cm)
}

func GatewayClassParametersReference(gatewayClass *gatewayv1.GatewayClass) string {
	if gatewayClass == nil {
		return ""
	}
	return formatGatewayClassParametersRef(gatewayClass.Spec.ParametersRef)
}

func formatGatewayClassParametersRef(ref *gatewayv1.ParametersReference) string {
	if ref == nil {
		return ""
	}
	if ref == nil || ref.Namespace == nil || strings.TrimSpace(string(*ref.Namespace)) == "" {
		return ref.Name
	}
	return formatNamespacedName(string(*ref.Namespace), ref.Name)
}

func formatNamespacedName(namespace, name string) string {
	return strings.TrimSpace(namespace) + "/" + strings.TrimSpace(name)
}
