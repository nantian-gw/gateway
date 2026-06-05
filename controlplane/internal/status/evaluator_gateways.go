package status

import (
	"crypto/tls"
	"crypto/x509"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapi"
)

const (
	listenerReasonInvalidCACertificateRef  = "InvalidCACertificateRef"
	listenerReasonInvalidCACertificateKind = "InvalidCACertificateKind"
	listenerReasonNoValidCACertificate     = "NoValidCACertificate"
)

func evaluateGatewayListener(
	state *clusterState,
	gateway gatewayv1.Gateway,
	allListeners []gatewayv1.Listener,
	listener gatewayv1.Listener,
	attachedRoutes int32,
) listenerEvaluation {
	policy := buildListenerPolicy(listener)
	accepted := acceptedListenerCondition(gateway.Generation)
	resolved := resolvedListenerCondition(gateway.Generation)
	programmed := programmedListenerCondition(gateway.Generation)
	extraConditions := make([]conditionSpec, 0, 2)
	allowProgrammedWithUnresolvedRefs := false

	switch {
	case !listenerProtocolSupported(listener.Protocol):
		accepted = rejectedListenerCondition(
			gateway.Generation,
			string(gatewayv1.ListenerReasonUnsupportedProtocol),
			"Listener protocol is not supported by aether-gateway",
		)
	case policy.invalidKindRefs:
		resolved.Status = metav1.ConditionFalse
		resolved.Reason = string(gatewayv1.ListenerReasonInvalidRouteKinds)
		resolved.Message = "Listener contains unsupported route kinds"
	default:
		if reason, message, ok := evaluateListenerSpec(listener); !ok {
			accepted = rejectedListenerCondition(gateway.Generation, reason, message)
			policy.supportedKinds = []gatewayv1.RouteGroupKind{}
		} else if reason, message, ok := evaluateListenerConflict(allListeners, listener); !ok {
			accepted = rejectedListenerCondition(gateway.Generation, reason, message)
			extraConditions = append(extraConditions, conditionSpec{
				Type:               string(gatewayv1.ListenerConditionConflicted),
				Status:             metav1.ConditionTrue,
				Reason:             reason,
				Message:            message,
				ObservedGeneration: gateway.Generation,
			})
		} else if refs := evaluateListenerTLSRefs(state, gateway, listener); !refs.ok {
			resolved.Status = metav1.ConditionFalse
			resolved.Reason = refs.reason
			resolved.Message = refs.message
			allowProgrammedWithUnresolvedRefs = refs.allowProgrammedWithUnresolvedRefs
			if refs.noValidCACertificate {
				accepted = rejectedListenerCondition(
					gateway.Generation,
					listenerReasonNoValidCACertificate,
					"FrontendValidation does not contain any valid CA certificate references",
				)
			}
		}
	}

	switch {
	case accepted.Status != metav1.ConditionTrue:
		programmed.Status = metav1.ConditionFalse
		programmed.Reason = accepted.Reason
		programmed.Message = accepted.Message
	case resolved.Status != metav1.ConditionTrue && !allowProgrammedWithUnresolvedRefs:
		programmed.Status = metav1.ConditionFalse
		programmed.Reason = string(gatewayv1.ListenerReasonInvalid)
		programmed.Message = resolved.Message
	}

	return listenerEvaluation{
		name:                listener.Name,
		supportedKinds:      policy.supportedKinds,
		attachedRoutes:      attachedRoutes,
		acceptedCondition:   accepted,
		resolvedCondition:   resolved,
		programmedCondition: programmed,
		extraConditions:     extraConditions,
	}
}

type listenerTLSRefEvaluation struct {
	reason                            string
	message                           string
	ok                                bool
	noValidCACertificate              bool
	allowProgrammedWithUnresolvedRefs bool
}

func listenerProtocolSupported(protocol gatewayv1.ProtocolType) bool {
	switch strings.ToUpper(string(protocol)) {
	case "HTTP", "HTTPS", "TLS", "TCP", "UDP":
		return true
	default:
		return false
	}
}

