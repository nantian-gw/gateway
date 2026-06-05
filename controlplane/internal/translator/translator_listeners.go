package translator

import (
	"crypto/tls"
	"fmt"
	"net"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	"github.com/nantian-gw/gateway/controlplane/internal/gatewayapi"
	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func (t *Translator) translateGatewayListeners(
	gateway gatewayv1.Gateway,
	secrets []corev1.Secret,
	configMaps []corev1.ConfigMap,
	referenceGrants []gatewayv1beta1.ReferenceGrant,
) []ir.Listener {
	return t.translateGatewayListenersWithIndexes(
		gateway,
		newTranslatorIndexes(nil, nil, nil, secrets, configMaps, referenceGrants),
		nil,
		nil,
	)
}

func (t *Translator) translateGatewayListenersWithIndexes(
	gateway gatewayv1.Gateway,
	indexes translatorIndexes,
	listenerSets []gatewayv1.ListenerSet,
	namespaces map[string]corev1.Namespace,
) []ir.Listener {
	listeners := gatewayapi.EffectiveListeners(gateway)
	listeners = mergeListenerSetListeners(gateway, listeners, listenerSets, namespaces)
	out := make([]ir.Listener, 0, len(listeners))
	addresses := gatewayListenerAddresses(gateway.Spec.Addresses)
	address := addresses[0]
	displayAddresses := gatewayStatusAddresses(gateway.Status.Addresses)

	for _, listener := range listeners {
		protocol := string(listener.Protocol)
		if protocol == "TLS" && listener.TLS != nil && listener.TLS.Mode != nil {
			mode := string(*listener.TLS.Mode)
			if mode == "Passthrough" {
				protocol = "TLS_PASSTHROUGH"
			}
			// For Terminate or unset, the protocol stays as "TLS" (which maps to
			// LISTENER_PROTOCOL_TLS in the gRPC snapshot, supporting mixed mode).
			// For HTTPS (terminated TLS), the protocol is "HTTPS".
		}

		item := ir.Listener{
			Name:       gateway.Namespace + "/" + gateway.Name + "/" + string(listener.Name),
			Address:    address,
			Addresses:  append([]string(nil), addresses...),
			Port:       uint32(listener.Port),
			Protocol:   protocol,
			BackendTLS: backendTLSForGatewayWithIndexes(gateway, indexes),
			Metadata: map[string]string{
				"gateway":   gateway.Name,
				"namespace": gateway.Namespace,
			},
			Status: listenerStatusSummary(gateway.Status.Listeners, listener.Name),
		}
		if len(addresses) > 1 {
			item.Metadata[listenerAddressesMetadataKey] = strings.Join(addresses, ",")
		}
		if len(displayAddresses) > 0 {
			item.Metadata[listenerDisplayAddressesMetadataKey] = strings.Join(displayAddresses, ",")
		}

		if listener.Hostname != nil {
			item.Hostnames = []string{string(*listener.Hostname)}
		}

		if listener.TLS != nil {
			tlsConfig := &ir.TLSConfig{
				Enabled: true,
			}
			if listener.TLS.Mode != nil {
				tlsConfig.Passthrough = string(*listener.TLS.Mode) == "Passthrough"
			}
			tlsConfig.SecretRefs = listenerCertificateSecretRefsWithIndexes(gateway, listener, indexes)
			tlsConfig.FrontendValidation = frontendValidationForListenerWithIndexes(gateway, listener, indexes)
			item.TLS = tlsConfig
		}

		out = append(out, item)
	}

	return out
}

func listenerCertificateSecretRefsWithIndexes(
	gateway gatewayv1.Gateway,
	listener gatewayv1.Listener,
	indexes translatorIndexes,
) []string {
	if listener.TLS == nil {
		return nil
	}

	out := make([]string, 0, len(listener.TLS.CertificateRefs))
	seen := make(map[string]struct{}, len(listener.TLS.CertificateRefs))
	for _, ref := range listener.TLS.CertificateRefs {
		if refGroup(&ref) != "" || refKind(&ref) != "Secret" || ref.Name == "" {
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
				Kind:  gatewayv1beta1.Kind("Secret"),
				Name:  objectNamePtr(string(ref.Name)),
			},
		) {
			continue
		}

		secret, ok := indexes.tlsSecret(targetNamespace, string(ref.Name))
		if !ok {
			continue
		}
		if _, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"]); err != nil {
			continue
		}

		key := fmt.Sprintf("%s/%s", targetNamespace, ref.Name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func gatewayListenerAddresses(addresses []gatewayv1.GatewaySpecAddress) []string {
	out := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))

	for _, address := range addresses {
		value := strings.TrimSpace(address.Value)
		if value == "" {
			continue
		}
		if !supportedListenerAddressType(address.Type, value) {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	if len(out) == 0 {
		return []string{"0.0.0.0"}
	}

	sort.Strings(out)
	return out
}

func gatewayStatusAddresses(addresses []gatewayv1.GatewayStatusAddress) []string {
	out := make([]string, 0, len(addresses))
	seen := make(map[string]struct{}, len(addresses))

	for _, address := range addresses {
		value := strings.TrimSpace(address.Value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	sort.Strings(out)
	return out
}

func supportedListenerAddressType(rawType *gatewayv1.AddressType, value string) bool {
	switch {
	case rawType == nil || *rawType == "":
		return net.ParseIP(value) != nil || value != ""
	case *rawType == gatewayv1.IPAddressType:
		return net.ParseIP(value) != nil
	case *rawType == gatewayv1.HostnameAddressType:
		return value != ""
	default:
		return false
	}
}

func mergeListenerSetListeners(
	gateway gatewayv1.Gateway,
	base []gatewayv1.Listener,
	sets []gatewayv1.ListenerSet,
	namespaces map[string]corev1.Namespace,
) []gatewayv1.Listener {
	if len(sets) == 0 {
		return base
	}

	// Sort ListenerSets: oldest first, alphabetical tiebreaker.
	sorted := make([]gatewayv1.ListenerSet, len(sets))
	copy(sorted, sets)
	sort.SliceStable(sorted, func(i, j int) bool {
		ti := sorted[i].CreationTimestamp.Time
		tj := sorted[j].CreationTimestamp.Time
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		ni := sorted[i].Namespace + "/" + sorted[i].Name
		nj := sorted[j].Namespace + "/" + sorted[j].Name
		return ni < nj
	})

	// Collect Gateway listeners into a set for conflict detection.
	type listenerKey struct {
		port     gatewayv1.PortNumber
		protocol string
		hostname string
	}
	seen := make(map[listenerKey]bool, len(base))
	for _, l := range base {
		host := ""
		if l.Hostname != nil {
			host = string(*l.Hostname)
		}
		seen[listenerKey{l.Port, string(l.Protocol), host}] = true
	}

	out := make([]gatewayv1.Listener, len(base))
	copy(out, base)

	for _, ls := range sorted {
		// Only attach to Gateway if the Gateway allows it.
		if !gatewayAllowsListenerSet(gateway, ls, namespaces) {
			continue
		}
		for _, entry := range ls.Spec.Listeners {
			host := ""
			if entry.Hostname != nil {
				host = string(*entry.Hostname)
			}
			key := listenerKey{entry.Port, string(entry.Protocol), host}
			if seen[key] {
				continue // Gateway or older ListenerSet wins
			}
			seen[key] = true
			out = append(out, gatewayv1.Listener{
				Name:     entry.Name,
				Hostname: entry.Hostname,
				Port:     entry.Port,
				Protocol: entry.Protocol,
				TLS:      entry.TLS,
			})
		}
	}

	return out
}

func gatewayAllowsListenerSet(gateway gatewayv1.Gateway, ls gatewayv1.ListenerSet, namespaces map[string]corev1.Namespace) bool {
	if gateway.Spec.AllowedListeners == nil || gateway.Spec.AllowedListeners.Namespaces == nil {
		return false
	}
	ns := gateway.Spec.AllowedListeners.Namespaces
	if ns.From == nil {
		return false
	}
	switch *ns.From {
	case gatewayv1.NamespacesFromAll:
		return true
	case gatewayv1.NamespacesFromSame:
		return gateway.Namespace == ls.Namespace
	case gatewayv1.NamespacesFromSelector:
		if ns.Selector == nil {
			return false
		}
		selector, err := metav1.LabelSelectorAsSelector(ns.Selector)
		if err != nil {
			return false
		}
		nsObj, ok := namespaces[ls.Namespace]
		if !ok {
			return false
		}
		return selector.Matches(labels.Set(nsObj.Labels))
	default:
		return false
	}
}
