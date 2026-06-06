package translator

import (
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
)

const frontendValidationRejectMode = "RejectClientCertificate"

func frontendValidationForListener(
	gateway gatewayv1.Gateway,
	listener gatewayv1.Listener,
	configMaps []corev1.ConfigMap,
	referenceGrants []gatewayv1beta1.ReferenceGrant,
) *ir.FrontendValidation {
	return frontendValidationForListenerWithIndexes(
		gateway,
		listener,
		newTranslatorIndexes(nil, nil, nil, nil, configMaps, referenceGrants),
	)
}

func frontendValidationForListenerWithIndexes(
	gateway gatewayv1.Gateway,
	listener gatewayv1.Listener,
	indexes translatorIndexes,
) *ir.FrontendValidation {
	validation := gatewayapi.FrontendValidationForListener(gateway, listener)
	if validation == nil {
		return nil
	}

	out := &ir.FrontendValidation{
		Mode: string(validation.Mode),
	}
	for _, ref := range validation.CACertificateRefs {
		group := string(ref.Group)
		if group != "" {
			continue
		}

		kind := string(ref.Kind)
		if kind == "" {
			kind = "ConfigMap"
		}
		if kind != "ConfigMap" {
			continue
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
				Kind:  gatewayv1beta1.Kind("ConfigMap"),
				Name:  objectNamePtr(string(ref.Name)),
			},
		) {
			continue
		}

		caPEM := indexes.configMapCAPEM(targetNamespace, string(ref.Name))
		if caPEM == "" {
			continue
		}
		out.ClientCAPEMs = append(out.ClientCAPEMs, caPEM)
	}

	if len(out.ClientCAPEMs) == 0 {
		out.Mode = frontendValidationRejectMode
	}
	return out
}

func configMapCAPEM(configMaps []corev1.ConfigMap, namespace, name string) string {
	return newTranslatorIndexes(nil, nil, nil, nil, configMaps, nil).configMapCAPEM(namespace, name)
}
