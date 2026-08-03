package listeners

import (
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/translator/backends"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

// NOTE: "RejectClientCertificate" is a project-specific mode string,
// not a standard sigs.k8s.io/gateway-api constant. Standard Gateway API
// constants are FrontendTLSValidationModeAccept, FrontendTLSValidationModeReject,
// and FrontendTLSValidationModeRequestClientCertificate (v1).
const frontendValidationRejectMode = "RejectClientCertificate"

func FrontendValidationForListener(
	gateway gatewayv1.Gateway,
	listener gatewayv1.Listener,
	configMaps []corev1.ConfigMap,
	referenceGrants []gatewayv1beta1.ReferenceGrant,
) *ir.FrontendValidation {
	return FrontendValidationForListenerWithIndexes(
		gateway,
		listener,
		shared.NewTranslatorIndexes(nil, nil, nil, nil, configMaps, referenceGrants),
	)
}

func FrontendValidationForListenerWithIndexes(
	gateway gatewayv1.Gateway,
	listener gatewayv1.Listener,
	indexes shared.TranslatorIndexes,
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

		targetNamespace := shared.NamespaceOrDefault(ref.Namespace, gateway.Namespace)
		if targetNamespace != gateway.Namespace && !backends.ReferenceGranted(
			indexes.ReferenceGrantsByNamespace[targetNamespace],
			targetNamespace,
			gatewayv1beta1.ReferenceGrantFrom{
				Group:     gatewayv1beta1.Group(gatewayv1.GroupVersion.Group),
				Kind:      gatewayv1beta1.Kind("Gateway"),
				Namespace: gatewayv1beta1.Namespace(gateway.Namespace),
			},
			gatewayv1beta1.ReferenceGrantTo{
				Group: gatewayv1beta1.Group(""),
				Kind:  gatewayv1beta1.Kind("ConfigMap"),
				Name:  backends.ObjectNamePtr(string(ref.Name)),
			},
		) {
			continue
		}

		caPEM := indexes.ConfigMapCAPEM(targetNamespace, string(ref.Name))
		if caPEM == "" {
			continue
		}
		out.ClientCAPEMs = append(out.ClientCAPEMs, caPEM)
	}

	if len(out.ClientCAPEMs) == 0 {
		originalMode := out.Mode
		slog.Warn("no CA PEMs found for frontend validation, forcing reject mode",
			"originalMode", originalMode,
		)
		out.Mode = frontendValidationRejectMode
	}
	return out
}
