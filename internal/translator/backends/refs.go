package backends

import (
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/nantian-gw/gateway/internal/extfilter"
	"github.com/nantian-gw/gateway/internal/gatewayapi"
	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/mesh"
	"github.com/nantian-gw/gateway/internal/translator/shared"
)

const (
	BackendRefMetaValid  = "nantian.dev/backend-ref-valid"
	BackendRefMetaReason = "nantian.dev/backend-ref-reason"
)

// RouteKind identifies a route kind for backend ref validation.
type RouteKind string

const (
	RouteKindHTTP RouteKind = "HTTPRoute"
	RouteKindGRPC RouteKind = "GRPCRoute"
	RouteKindTCP  RouteKind = "TCPRoute"
	RouteKindUDP  RouteKind = "UDPRoute"
	RouteKindTLS  RouteKind = "TLSRoute"
)

// HTTPFilterFunc resolves HTTP route filters to IR filters.
type HTTPFilterFunc func(filters []gatewayv1.HTTPRouteFilter, defaultNamespace string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter

// GRPCFilterFunc resolves GRPC route filters to IR filters.
type GRPCFilterFunc func(filters []gatewayv1.GRPCRouteFilter, defaultNamespace string, resolver extfilter.Resolver, target extfilter.Target) []ir.Filter

type BackendRefTranslator struct {
	servicePorts               map[string]map[uint32]struct{}
	serviceImportPorts         map[string]map[uint32]struct{}
	referenceGrantsByNamespace map[string][]gatewayv1beta1.ReferenceGrant
	extensionResolver          extfilter.Resolver
	httpFilter                 HTTPFilterFunc
	grpcFilter                 GRPCFilterFunc
}

func NewBackendRefTranslator(
	services []corev1.Service,
	serviceImports []mcsv1alpha1.ServiceImport,
	referenceGrants []gatewayv1beta1.ReferenceGrant,
	extensionResolver extfilter.Resolver,
	httpFilter HTTPFilterFunc,
	grpcFilter GRPCFilterFunc,
) BackendRefTranslator {
	servicePorts := make(map[string]map[uint32]struct{}, len(services))
	for _, service := range services {
		key := shared.BackendObjectKey(service.Namespace, service.Name)
		ports := make(map[uint32]struct{}, len(service.Spec.Ports))
		for _, port := range service.Spec.Ports {
			ports[uint32(port.Port)] = struct{}{} //nolint:gosec
		}
		servicePorts[key] = ports
	}

	serviceImportPorts := make(map[string]map[uint32]struct{}, len(serviceImports))
	for _, serviceImport := range serviceImports {
		key := shared.BackendObjectKey(serviceImport.Namespace, serviceImport.Name)
		ports := make(map[uint32]struct{}, len(serviceImport.Spec.Ports))
		for _, port := range serviceImport.Spec.Ports {
			ports[uint32(port.Port)] = struct{}{} //nolint:gosec
		}
		serviceImportPorts[key] = ports
	}

	return BackendRefTranslator{
		servicePorts:               servicePorts,
		serviceImportPorts:         serviceImportPorts,
		referenceGrantsByNamespace: indexReferenceGrantsByNamespace(referenceGrants),
		extensionResolver:          extensionResolver,
		httpFilter:                 httpFilter,
		grpcFilter:                 grpcFilter,
	}
}

func (t BackendRefTranslator) AnnotateHTTPRoute(target *ir.HTTPRoute, source gatewayv1.HTTPRoute) {
	allowCrossNamespaceRefs := RouteUsesOnlyServiceParents(target.ParentRefs)
	validation := gatewayapi.ValidateHTTPRouteRules(source)
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
			RouteKindHTTP,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
		targetRuleIndex++
	}
}

func (t BackendRefTranslator) AnnotateGRPCRoute(target *ir.GRPCRoute, source gatewayv1.GRPCRoute) {
	allowCrossNamespaceRefs := RouteUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.grpcBackendRefs(
			source.Namespace,
			RouteKindGRPC,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t BackendRefTranslator) AnnotateTCPRoute(target *ir.StreamRoute, source gatewayv1alpha2.TCPRoute) {
	allowCrossNamespaceRefs := RouteUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.routeBackendRefs(
			source.Namespace,
			RouteKindTCP,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t BackendRefTranslator) AnnotateUDPRoute(target *ir.StreamRoute, source gatewayv1alpha2.UDPRoute) {
	allowCrossNamespaceRefs := RouteUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.routeBackendRefs(
			source.Namespace,
			RouteKindUDP,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t BackendRefTranslator) AnnotateTLSRoute(target *ir.StreamRoute, source gatewayv1alpha2.TLSRoute) {
	allowCrossNamespaceRefs := RouteUsesOnlyServiceParents(target.ParentRefs)
	for index, rule := range source.Spec.Rules {
		target.Rules[index].BackendRefs = t.routeBackendRefs(
			source.Namespace,
			RouteKindTLS,
			allowCrossNamespaceRefs,
			rule.BackendRefs,
		)
	}
}

func (t BackendRefTranslator) httpBackendRefs(
	routeNamespace string,
	routeKind RouteKind,
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
				Group:     shared.StringValue(ref.Group),
				Kind:      shared.StringValue(ref.Kind),
				Namespace: shared.NamespaceOrDefault(ref.Namespace, routeNamespace),
				Name:      string(ref.Name),
				Port:      shared.PortValue(ref.Port),
				Weight:    uint32(shared.WeightValue(ref.Weight)), //nolint:gosec
				Filters: t.httpFilter(
					ref.Filters,
					routeNamespace,
					t.extensionResolver,
					extfilter.TargetHTTP,
				),
			},
		))
	}
	return out
}