func evaluateListenerSpec(listener gatewayv1.Listener) (reason string, message string, ok bool) {
	switch strings.ToUpper(string(listener.Protocol)) {
	case "HTTP", "TCP", "UDP":
		if listener.TLS != nil {
			return string(gatewayv1.ListenerReasonInvalid), "TLS configuration is not allowed for HTTP, TCP, or UDP listeners", false
		}
		if (strings.EqualFold(string(listener.Protocol), "TCP") || strings.EqualFold(string(listener.Protocol), "UDP")) &&
			listener.Hostname != nil && strings.TrimSpace(string(*listener.Hostname)) != "" {
			return string(gatewayv1.ListenerReasonInvalid), "Hostname is not allowed for TCP or UDP listeners", false
		}
	case "HTTPS", "TLS":
		if listener.TLS == nil {
			return string(gatewayv1.ListenerReasonInvalid), "TLS configuration is required for HTTPS and TLS listeners", false
		}
		mode := listenerTLSMode(listener)
		if strings.EqualFold(string(listener.Protocol), "HTTPS") && mode != gatewayv1.TLSModeTerminate {
			return string(gatewayv1.ListenerReasonInvalid), "HTTPS listeners must use TLS mode Terminate", false
		}
	}

	return "", "", true
}

func evaluateListenerConflict(listeners []gatewayv1.Listener, current gatewayv1.Listener) (reason string, message string, ok bool) {
	for _, other := range listeners {
		if other.Name == current.Name {
			continue
		}
		if current.Port != other.Port {
			continue
		}

		if listenersProtocolConflict(current, other) {
			return string(gatewayv1.ListenerReasonProtocolConflict), "Listener protocol conflicts with another listener on the same port", false
		}
		if listenerHostnamesOverlap(current, other) && listenerHostnamesConflict(current, other) {
			return string(gatewayv1.ListenerReasonHostnameConflict), "Listener hostname conflicts with another listener on the same port", false
		}
	}

	return "", "", true
}

func listenerHostnamesOverlap(left, right gatewayv1.Listener) bool {
	leftHostname, leftSpecified := listenerHostname(left)
	rightHostname, rightSpecified := listenerHostname(right)
	if !leftSpecified && !rightSpecified {
		return true
	}
	if !leftSpecified || !rightSpecified {
		return false
	}
	return normalizeHostname(leftHostname) == normalizeHostname(rightHostname)
}

func listenersProtocolConflict(left, right gatewayv1.Listener) bool {
	leftProtocol := strings.ToUpper(string(left.Protocol))
	rightProtocol := strings.ToUpper(string(right.Protocol))
	if leftProtocol != rightProtocol {
		return true
	}

	return false
}

func listenerHostnamesConflict(left, right gatewayv1.Listener) bool {
	leftHostname, leftSpecified := listenerHostname(left)
	rightHostname, rightSpecified := listenerHostname(right)
	if !leftSpecified && !rightSpecified {
		return true
	}
	if !leftSpecified || !rightSpecified {
		return false
	}
	return leftHostname == rightHostname
}

func listenerHostname(listener gatewayv1.Listener) (string, bool) {
	if listener.Hostname == nil {
		return "", false
	}
	value := strings.TrimSpace(string(*listener.Hostname))
	if value == "" {
		return "", false
	}
	return value, true
}

func listenerTLSMode(listener gatewayv1.Listener) gatewayv1.TLSModeType {
	if listener.TLS == nil || listener.TLS.Mode == nil || *listener.TLS.Mode == "" {
		return gatewayv1.TLSModeTerminate
	}
	return *listener.TLS.Mode
}

func resolvedListenerCondition(generation int64) conditionSpec {
	return conditionSpec{
		Type:               string(gatewayv1.ListenerConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.ListenerReasonResolvedRefs),
		Message:            "Listener references are resolved",
		ObservedGeneration: generation,
	}
}

