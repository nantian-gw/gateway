package translator

import (
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gwapi"
	"github.com/nantian-gw/gateway/internal/ir"
)

const (
	backendRefMetaValid  = "nantian.dev/backend-ref-valid"
	backendRefMetaReason = "nantian.dev/backend-ref-reason"
)

type routeKind string

const (
	routeKindHTTP routeKind = "HTTPRoute"
	routeKindGRPC routeKind = "GRPCRoute"
	routeKindTCP  routeKind = "TCPRoute"
	routeKindUDP  routeKind = "UDPRoute"
	routeKindTLS  routeKind = "TLSRoute"
)

type backendRefTranslator struct {
	servicePorts               map[string]map[uint32]struct{}
	serviceImportPorts         map[string]map[uint32]struct{}
	referenceGrantsByNamespace map[string][]gatewayv1beta1.ReferenceGrant
	extensionResolver          extfilter.Resolver
}

func newBackendRefTranslator(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	referenceGrants []gatewayv1beta1.ReferenceGrant,
	extensionResolver extfilter.Resolver,
) backendRefTranslator {
	servicePorts := make(map[string]map[uint32]struct{}, len(services))
	for _, service := range services {
		key := backendObjectKey(service.Namespace, service.Name)
		ports := make(map[uint32]struct{}, len(service.Spec.Ports))
		for _, port := range service.Spec.Ports {
			ports[uint32(port.Port)] = struct{}{}
		}
		servicePorts[key] = ports
	}

	serviceImportPorts := make(map[string]map[uint32]struct{}, len(serviceImports))
	for _, serviceImport := range serviceImports {
		key := backendObjectKey(serviceImport.Namespace, serviceImport.Name)
		ports := make(map[uint32]struct{}, len(serviceImport.Spec.Ports))
		for _, port := range serviceImport.Spec.Ports {
			ports[uint32(port.Port)] = struct{}{}
		}
		serviceImportPorts[key] = ports
	}

	return backendRefTranslator{
		servicePorts:               servicePorts,
		serviceImportPorts:         serviceImportPorts,
		referenceGrantsByNamespace: indexReferenceGrantsByNamespace(referenceGrants),
		extensionResolver:          extensionResolver,
	}
}

func (t backendRefTranslator) annotateHTTPRoute(target *ir.HTTPRoute, source gatewayv1.HTTPRoute) {
	allowCrossNamespaceRefs := routeUsesOnlyServiceParents(target.ParentRefs)
	validation := gwapi.ValidateHTTPRouteRules(source)
	invalidRules := make(map[int]struct{}, len(validation.InvalidRuleIndexes))
	for _, index := range validation.InvalidRuleIndexes {
		invalidRules[index] = struct{}{}
	}
	targetRuleIndex := 0
	for index, rule := range source.Spec.Rules {
		if _, invalid := invalidRules[index]; invalid {
			continue
		}
		if targetRuleIndex >= len(target.Rules) {
			break
		}
		target.Rules[targetRuleIndex].BackendRefs = t.httpBackendRefs(
			source.Namespace,
			routeKindHTTP,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
		targetRuleIndex++
	}
}

func (t backendRefTranslator) annotateGRPCRoute(target *ir.GRPCRoute, source gatewayv1.GRPCRoute) {
	allowCrossNamespaceRefs := routeUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.grpcBackendRefs(
			source.Namespace,
			routeKindGRPC,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t backendRefTranslator) annotateTCPRoute(target *ir.StreamRoute, source gatewayv1alpha2.TCPRoute) {
	allowCrossNamespaceRefs := routeUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.routeBackendRefs(
			source.Namespace,
			routeKindTCP,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t backendRefTranslator) annotateUDPRoute(target *ir.StreamRoute, source gatewayv1alpha2.UDPRoute) {
	allowCrossNamespaceRefs := routeUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.routeBackendRefs(
			source.Namespace,
			routeKindUDP,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t backendRefTranslator) annotateTLSRoute(target *ir.StreamRoute, source gatewayv1alpha2.TLSRoute) {
	allowCrossNamespaceRefs := routeUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.routeBackendRefs(
			source.Namespace,
			routeKindTLS,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t backendRefTranslator) httpBackendRefs(
	routeNamespace string,
	routeKind routeKind,
	allowCrossNamespaceRefs bool,
	refs []gatewayv1.HTTPBackendRef,
) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, t.annotateBackendRef(
			routeNamespace,
			routeKind,
			allowCrossNamespaceRefs,
			ir.BackendRef{
				Group:     stringValue(ref.BackendRef.Group),
				Kind:      stringValue(ref.BackendRef.Kind),
				Namespace: namespaceOrDefault(ref.BackendRef.Namespace, routeNamespace),
				Name:      string(ref.BackendRef.Name),
				Port:      portValue(ref.BackendRef.Port),
				Weight:    uint32(weightValue(ref.Weight)),
				Filters: filtersFromHTTPWithResolver(
					ref.Filters,
					routeNamespace,
					t.extensionResolver,
					extfilter.TargetHTTP,
					nil,
					0,
				),
			},
		))
	}
	return out
}

func (t backendRefTranslator) grpcBackendRefs(
	routeNamespace string,
	routeKind routeKind,
	allowCrossNamespaceRefs bool,
	refs []gatewayv1.GRPCBackendRef,
) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, t.annotateBackendRef(
			routeNamespace,
			routeKind,
			allowCrossNamespaceRefs,
			ir.BackendRef{
				Group:     stringValue(ref.BackendRef.Group),
				Kind:      stringValue(ref.BackendRef.Kind),
				Namespace: namespaceOrDefault(ref.BackendRef.Namespace, routeNamespace),
				Name:      string(ref.BackendRef.Name),
				Port:      portValue(ref.BackendRef.Port),
				Weight:    uint32(weightValue(ref.Weight)),
				Filters: filtersFromGRPCWithResolver(
					ref.Filters,
					routeNamespace,
					t.extensionResolver,
					extfilter.TargetGRPC,
				),
			},
		))
	}
	return out
}

