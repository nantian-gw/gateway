package translator

import (
	"crypto/tls"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func backendTLSForGateway(
	gateway gatewayv1.Gateway,
	secrets []corev1.Secret,
	referenceGrants []gatewayv1beta1.ReferenceGrant,
) *ir.BackendTLSConfig {
	return backendTLSForGatewayWithIndexes(
		gateway,
		newTranslatorIndexes(nil, nil, nil, secrets, nil, referenceGrants),
	)
}

func backendTLSForGatewayWithIndexes(
	gateway gatewayv1.Gateway,
	indexes translatorIndexes,
) *ir.BackendTLSConfig {
	backendTLS := gatewayapi.GatewayBackendTLS(gateway)
	if backendTLS == nil || backendTLS.ClientCertificateRef == nil {
		return nil
	}

	ref := backendTLS.ClientCertificateRef
	if refGroup(ref) != "" {
		return nil
	}

	kind := refKind(ref)
	if kind != "Secret" {
		return nil
	}

	targetNamespace := namespaceOrDefault(ref.Namespace, gateway.Namespace)
	if targetNamespace != gateway.Namespace && !referenceGranted(
		indexes.referenceGrantsByNamespace[targetNamespace],
		targetNamespace,
		gatewayv1beta1.ReferenceGrantFrom{
			Group:     gatewayv1beta1.Group(gatewayv1.GroupVersion.Group),
			Kind:      gatewayv1beta1.Kind("Gateway"),
			Namespace: gatewayv1beta1.Namespace(gateway.Namespace),
		},
		gatewayv1beta1.ReferenceGrantTo{
			Group: gatewayv1beta1.Group(""),
			Kind:  gatewayv1beta1.Kind("Secret"),
			Name:  objectNamePtr(string(ref.Name)),
		},
	) {
		return nil
	}

	secret, ok := indexes.tlsSecret(targetNamespace, string(ref.Name))
	if !ok {
		return nil
	}
	if _, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"]); err != nil {
		return nil
	}

	return &ir.BackendTLSConfig{
		ClientCertificateRef: fmt.Sprintf("%s/%s", targetNamespace, ref.Name),
	}
}

func tlsSecret(secrets []corev1.Secret, namespace, name string) (corev1.Secret, bool) {
	return newTranslatorIndexes(nil, nil, nil, secrets, nil, nil).tlsSecret(namespace, name)
}

func refGroup(ref *gatewayv1.SecretObjectReference) string {
	if ref == nil || ref.Group == nil {
		return ""
	}
	return string(*ref.Group)
}

func refKind(ref *gatewayv1.SecretObjectReference) string {
	if ref == nil || ref.Kind == nil || string(*ref.Kind) == "" {
		return "Secret"
	}
	return string(*ref.Kind)
}