func programmedListenerCondition(generation int64) conditionSpec {
	return conditionSpec{
		Type:               string(gatewayv1.ListenerConditionProgrammed),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.ListenerReasonProgrammed),
		Message:            "Listener is programmed",
		ObservedGeneration: generation,
	}
}

func rejectedListenerCondition(generation int64, reason, message string) conditionSpec {
	return conditionSpec{
		Type:               string(gatewayv1.ListenerConditionAccepted),
		Status:             metav1.ConditionFalse,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: generation,
	}
}

func evaluateListenerTLSRefs(
	state *clusterState,
	gateway gatewayv1.Gateway,
	listener gatewayv1.Listener,
) listenerTLSRefEvaluation {
	if listener.TLS != nil {
		var (
			firstReason          string
			firstMessage         string
			validCertificateRefs int
		)
		for _, certificateRef := range listener.TLS.CertificateRefs {
			group := stringOrEmpty(certificateRef.Group)
			if group != "" {
				if firstReason == "" {
					firstReason = string(gatewayv1.ListenerReasonInvalidCertificateRef)
					firstMessage = "CertificateRef group is not supported"
				}
				continue
			}

			kind := stringOrEmpty(certificateRef.Kind)
			if kind == "" {
				kind = "Secret"
			}
			if kind != "Secret" {
				if firstReason == "" {
					firstReason = string(gatewayv1.ListenerReasonInvalidCertificateRef)
					firstMessage = "CertificateRef kind is not supported"
				}
				continue
			}

			targetNamespace := namespaceOrDefault(certificateRef.Namespace, gateway.Namespace)
			if targetNamespace != gateway.Namespace && !referenceGranted(
				state.referenceGrants,
				targetNamespace,
				gatewayv1beta1.ReferenceGrantFrom{
					Group:     gatewayv1beta1.Group(gatewayGroup),
					Kind:      gatewayv1beta1.Kind("Gateway"),
					Namespace: gatewayv1beta1.Namespace(gateway.Namespace),
				},
				gatewayv1beta1.ReferenceGrantTo{
					Group: gatewayv1beta1.Group(""),
					Kind:  gatewayv1beta1.Kind("Secret"),
					Name:  objectNamePtr(string(certificateRef.Name)),
				},
			) {
				if firstReason == "" {
					firstReason = string(gatewayv1.ListenerReasonRefNotPermitted)
					firstMessage = "Cross-namespace CertificateRef is not permitted"
				}
				continue
			}

			secret, ok := state.secretByKey[namespacedName(targetNamespace, string(certificateRef.Name))]
			if !ok || secret.Type != corev1.SecretTypeTLS || len(secret.Data["tls.crt"]) == 0 || len(secret.Data["tls.key"]) == 0 {
				if firstReason == "" {
					firstReason = string(gatewayv1.ListenerReasonInvalidCertificateRef)
					firstMessage = "CertificateRef does not point to a valid TLS Secret"
				}
				continue
			}
			if _, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"]); err != nil {
				if firstReason == "" {
					firstReason = string(gatewayv1.ListenerReasonInvalidCertificateRef)
					firstMessage = "CertificateRef does not point to a valid TLS Secret"
				}
				continue
			}
			validCertificateRefs++
		}

		if firstReason != "" {
			return listenerTLSRefEvaluation{
				reason:                            firstReason,
				message:                           firstMessage,
				allowProgrammedWithUnresolvedRefs: validCertificateRefs > 0,
			}
		}

		if validation := gatewayapi.FrontendValidationForListener(gateway, listener); validation != nil {
			var (
				firstReason  string
				firstMessage string
				validCARefs  int
			)
			for _, caRef := range validation.CACertificateRefs {
				reason, message, ok := evaluateFrontendValidationCARef(state, gateway, caRef)
				if !ok {
					if firstReason == "" {
						firstReason = reason
						firstMessage = message
					}
					continue
				}
				validCARefs++
			}
			if firstReason != "" {
				return listenerTLSRefEvaluation{
					reason:                            firstReason,
					message:                           firstMessage,
					noValidCACertificate:              validCARefs == 0,
					allowProgrammedWithUnresolvedRefs: validCARefs > 0,
				}
			}
		}
	}

	if refs := evaluateGatewayBackendTLSRef(state, gateway); !refs.ok {
		return refs
	}

	return listenerTLSRefEvaluation{
		reason:  string(gatewayv1.ListenerReasonResolvedRefs),
		message: "Listener references are resolved",
		ok:      true,
	}
}

