package chatbot

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
	"github.com/nantian-gw/gateway/internal/constants"
)

const (
	kindGateway         = "Gateway"
	kindHTTPRoute       = "HTTPRoute"
	kindGRPCRoute       = "GRPCRoute"
	kindTLSRoute        = "TLSRoute"
	kindTCPRoute        = "TCPRoute"
	kindUDPRoute        = "UDPRoute"
	kindService         = "Service"
	kindAIService       = "AIService"
	kindTokenPolicy     = "TokenPolicy"
	kindWasmPlugin      = "WasmPlugin"
	kindBackendLBPolicy = "BackendLBPolicy"
)

// ResourceRef identifies one cluster resource in the index.
type ResourceRef struct {
	Kind      string
	Namespace string
	Name      string
}

func (r ResourceRef) String() string {
	return fmt.Sprintf("%s %s/%s", r.Kind, r.Namespace, r.Name)
}

// IndexEntry is the lightweight index-layer record for one resource.
type IndexEntry struct {
	Ref           ResourceRef
	Summary       string // one-line spec digest
	StatusSummary string // condensed conditions
	Abnormal      bool   // status is not a healthy/ready state
	assoc         []ResourceRef
}

// ClusterIndex holds the lightweight index plus the source objects retained
// from the List pass, so renderContext can format details without re-fetching.
type ClusterIndex struct {
	Entries         []IndexEntry
	objects         map[ResourceRef]client.Object
	hasManagedClass bool
}

func newIndex() *ClusterIndex {
	return &ClusterIndex{objects: make(map[ResourceRef]client.Object)}
}

func (idx *ClusterIndex) add(entry IndexEntry, obj client.Object) {
	idx.Entries = append(idx.Entries, entry)
	idx.objects[entry.Ref] = obj
}

// collectIndex lists the live cluster and builds a lightweight index scoped to
// the managed GatewayClasses. Standard Gateway API types are filtered by a
// managed-class cascade (managed Gateways -> their attached Routes -> those
// Routes' backend Services). Nantian CRDs are always included when the scheme
// recognizes them. A single kind's List failure is tolerated (logged, skipped).
func collectIndex(ctx context.Context, cl client.Client, controllerName string, logger *slog.Logger) (ClusterIndex, error) {
	if logger == nil {
		logger = slog.Default()
	}
	idx := newIndex()

	var gcList gatewayv1.GatewayClassList
	if err := cl.List(ctx, &gcList); err != nil {
		return *idx, fmt.Errorf("collectIndex: list gatewayclasses: %w", err)
	}
	managed := make(map[string]bool)
	for i := range gcList.Items {
		if string(gcList.Items[i].Spec.ControllerName) == controllerName {
			managed[gcList.Items[i].Name] = true
		}
	}
	idx.hasManagedClass = len(managed) > 0
	if !idx.hasManagedClass {
		return *idx, nil
	}

	managedGW := collectGateways(ctx, cl, idx, managed, logger)
	keptSvc := make(map[types.NamespacedName]bool)
	collectHTTPRoutes(ctx, cl, idx, managedGW, keptSvc, logger)
	collectGRPCRoutes(ctx, cl, idx, managedGW, keptSvc, logger)
	collectL4Routes(ctx, cl, idx, managedGW, keptSvc, logger)
	collectServices(ctx, cl, idx, keptSvc, logger)

	collectAIServices(ctx, cl, idx, logger)
	collectTokenPolicies(ctx, cl, idx, logger)
	collectWasmPlugins(ctx, cl, idx, logger)
	collectBackendLBPolicies(ctx, cl, idx, logger)

	linkAssociations(idx)
	return *idx, nil
}

func collectGateways(ctx context.Context, cl client.Client, idx *ClusterIndex, managed map[string]bool, logger *slog.Logger) map[types.NamespacedName]bool {
	if logger == nil {
		logger = slog.Default()
	}
	managedGW := make(map[types.NamespacedName]bool)
	var list gatewayv1.GatewayList
	if err := cl.List(ctx, &list); err != nil {
		logger.Warn("chatbot rag: list gateways", "error", err)
		return managedGW
	}
	for i := range list.Items {
		gw := &list.Items[i]
		if !managed[string(gw.Spec.GatewayClassName)] {
			continue
		}
		managedGW[types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name}] = true
		status, abnormal := summarizeConditions(gw.Status.Conditions)
		idx.add(IndexEntry{
			Ref:           ResourceRef{Kind: kindGateway, Namespace: gw.Namespace, Name: gw.Name},
			Summary:       fmt.Sprintf("class=%s, listeners=%d", sanitizeUntrusted(string(gw.Spec.GatewayClassName)), len(gw.Spec.Listeners)),
			StatusSummary: status,
			Abnormal:      abnormal,
		}, gw.DeepCopy())
	}
	return managedGW
}