func (t BackendRefTranslator) grpcBackendRefs(
	routeNamespace string,
	routeKind RouteKind,
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
				Group:     shared.StringValue(ref.Group),
				Kind:      shared.StringValue(ref.Kind),
				Namespace: shared.NamespaceOrDefault(ref.Namespace, routeNamespace),
				Name:      string(ref.Name),
				Port:      shared.PortValue(ref.Port),
				Weight:    uint32(shared.WeightValue(ref.Weight)), //nolint:gosec
				Filters: t.grpcFilter(
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

func (t BackendRefTranslator) routeBackendRefs(
	routeNamespace string,
	routeKind RouteKind,
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
				Group:     shared.StringValue(ref.Group),
				Kind:      shared.StringValue(ref.Kind),
				Namespace: shared.NamespaceOrDefault(ref.Namespace, routeNamespace),
				Name:      string(ref.Name),
				Port:      shared.PortValue(ref.Port),
				Weight:    uint32(shared.WeightValue(ref.Weight)), //nolint:gosec
			},
		))
	}
	return out
}

func (t BackendRefTranslator) annotateBackendRef(
	routeNamespace string,
	routeKind RouteKind,
	allowCrossNamespaceRefs bool,
	ref ir.BackendRef,
) ir.BackendRef {
	if metadata := t.BackendRefMetadata(routeNamespace, routeKind, allowCrossNamespaceRefs, ref); len(metadata) > 0 {
		ref.Metadata = metadata
	}
	return ref
}

func (t BackendRefTranslator) BackendRefMetadata(
	routeNamespace string,
	routeKind RouteKind,
	allowCrossNamespaceRefs bool,
	ref ir.BackendRef,
) map[string]string {
	targetGroup := ref.Group
	if targetGroup != "" && targetGroup != mcsv1alpha1.GroupName {
		return invalidBackendRefMetadata(string(gatewayv1.RouteReasonInvalidKind))
	}

	targetKind, ok := BackendKindForRef(ref.Group, ref.Kind)
	if !ok {
		return invalidBackendRefMetadata(string(gatewayv1.RouteReasonInvalidKind))
	}

	if ref.Namespace != routeNamespace && !allowCrossNamespaceRefs && !ReferenceGranted(
		t.referenceGrantsByNamespace[ref.Namespace],
		ref.Namespace,
		gatewayv1beta1.ReferenceGrantFrom{
			Group:     gatewayv1beta1.Group(routeGroupForKind()),
			Kind:      gatewayv1beta1.Kind(routeKindName(routeKind)),
			Namespace: gatewayv1beta1.Namespace(routeNamespace),
		},
		gatewayv1beta1.ReferenceGrantTo{
			Group: gatewayv1beta1.Group(targetGroup),
			Kind:  gatewayv1beta1.Kind(targetKind),
			Name:  ObjectNamePtr(ref.Name),
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

func (t BackendRefTranslator) serviceExists(namespace string, name string, port uint32) bool {
	ports, ok := t.servicePorts[shared.BackendObjectKey(namespace, name)]
	if !ok {
		return false
	}
	if port == 0 {
		return true
	}
	_, ok = ports[port]
	return ok
}

func (t BackendRefTranslator) serviceImportExists(namespace string, name string, port uint32) bool {
	ports, ok := t.serviceImportPorts[shared.BackendObjectKey(namespace, name)]
	if !ok {
		return false
	}
	if port == 0 {
		return true
	}
	_, ok = ports[port]
	return ok
}

func BackendKindForRef(group string, kind string) (string, bool) {
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
		BackendRefMetaValid:  "false",
		BackendRefMetaReason: reason,
	}
}

func ReferenceGranted(
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

func ObjectNamePtr(name string) *gatewayv1beta1.ObjectName {
	item := gatewayv1beta1.ObjectName(name)
	return &item
}

func routeKindName(kind RouteKind) string {
	return string(kind)
}

func routeGroupForKind() string {
	return gatewayv1.GroupName
}

// RouteUsesOnlyServiceParents reports whether every parent ref targets a
// Service (used to determine if cross-namespace backend refs are allowed).
func RouteUsesOnlyServiceParents(parentRefs []ir.ParentRef) bool {
	if len(parentRefs) == 0 {
		return false
	}
	for _, parentRef := range parentRefs {
		if !IsServiceParentRef(parentRef) {
			return false
		}
	}
	return true
}

// IsServiceParentRef reports whether the parent ref targets a Kubernetes Service.
func IsServiceParentRef(parentRef ir.ParentRef) bool {
	return parentRef.Group == "" && parentRef.Kind == mesh.FrontendKindService
}