func evaluateGatewayBackendTLSRef(state *clusterState, gateway gatewayv1.Gateway) listenerTLSRefEvaluation {
	backendTLS := gatewayapi.GatewayBackendTLS(gateway)
	if backendTLS == nil || backendTLS.ClientCertificateRef == nil {
		return listenerTLSRefEvaluation{ok: true}
	}

	clientCertRef := backendTLS.ClientCertificateRef
	group := stringOrEmpty(clientCertRef.Group)
	if group != "" {
		return listenerTLSRefEvaluation{
			reason:  string(gatewayv1.ListenerReasonInvalidCertificateRef),
			message: "BackendTLS clientCertificateRef group is not supported",
		}
	}

	kind := stringOrEmpty(clientCertRef.Kind)
	if kind == "" {
		kind = "Secret"
	}
	if kind != "Secret" {
		return listenerTLSRefEvaluation{
			reason:  string(gatewayv1.ListenerReasonInvalidCertificateRef),
			message: "BackendTLS clientCertificateRef kind is not supported",
		}
	}

	targetNamespace := namespaceOrDefault(clientCertRef.Namespace, gateway.Namespace)
	if targetNamespace != gateway.Namespace && !referenceGranted(
		state.referenceGrants,
		targetNamespace,
		gatewayv1beta1.ReferenceGrantFrom{
			Group:     gatewayv1beta1.Group(gatewayGroup),
			Kind:      gatewayv1beta1.Kind("Gateway"),
			Namespace: gatewayv1beta1.Namespace(gateway.Namespace),
		},
		gatewayv1beta1.ReferenceGrantTo{
			Group: gatewayv1beta1.Group(""),
			Kind:  gatewayv1beta1.Kind("Secret"),
			Name:  objectNamePtr(string(clientCertRef.Name)),
		},
	) {
		return listenerTLSRefEvaluation{
			reason:  string(gatewayv1.ListenerReasonRefNotPermitted),
			message: "Cross-namespace BackendTLS clientCertificateRef is not permitted",
		}
	}

	secret, ok := state.secretByKey[namespacedName(targetNamespace, string(clientCertRef.Name))]
	if !ok || secret.Type != corev1.SecretTypeTLS || len(secret.Data["tls.crt"]) == 0 || len(secret.Data["tls.key"]) == 0 {
		return listenerTLSRefEvaluation{
			reason:  string(gatewayv1.ListenerReasonInvalidCertificateRef),
			message: "BackendTLS clientCertificateRef does not point to a valid TLS Secret",
		}
	}
	if _, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"]); err != nil {
		return listenerTLSRefEvaluation{
			reason:  string(gatewayv1.ListenerReasonInvalidCertificateRef),
			message: "BackendTLS clientCertificateRef does not point to a valid TLS Secret",
		}
	}

	return listenerTLSRefEvaluation{ok: true}
}

func acceptedListenerCondition(generation int64) conditionSpec {
	return conditionSpec{
		Type:               string(gatewayv1.ListenerConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.ListenerReasonAccepted),
		Message:            "Listener is accepted by aether-gateway",
		ObservedGeneration: generation,
	}
}

func gatewayExtraConditions(state *clusterState, gateway gatewayv1.Gateway) []conditionSpec {
	out := gatewayFrontendValidationConditions(gateway)
	if gatewayapi.GatewayActsAsDefault(gateway) {
		out = append(out, conditionSpec{
			Type:               gatewayapi.GatewayConditionDefaultGateway,
			Status:             metav1.ConditionTrue,
			Reason:             gatewayapi.GatewayReasonDefaultGateway,
			Message:            "Gateway has default scope " + string(gateway.Spec.DefaultScope),
			ObservedGeneration: gateway.Generation,
		})
	}
	if condition, ok := gatewayBackendTLSResolvedRefsCondition(state, gateway); ok {
		out = append(out, condition)
	}

	return out
}