func (t backendRefTranslator) routeBackendRefs(
	routeNamespace string,
	routeKind routeKind,
	allowCrossNamespaceRefs bool,
	refs []gatewayv1.BackendRef,
) []ir.BackendRef {
	out := make([]ir.BackendRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, t.annotateBackendRef(
			routeNamespace,
			routeKind,
			allowCrossNamespaceRefs,
			ir.BackendRef{
				Group:     stringValue(ref.Group),
				Kind:      stringValue(ref.Kind),
				Namespace: namespaceOrDefault(ref.Namespace, routeNamespace),
				Name:      string(ref.Name),
				Port:      portValue(ref.Port),
				Weight:    uint32(weightValue(ref.Weight)),
			},
		))
	}
	return out
}

func (t backendRefTranslator) annotateBackendRef(
	routeNamespace string,
	routeKind routeKind,
	allowCrossNamespaceRefs bool,
	ref ir.BackendRef,
) ir.BackendRef {
	if metadata := t.backendRefMetadata(routeNamespace, routeKind, allowCrossNamespaceRefs, ref); len(metadata) > 0 {
		ref.Metadata = metadata
	}
	return ref
}

func (t backendRefTranslator) backendRefMetadata(
	routeNamespace string,
	routeKind routeKind,
	allowCrossNamespaceRefs bool,
	ref ir.BackendRef,
) map[string]string {
	targetGroup := ref.Group
	if targetGroup != "" && targetGroup != mcsv1alpha1.GroupName {
		return invalidBackendRefMetadata(string(gatewayv1.RouteReasonInvalidKind))
	}

	targetKind, ok := backendKindForRef(ref.Group, ref.Kind)
	if !ok {
		return invalidBackendRefMetadata(string(gatewayv1.RouteReasonInvalidKind))
	}

	if ref.Namespace != routeNamespace && !allowCrossNamespaceRefs && !referenceGranted(
		t.referenceGrantsByNamespace[ref.Namespace],
		ref.Namespace,
		gatewayv1beta1.ReferenceGrantFrom{
			Group:     gatewayv1beta1.Group(routeGroupForKind(routeKind)),
			Kind:      gatewayv1beta1.Kind(routeKindName(routeKind)),
			Namespace: gatewayv1beta1.Namespace(routeNamespace),
		},
		gatewayv1beta1.ReferenceGrantTo{
			Group: gatewayv1beta1.Group(targetGroup),
			Kind:  gatewayv1beta1.Kind(targetKind),
			Name:  objectNamePtr(ref.Name),
		},
	) {
		return invalidBackendRefMetadata(string(gatewayv1.RouteReasonRefNotPermitted))
	}

	switch targetKind {
	case "Service":
		if !t.serviceExists(ref.Namespace, ref.Name, ref.Port) {
			return invalidBackendRefMetadata(string(gatewayv1.RouteReasonBackendNotFound))
		}
	case "ServiceImport":
		if !t.serviceImportExists(ref.Namespace, ref.Name, ref.Port) {
			return invalidBackendRefMetadata(string(gatewayv1.RouteReasonBackendNotFound))
		}
	default:
		return invalidBackendRefMetadata(string(gatewayv1.RouteReasonBackendNotFound))
	}

	return nil
}