func parentGatewayRefs(routeNS string, refs []gatewayv1.ParentReference) []ResourceRef {
	out := make([]ResourceRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind != nil && string(*ref.Kind) != kindGateway {
			continue
		}
		ns := routeNS
		if ref.Namespace != nil {
			ns = string(*ref.Namespace)
		}
		out = append(out, ResourceRef{Kind: kindGateway, Namespace: ns, Name: string(ref.Name)})
	}
	return out
}

func HasAnyManagedParent(parents []ResourceRef, managedGW map[types.NamespacedName]bool) bool {
	for _, p := range parents {
		if managedGW[types.NamespacedName{Namespace: p.Namespace, Name: p.Name}] {
			return true
		}
	}
	return false
}

func backendServiceRef(routeNS string, ref gatewayv1.BackendObjectReference) (ResourceRef, bool) {
	if ref.Kind != nil && string(*ref.Kind) != kindService {
		return ResourceRef{}, false
	}
	ns := routeNS
	if ref.Namespace != nil {
		ns = string(*ref.Namespace)
	}
	return ResourceRef{Kind: kindService, Namespace: ns, Name: string(ref.Name)}, true
}

// addRoute applies the managed-parent filter and, when kept, records the route
// entry plus its backend Services into keptSvc for the cascade.
func (idx *ClusterIndex) addRoute(kind, ns, name string, parentRefs []gatewayv1.ParentReference, backendRefs []gatewayv1.BackendRef, parentStatus []gatewayv1.RouteParentStatus, ruleCount int, managedGW, keptSvc map[types.NamespacedName]bool, obj client.Object) {
	parents := parentGatewayRefs(ns, parentRefs)
	if !HasAnyManagedParent(parents, managedGW) {
		return
	}
	var backends []ResourceRef
	for _, br := range backendRefs {
		if svc, ok := backendServiceRef(ns, br.BackendObjectReference); ok {
			backends = append(backends, svc)
			keptSvc[types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}] = true
		}
	}
	status, abnormal := summarizeRouteParents(parentStatus)
	idx.add(IndexEntry{
		Ref:           ResourceRef{Kind: kind, Namespace: ns, Name: name},
		Summary:       fmt.Sprintf("parents=%d, rules=%d", len(parentRefs), ruleCount),
		StatusSummary: status,
		Abnormal:      abnormal,
		assoc:         append(append([]ResourceRef{}, parents...), backends...),
	}, obj)
}

// routeInfo holds the per-item data extracted from a route object for indexing.
type routeInfo struct {
	namespace    string
	name         string
	parentRefs   []gatewayv1.ParentReference
	backendRefs  []gatewayv1.BackendRef
	parentStatus []gatewayv1.RouteParentStatus
	ruleCount    int
	obj          client.Object
}

// collectRoutes is a generic helper that lists a route type, iterates its items,
// and indexes each through the addRoute method. The extract callback converts
// each list item (accessed via reflection from the client.ObjectList) into the
// routeInfo struct needed by addRoute.
func collectRoutes(
	ctx context.Context, cl client.Client, idx *ClusterIndex, kind string,
	managedGW, keptSvc map[types.NamespacedName]bool, logger *slog.Logger,
	list client.ObjectList,
	extract func(client.Object) routeInfo,
) {
	if logger == nil {
		logger = slog.Default()
	}
	if err := cl.List(ctx, list); err != nil {
		logger.Warn("chatbot rag: list "+kind, "error", err)
		return
	}
	items := reflect.ValueOf(list).Elem().FieldByName("Items")
	for i := 0; i < items.Len(); i++ {
		ri := extract(items.Index(i).Addr().Interface().(client.Object))
		idx.addRoute(kind, ri.namespace, ri.name, ri.parentRefs, ri.backendRefs, ri.parentStatus, ri.ruleCount, managedGW, keptSvc, ri.obj)
	}
}

func collectHTTPRoutes(ctx context.Context, cl client.Client, idx *ClusterIndex, managedGW, keptSvc map[types.NamespacedName]bool, logger *slog.Logger) {
	collectRoutes(ctx, cl, idx, kindHTTPRoute, managedGW, keptSvc, logger, &gatewayv1.HTTPRouteList{},
		func(obj client.Object) routeInfo {
			r := obj.(*gatewayv1.HTTPRoute)
			var brs []gatewayv1.BackendRef
			for _, rule := range r.Spec.Rules {
				for _, hbr := range rule.BackendRefs {
					brs = append(brs, hbr.BackendRef)
				}
			}
			return routeInfo{r.Namespace, r.Name, r.Spec.ParentRefs, brs, r.Status.Parents, len(r.Spec.Rules), r.DeepCopy()}
		},
	)
}

