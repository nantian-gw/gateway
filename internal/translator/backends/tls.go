package backends

import (
	"crypto/tls"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

func BackendTLSForGatewayWithIndexes(
	gateway gatewayv1.Gateway,
	indexes shared.TranslatorIndexes,
) *ir.BackendTLSConfig {
	backendTLS := gatewayapi.GatewayBackendTLS(gateway)
	if backendTLS == nil || backendTLS.ClientCertificateRef == nil {
		return nil
	}

	ref := backendTLS.ClientCertificateRef
	if RefGroup(ref) != "" {
		return nil
	}

	kind := RefKind(ref)
	if kind != "Secret" {
		return nil
	}

	targetNamespace := shared.NamespaceOrDefault(ref.Namespace, gateway.Namespace)
	if targetNamespace != gateway.Namespace && !ReferenceGranted(
		indexes.ReferenceGrantsByNamespace[targetNamespace],
		targetNamespace,
		gatewayv1beta1.ReferenceGrantFrom{
			Group:     gatewayv1beta1.Group(gatewayv1.GroupVersion.Group),
			Kind:      gatewayv1beta1.Kind("Gateway"),
			Namespace: gatewayv1beta1.Namespace(gateway.Namespace),
		},
		gatewayv1beta1.ReferenceGrantTo{
			Group: gatewayv1beta1.Group(""),
			Kind:  gatewayv1beta1.Kind("Secret"),
			Name:  ObjectNamePtr(string(ref.Name)),
		},
	) {
		return nil
	}

	secret, ok := indexes.TLSSecret(targetNamespace, string(ref.Name))
	if !ok {
		return nil
	}
	if _, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"]); err != nil {
		return nil
	}

	return &ir.BackendTLSConfig{
		ClientCertificateRef: targetNamespace + "/" + string(ref.Name),
	}
}

func RefGroup(ref *gatewayv1.SecretObjectReference) string {
	if ref == nil || ref.Group == nil {
		return ""
	}
	return string(*ref.Group)
}

func RefKind(ref *gatewayv1.SecretObjectReference) string {
	if ref == nil || ref.Kind == nil || string(*ref.Kind) == "" {
		return "Secret"
	}
	return string(*ref.Kind)
}
