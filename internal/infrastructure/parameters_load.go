package infrastructure

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func loadGatewayServiceParameters(
	ctx context.Context,
	cl client.Client,
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

	cm := &corev1.ConfigMap{}
	if err := cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, cm); err != nil {
		return gatewayServiceParameters{}, err
	}

	return decodeGatewayServiceParameters(*cm)
}

func loadGatewayClassServiceParameters(
	ctx context.Context,
	cl client.Client,
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

	cm := &corev1.ConfigMap{}
	if err := cl.Get(ctx, client.ObjectKey{Namespace: string(*ref.Namespace), Name: ref.Name}, cm); err != nil {
		return gatewayServiceParameters{}, err
	}

	return decodeGatewayServiceParameters(*cm)
}

func decodeGatewayServiceParameters(cm corev1.ConfigMap) (gatewayServiceParameters, error) {
	raw, err := serviceParametersDocument(cm)
	if err != nil {
		return gatewayServiceParameters{}, err
	}

	params := gatewayServiceParameters{}
	if strings.TrimSpace(raw) == "" {
		return params, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(raw)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&params); err != nil {
		return gatewayServiceParameters{}, fmt.Errorf("parse service parameters: %w", err)
	}

	params.normalize()
	if err := params.validate(); err != nil {
		return gatewayServiceParameters{}, err
	}

	return params, nil
}

func serviceParametersDocument(cm corev1.ConfigMap) (string, error) {
	foundKeys := make([]string, 0, len(supportedServiceParameterKeys))
	var document string
	for _, key := range supportedServiceParameterKeys {
		if value, ok := cm.Data[key]; ok {
			foundKeys = append(foundKeys, key)
			document = value
		}
	}
	if len(foundKeys) > 1 {
		return "", fmt.Errorf(
			"ConfigMap %s/%s contains multiple supported service parameter keys (%s)",
			cm.Namespace,
			cm.Name,
			strings.Join(foundKeys, ", "),
		)
	}
	if len(foundKeys) == 1 {
		return document, nil
	}

	return "", fmt.Errorf(
		"ConfigMap %s/%s does not contain any supported service parameter key (%s)",
		cm.Namespace,
		cm.Name,
		strings.Join(supportedServiceParameterKeys, ", "),
	)
}