func collectGRPCRoutes(ctx context.Context, cl client.Client, idx *ClusterIndex, managedGW, keptSvc map[types.NamespacedName]bool, logger *slog.Logger) {
	collectRoutes(ctx, cl, idx, kindGRPCRoute, managedGW, keptSvc, logger, &gatewayv1.GRPCRouteList{},
		func(obj client.Object) routeInfo {
			r := obj.(*gatewayv1.GRPCRoute)
			var brs []gatewayv1.BackendRef
			for _, rule := range r.Spec.Rules {
				for _, gbr := range rule.BackendRefs {
					brs = append(brs, gbr.BackendRef)
				}
			}
			return routeInfo{r.Namespace, r.Name, r.Spec.ParentRefs, brs, r.Status.Parents, len(r.Spec.Rules), r.DeepCopy()}
		},
	)
}

func collectL4Routes(ctx context.Context, cl client.Client, idx *ClusterIndex, managedGW, keptSvc map[types.NamespacedName]bool, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	var tlsList gatewayv1alpha2.TLSRouteList
	if err := cl.List(ctx, &tlsList); err != nil {
		logger.Warn("chatbot rag: list tlsroutes", "error", err)
	} else {
		for i := range tlsList.Items {
			r := &tlsList.Items[i]
			var brs []gatewayv1.BackendRef
			for _, rule := range r.Spec.Rules {
				brs = append(brs, rule.BackendRefs...)
			}
			idx.addRoute(kindTLSRoute, r.Namespace, r.Name, r.Spec.ParentRefs, brs, r.Status.Parents, len(r.Spec.Rules), managedGW, keptSvc, r.DeepCopy())
		}
	}

	var tcpList gatewayv1alpha2.TCPRouteList
	if err := cl.List(ctx, &tcpList); err != nil {
		logger.Warn("chatbot rag: list tcproutes", "error", err)
	} else {
		for i := range tcpList.Items {
			r := &tcpList.Items[i]
			var brs []gatewayv1.BackendRef
			for _, rule := range r.Spec.Rules {
				brs = append(brs, rule.BackendRefs...)
			}
			idx.addRoute(kindTCPRoute, r.Namespace, r.Name, r.Spec.ParentRefs, brs, r.Status.Parents, len(r.Spec.Rules), managedGW, keptSvc, r.DeepCopy())
		}
	}

	var udpList gatewayv1alpha2.UDPRouteList
	if err := cl.List(ctx, &udpList); err != nil {
		logger.Warn("chatbot rag: list udproutes", "error", err)
	} else {
		for i := range udpList.Items {
			r := &udpList.Items[i]
			var brs []gatewayv1.BackendRef
			for _, rule := range r.Spec.Rules {
				brs = append(brs, rule.BackendRefs...)
			}
			idx.addRoute(kindUDPRoute, r.Namespace, r.Name, r.Spec.ParentRefs, brs, r.Status.Parents, len(r.Spec.Rules), managedGW, keptSvc, r.DeepCopy())
		}
	}
}

func collectServices(ctx context.Context, cl client.Client, idx *ClusterIndex, keptSvc map[types.NamespacedName]bool, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	var list corev1.ServiceList
	if err := cl.List(ctx, &list); err != nil {
		logger.Warn("chatbot rag: list services", "error", err)
		return
	}
	for i := range list.Items {
		svc := &list.Items[i]
		if !keptSvc[types.NamespacedName{Namespace: svc.Namespace, Name: svc.Name}] {
			continue
		}
		ports := make([]string, 0, len(svc.Spec.Ports))
		for _, p := range svc.Spec.Ports {
			ports = append(ports, strconv.Itoa(int(p.Port)))
		}
		idx.add(IndexEntry{
			Ref:     ResourceRef{Kind: kindService, Namespace: svc.Namespace, Name: svc.Name},
			Summary: fmt.Sprintf("type=%s, ports=[%s]", svc.Spec.Type, strings.Join(ports, ",")),
		}, svc.DeepCopy())
	}
}

func recognizes(cl client.Client, gv schema.GroupVersion, kind string) bool {
	return cl.Scheme().Recognizes(gv.WithKind(kind))
}