func gatewayBackendTLSResolvedRefsCondition(state *clusterState, gateway gatewayv1.Gateway) (conditionSpec, bool) {
	backendTLS := gatewayapi.GatewayBackendTLS(gateway)
	if backendTLS == nil || backendTLS.ClientCertificateRef == nil {
		return conditionSpec{}, false
	}

	refs := evaluateGatewayBackendTLSRef(state, gateway)
	condition := conditionSpec{
		Type:               string(gatewayv1.GatewayConditionResolvedRefs),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonResolvedRefs),
		Message:            "Gateway backend TLS references are resolved",
		ObservedGeneration: gateway.Generation,
	}
	if refs.ok {
		return condition, true
	}

	condition.Status = metav1.ConditionFalse
	condition.Message = refs.message
	if refs.reason == string(gatewayv1.ListenerReasonRefNotPermitted) {
		condition.Reason = string(gatewayv1.GatewayReasonRefNotPermitted)
	} else {
		condition.Reason = string(gatewayv1.GatewayReasonInvalidClientCertificateRef)
	}

	return condition, true
}

func gatewayFrontendValidationConditions(gateway gatewayv1.Gateway) []conditionSpec {
	for _, listener := range gateway.Spec.Listeners {
		validation := gatewayapi.FrontendValidationForListener(gateway, listener)
		if validation == nil || validation.Mode != gatewayv1.AllowInsecureFallback {
			continue
		}

		return []conditionSpec{{
			Type:    string(gatewayv1.GatewayConditionInsecureFrontendValidationMode),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayv1.GatewayReasonConfigurationChanged),
			Message: "One or more HTTPS listeners use AllowInsecureFallback client certificate validation mode",
		}}
	}

	return nil
}

func evaluateFrontendValidationCARef(
	state *clusterState,
	gateway gatewayv1.Gateway,
	caRef gatewayv1.ObjectReference,
) (reason string, message string, ok bool) {
	group := string(caRef.Group)
	if group != "" {
		return listenerReasonInvalidCACertificateKind, "FrontendValidation CA ref group is not supported", false
	}

	kind := string(caRef.Kind)
	if kind == "" {
		kind = "ConfigMap"
	}
	if kind != "ConfigMap" {
		return listenerReasonInvalidCACertificateKind, "FrontendValidation CA ref kind is not supported", false
	}

	targetNamespace := namespaceOrDefault(caRef.Namespace, gateway.Namespace)
	if targetNamespace != gateway.Namespace && !referenceGranted(
		state.referenceGrants,
		targetNamespace,
		gatewayv1beta1.ReferenceGrantFrom{
			Group:     gatewayv1beta1.Group(gatewayGroup),
			Kind:      gatewayv1beta1.Kind("Gateway"),
			Namespace: gatewayv1beta1.Namespace(gateway.Namespace),
		},
		gatewayv1beta1.ReferenceGrantTo{
			Group: gatewayv1beta1.Group(""),
			Kind:  gatewayv1beta1.Kind("ConfigMap"),
			Name:  objectNamePtr(string(caRef.Name)),
		},
	) {
		return string(gatewayv1.ListenerReasonRefNotPermitted), "Cross-namespace FrontendValidation CA ref is not permitted", false
	}

	configMap, ok := state.configMapByKey[namespacedName(targetNamespace, string(caRef.Name))]
	caPEM := []byte(configMap.Data["ca.crt"])
	if !ok || len(caPEM) == 0 {
		return listenerReasonInvalidCACertificateRef, "FrontendValidation CA ref does not point to a valid ConfigMap", false
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return listenerReasonInvalidCACertificateRef, "FrontendValidation CA ref does not point to a valid ConfigMap", false
	}

	return "", "", true
}