func (t backendRefTranslator) serviceExists(namespace string, name string, port uint32) bool {
	ports, ok := t.servicePorts[backendObjectKey(namespace, name)]
	if !ok {
		return false
	}
	if port == 0 {
		return true
	}
	_, ok = ports[port]
	return ok
}

func (t backendRefTranslator) serviceImportExists(namespace string, name string, port uint32) bool {
	ports, ok := t.serviceImportPorts[backendObjectKey(namespace, name)]
	if !ok {
		return false
	}
	if port == 0 {
		return true
	}
	_, ok = ports[port]
	return ok
}

func backendKindForRef(group string, kind string) (string, bool) {
	if group == "" {
		if kind == "" || kind == "Service" {
			return "Service", true
		}
		return "", false
	}
	if group == mcsv1alpha1.GroupName && kind == "ServiceImport" {
		return "ServiceImport", true
	}
	return "", false
}

func invalidBackendRefMetadata(reason string) map[string]string {
	return map[string]string{
		backendRefMetaValid:  "false",
		backendRefMetaReason: reason,
	}
}

func backendObjectKey(namespace string, name string) string {
	return namespace + "/" + name
}

func referenceGranted(
	grants []gatewayv1beta1.ReferenceGrant,
	targetNamespace string,
	from gatewayv1beta1.ReferenceGrantFrom,
	to gatewayv1beta1.ReferenceGrantTo,
) bool {
	for _, grant := range grants {
		if grant.Namespace != targetNamespace {
			continue
		}
		if !referenceGrantFromAllowed(grant.Spec.From, from) {
			continue
		}
		if referenceGrantToAllowed(grant.Spec.To, to) {
			return true
		}
	}
	return false
}

func indexReferenceGrantsByNamespace(
	grants []gatewayv1beta1.ReferenceGrant,
) map[string][]gatewayv1beta1.ReferenceGrant {
	index := make(map[string][]gatewayv1beta1.ReferenceGrant)
	for _, grant := range grants {
		index[grant.Namespace] = append(index[grant.Namespace], grant)
	}
	return index
}

func referenceGrantFromAllowed(
	items []gatewayv1beta1.ReferenceGrantFrom,
	expected gatewayv1beta1.ReferenceGrantFrom,
) bool {
	for _, item := range items {
		if item.Group == expected.Group &&
			item.Kind == expected.Kind &&
			item.Namespace == expected.Namespace {
			return true
		}
	}
	return false
}

func referenceGrantToAllowed(
	items []gatewayv1beta1.ReferenceGrantTo,
	expected gatewayv1beta1.ReferenceGrantTo,
) bool {
	for _, item := range items {
		if item.Group != expected.Group || item.Kind != expected.Kind {
			continue
		}
		if item.Name == nil || expected.Name == nil || *item.Name == *expected.Name {
			return true
		}
	}
	return false
}

func objectNamePtr(name string) *gatewayv1beta1.ObjectName {
	item := gatewayv1beta1.ObjectName(name)
	return &item
}

func routeKindName(kind routeKind) string {
	return string(kind)
}

func routeGroupForKind(kind routeKind) string {
	switch kind {
	case routeKindHTTP, routeKindGRPC, routeKindTCP, routeKindUDP, routeKindTLS:
		return gatewayv1.GroupName
	default:
		return gatewayv1.GroupName
	}
}