func collectAIServices(ctx context.Context, cl client.Client, idx *ClusterIndex, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	if !recognizes(cl, aiservice.GroupVersion, "AIService") {
		return
	}
	var list aiservice.AIServiceList
	if err := cl.List(ctx, &list); err != nil {
		logger.Warn("chatbot rag: list aiservices", "error", err)
		return
	}
	for i := range list.Items {
		s := &list.Items[i]
		status, abnormal := summarizeConditions(s.Status.Conditions)
		idx.add(IndexEntry{
			Ref:           ResourceRef{Kind: kindAIService, Namespace: s.Namespace, Name: s.Name},
			Summary:       fmt.Sprintf("provider=%s, model=%s, endpoint=%s", sanitizeUntrusted(s.Spec.Provider), sanitizeUntrusted(s.Spec.Model), sanitizeUntrusted(s.Spec.Endpoint)),
			StatusSummary: status,
			Abnormal:      abnormal,
		}, s.DeepCopy())
	}
}

func collectTokenPolicies(ctx context.Context, cl client.Client, idx *ClusterIndex, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	if !recognizes(cl, tokenpolicy.GroupVersion, "TokenPolicy") {
		return
	}
	var list tokenpolicy.TokenPolicyList
	if err := cl.List(ctx, &list); err != nil {
		logger.Warn("chatbot rag: list tokenpolicies", "error", err)
		return
	}
	for i := range list.Items {
		p := &list.Items[i]
		status, abnormal := summarizeConditions(p.Status.Conditions)
		idx.add(IndexEntry{
			Ref:           ResourceRef{Kind: kindTokenPolicy, Namespace: p.Namespace, Name: p.Name},
			Summary:       fmt.Sprintf("targetRefs=%d, rpm=%d", len(p.Spec.TargetRefs), p.Spec.RequestsPerMinute),
			StatusSummary: status,
			Abnormal:      abnormal,
		}, p.DeepCopy())
	}
}

func collectWasmPlugins(ctx context.Context, cl client.Client, idx *ClusterIndex, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	if !recognizes(cl, wasmplugin.GroupVersion, "WasmPlugin") {
		return
	}
	var list wasmplugin.WasmPluginList
	if err := cl.List(ctx, &list); err != nil {
		logger.Warn("chatbot rag: list wasmplugins", "error", err)
		return
	}
	for i := range list.Items {
		p := &list.Items[i]
		status, abnormal := summarizeConditions(p.Status.Conditions)
		idx.add(IndexEntry{
			Ref:           ResourceRef{Kind: kindWasmPlugin, Namespace: p.Namespace, Name: p.Name},
			Summary:       fmt.Sprintf("hooks=%d, targets=%d", len(p.Spec.Hooks), len(p.Spec.TargetRefs)),
			StatusSummary: status,
			Abnormal:      abnormal,
		}, p.DeepCopy())
	}
}

func collectBackendLBPolicies(ctx context.Context, cl client.Client, idx *ClusterIndex, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	if !recognizes(cl, backend.GroupVersion, "BackendLBPolicy") {
		return
	}
	var list backend.BackendLBPolicyList
	if err := cl.List(ctx, &list); err != nil {
		logger.Warn("chatbot rag: list backendlbpolicies", "error", err)
		return
	}
	for i := range list.Items {
		p := &list.Items[i]
		status, abnormal := summarizeAncestors(p.Status.Ancestors)
		algo := constants.StrDefault
		if p.Spec.LoadBalancing != nil && p.Spec.LoadBalancing.Type != nil {
			algo = string(*p.Spec.LoadBalancing.Type)
		}
		idx.add(IndexEntry{
			Ref:           ResourceRef{Kind: kindBackendLBPolicy, Namespace: p.Namespace, Name: p.Name},
			Summary:       fmt.Sprintf("targetRefs=%d, lb=%s", len(p.Spec.TargetRefs), algo),
			StatusSummary: status,
			Abnormal:      abnormal,
		}, p.DeepCopy())
	}
}

func containsRef(refs []ResourceRef, want ResourceRef) bool {
	for _, r := range refs {
		if r == want {
			return true
		}
	}
	return false
}

// linkAssociations adds reverse one-hop edges so that selecting a Gateway pulls
// in its attached Routes (and a Service pulls in the Routes that back it), not
// just the forward Route -> parent/backend direction recorded at collection.
func linkAssociations(idx *ClusterIndex) {
	pos := make(map[ResourceRef]int, len(idx.Entries))
	for i := range idx.Entries {
		pos[idx.Entries[i].Ref] = i
	}
	for i := range idx.Entries {
		src := idx.Entries[i].Ref
		for _, dst := range idx.Entries[i].assoc {
			j, ok := pos[dst]
			if !ok {
				continue
			}
			if !containsRef(idx.Entries[j].assoc, src) {
				idx.Entries[j].assoc = append(idx.Entries[j].assoc, src)
			}
		}
	}
}
