package status

import (
	"net"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
	mcsv1alpha1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/extensionfilter"
	backendlbv1alpha2 "github.com/aether-gateway/aether-gateway/controlplane/internal/gatewayapiexperimental/backendlbv1alpha2"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/infrastructure"
)

const gatewayGroup = gatewayv1.GroupName

type routeKind string

const (
	routeKindHTTP routeKind = "HTTPRoute"
	routeKindGRPC routeKind = "GRPCRoute"
	routeKindTCP  routeKind = "TCPRoute"
	routeKindUDP  routeKind = "UDPRoute"
	routeKindTLS  routeKind = "TLSRoute"
)

type clusterState struct {
	controllerName     string
	statusAddresses    []string
	gatewayClasses     []gatewayv1.GatewayClass
	gateways           []gatewayv1.Gateway
	managedGateways    []gatewayv1.Gateway
	httpRoutes         []gatewayv1.HTTPRoute
	grpcRoutes         []gatewayv1.GRPCRoute
	tcpRoutes          []gatewayv1alpha2.TCPRoute
	udpRoutes          []gatewayv1alpha2.UDPRoute
	tlsRoutes          []gatewayv1alpha2.TLSRoute
	backendLBPolicies  []backendlbv1alpha2.BackendLBPolicy
	backendTLSPolicies []gatewayv1alpha3.BackendTLSPolicy
	listenerSets       []gatewayv1.ListenerSet
	listenerSetByKey   map[string]gatewayv1.ListenerSet
	services           []corev1.Service
	endpointSlices     []discoveryv1.EndpointSlice
	serviceImports     []mcsv1alpha1.ServiceImport
	namespaces         []corev1.Namespace
	secrets            []corev1.Secret
	configMaps         []corev1.ConfigMap
	referenceGrants    []gatewayv1beta1.ReferenceGrant

	managedGatewayClasses   map[string]gatewayv1.GatewayClass
	managedGatewayByKey     map[string]gatewayv1.Gateway
	serviceByKey            map[string]corev1.Service
	endpointSlicesByService map[string][]discoveryv1.EndpointSlice
	serviceImportByKey      map[string]mcsv1alpha1.ServiceImport
	namespaceByName         map[string]corev1.Namespace
	secretByKey             map[string]corev1.Secret
	configMapByKey          map[string]corev1.ConfigMap
}

type conditionSpec struct {
	Type               string
	Status             metav1.ConditionStatus
	Reason             string
	Message            string
	ObservedGeneration int64
}

type backendInput struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
	Port      uint16
}

type routeInput struct {
	kind                         routeKind
	namespace                    string
	name                         string
	generation                   int64
	hostnames                    []gatewayv1.Hostname
	parentRefs                   []gatewayv1.ParentReference
	defaultGatewayScope          gatewayv1.GatewayDefaultScope
	backends                     []backendInput
	extensionRefs                []extensionfilter.Ref
	acceptedErrorMessage         string
	resolvedRefsErrorMessage     string
	partiallyInvalidErrorMessage string
}

type routeParentEvaluation struct {
	parentRef         gatewayv1.ParentReference
	controllerName    gatewayv1.GatewayController
	acceptedCondition conditionSpec
	resolvedCondition conditionSpec
	extraConditions   []conditionSpec
	matchedListeners  []listenerKey
}

type routeResolutionEvaluation struct {
	resolvedCondition conditionSpec
	extraConditions   []conditionSpec
}

type listenerKey struct {
	gatewayNamespace string
	gatewayName      string
	listenerName     gatewayv1.SectionName
}

type gatewayEvaluation struct {
	sourceGeneration     int64
	addresses            []gatewayv1.GatewayStatusAddress
	acceptedCondition    conditionSpec
	programmedCondition  conditionSpec
	extraConditions      []conditionSpec
	listeners            []listenerEvaluation
	infraValidation      infrastructure.GatewayInfrastructureParameterValidation
	convergence          gatewayConvergenceObservation
	translationReady     bool
	infraConverged       bool
	attachedListenerSets int32
}

type listenerEvaluation struct {
	name                gatewayv1.SectionName
	supportedKinds      []gatewayv1.RouteGroupKind
	attachedRoutes      int32
	acceptedCondition   conditionSpec
	resolvedCondition   conditionSpec
	programmedCondition conditionSpec
	extraConditions     []conditionSpec
}

type routeState struct {
	http        map[client.ObjectKey][]routeParentEvaluation
	grpc        map[client.ObjectKey][]routeParentEvaluation
	tcp         map[client.ObjectKey][]routeParentEvaluation
	udp         map[client.ObjectKey][]routeParentEvaluation
	tls         map[client.ObjectKey][]routeParentEvaluation
	attachments map[listenerKey]map[string]struct{}
}

type listenerPolicy struct {
	supportedKinds  []gatewayv1.RouteGroupKind
	allowedKinds    map[routeKind]struct{}
	invalidKindRefs bool
	namespaceMode   gatewayv1.FromNamespaces
	selector        labels.Selector
}

func (s *clusterState) index() {
	s.managedGatewayClasses = make(map[string]gatewayv1.GatewayClass)
	for _, gatewayClass := range s.gatewayClasses {
		if string(gatewayClass.Spec.ControllerName) == s.controllerName {
			s.managedGatewayClasses[gatewayClass.Name] = gatewayClass
		}
	}

	s.managedGateways = s.managedGateways[:0]
	s.managedGatewayByKey = make(map[string]gatewayv1.Gateway)
	for _, gateway := range s.gateways {
		if len(s.managedGatewayClasses) > 0 {
			if _, ok := s.managedGatewayClasses[string(gateway.Spec.GatewayClassName)]; !ok {
				continue
			}
		}
		if len(s.managedGatewayClasses) == 0 && gateway.Spec.GatewayClassName == "" {
			continue
		}
		s.managedGateways = append(s.managedGateways, gateway)
		s.managedGatewayByKey[namespacedName(gateway.Namespace, gateway.Name)] = gateway
	}

	s.serviceByKey = make(map[string]corev1.Service)
	for _, service := range s.services {
		s.serviceByKey[namespacedName(service.Namespace, service.Name)] = service
	}

	s.endpointSlicesByService = make(map[string][]discoveryv1.EndpointSlice)
	for _, endpointSlice := range s.endpointSlices {
		serviceName := endpointSlice.Labels[discoveryv1.LabelServiceName]
		if serviceName == "" {
			continue
		}

		key := namespacedName(endpointSlice.Namespace, serviceName)
		s.endpointSlicesByService[key] = append(s.endpointSlicesByService[key], endpointSlice)
	}

	s.serviceImportByKey = make(map[string]mcsv1alpha1.ServiceImport)
	for _, serviceImport := range s.serviceImports {
		s.serviceImportByKey[namespacedName(serviceImport.Namespace, serviceImport.Name)] = serviceImport
	}

	s.namespaceByName = make(map[string]corev1.Namespace)
	for _, namespace := range s.namespaces {
		s.namespaceByName[namespace.Name] = namespace
	}

	s.secretByKey = make(map[string]corev1.Secret)
	for _, secret := range s.secrets {
		s.secretByKey[namespacedName(secret.Namespace, secret.Name)] = secret
	}

	s.configMapByKey = make(map[string]corev1.ConfigMap)
	for _, configMap := range s.configMaps {
		s.configMapByKey[namespacedName(configMap.Namespace, configMap.Name)] = configMap
	}

	s.listenerSetByKey = make(map[string]gatewayv1.ListenerSet)
	for _, ls := range s.listenerSets {
		s.listenerSetByKey[namespacedName(ls.Namespace, ls.Name)] = ls
	}
}

func setCondition(conditions *[]metav1.Condition, spec conditionSpec) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:               spec.Type,
		Status:             spec.Status,
		Reason:             spec.Reason,
		Message:            spec.Message,
		ObservedGeneration: spec.ObservedGeneration,
	})
}

func removeCondition(conditions *[]metav1.Condition, targetType string) {
	if len(*conditions) == 0 {
		return
	}

	filtered := (*conditions)[:0]
	for _, condition := range *conditions {
		if condition.Type == targetType {
			continue
		}
		filtered = append(filtered, condition)
	}
	*conditions = filtered
}

func mergeListenerStatuses(existing []gatewayv1.ListenerStatus, evals []listenerEvaluation) []gatewayv1.ListenerStatus {
	index := make(map[gatewayv1.SectionName]gatewayv1.ListenerStatus, len(existing))
	for _, item := range existing {
		index[item.Name] = item
	}

	out := make([]gatewayv1.ListenerStatus, 0, len(evals))
	for _, eval := range evals {
		item := index[eval.name]
		item.Conditions = append([]metav1.Condition(nil), item.Conditions...)
		item.Name = eval.name
		item.SupportedKinds = eval.supportedKinds
		item.AttachedRoutes = eval.attachedRoutes
		setCondition(&item.Conditions, eval.acceptedCondition)
		setCondition(&item.Conditions, eval.resolvedCondition)
		setCondition(&item.Conditions, eval.programmedCondition)
		if len(eval.extraConditions) == 0 {
			removeCondition(&item.Conditions, string(gatewayv1.ListenerConditionConflicted))
		}
		for _, extra := range eval.extraConditions {
			setCondition(&item.Conditions, extra)
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func mergeRouteParents(existing []gatewayv1.RouteParentStatus, evals []routeParentEvaluation) []gatewayv1.RouteParentStatus {
	index := make(map[string]gatewayv1.RouteParentStatus, len(existing))
	for _, item := range existing {
		index[parentStatusKey(item.ParentRef, item.ControllerName)] = item
	}

	out := make([]gatewayv1.RouteParentStatus, 0, len(evals))
	for _, eval := range evals {
		key := parentStatusKey(eval.parentRef, eval.controllerName)
		item := index[key]
		item.Conditions = append([]metav1.Condition(nil), item.Conditions...)
		item.ParentRef = eval.parentRef
		item.ControllerName = eval.controllerName
		setCondition(&item.Conditions, eval.acceptedCondition)
		setCondition(&item.Conditions, eval.resolvedCondition)
		if len(eval.extraConditions) == 0 {
			removeCondition(&item.Conditions, string(gatewayv1.RouteConditionPartiallyInvalid))
		}
		for _, extra := range eval.extraConditions {
			setCondition(&item.Conditions, extra)
		}
		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool {
		return parentStatusKey(out[i].ParentRef, out[i].ControllerName) < parentStatusKey(out[j].ParentRef, out[j].ControllerName)
	})
	return out
}

func buildStatusAddresses(raw []string) []gatewayv1.GatewayStatusAddress {
	return canonicalGatewayPublishedAddresses(func(appendAddress func(string)) {
		for _, value := range raw {
			appendAddress(value)
		}
	})
}

func canonicalGatewayPublishedAddresses(collect func(func(string))) []gatewayv1.GatewayStatusAddress {
	out := make([]gatewayv1.GatewayStatusAddress, 0)
	seen := make(map[string]struct{})

	appendAddress := func(raw string) {
		value := strings.TrimSpace(raw)
		if value == "" {
			return
		}

		addressType := gatewayv1.HostnameAddressType
		if ip := net.ParseIP(value); ip != nil {
			addressType = gatewayv1.IPAddressType
			value = ip.String()
		} else {
			value = normalizeHostnameValue(value)
		}

		key := gatewayStatusAddressKey(addressType, value)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}

		out = append(out, gatewayv1.GatewayStatusAddress{
			Type:  &addressType,
			Value: value,
		})
	}

	collect(appendAddress)
	sort.Slice(out, func(i, j int) bool {
		leftType := gatewayAddressType(out[i].Type, out[i].Value)
		rightType := gatewayAddressType(out[j].Type, out[j].Value)
		if leftType != rightType {
			if leftType == gatewayv1.IPAddressType {
				return true
			}
			if rightType == gatewayv1.IPAddressType {
				return false
			}
		}
		return gatewayStatusAddressKey(*out[i].Type, out[i].Value) < gatewayStatusAddressKey(*out[j].Type, out[j].Value)
	})
	return out
}

func namespacedName(namespace, name string) string {
	return namespace + "/" + name
}

func namespaceOrDefault(namespace *gatewayv1.Namespace, fallback string) string {
	if namespace == nil || *namespace == "" {
		return fallback
	}
	return string(*namespace)
}

func stringOrEmpty[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func parentStatusKey(parentRef gatewayv1.ParentReference, controllerName gatewayv1.GatewayController) string {
	var builder strings.Builder
	builder.WriteString(string(controllerName))
	builder.WriteByte('|')
	builder.WriteString(stringOrEmpty(parentRef.Group))
	builder.WriteByte('|')
	builder.WriteString(stringOrEmpty(parentRef.Kind))
	builder.WriteByte('|')
	builder.WriteString(string(parentRef.Name))
	builder.WriteByte('|')
	builder.WriteString(namespaceOrDefault(parentRef.Namespace, ""))
	builder.WriteByte('|')
	builder.WriteString(stringOrEmpty(parentRef.SectionName))
	builder.WriteByte('|')
	if parentRef.Port != nil {
		builder.WriteString(strconv.FormatInt(int64(*parentRef.Port), 10))
	}
	return builder.String()
}

func servicePortExists(service corev1.Service, port uint16) bool {
	if port == 0 {
		return true
	}

	for _, item := range service.Spec.Ports {
		if item.Port == int32(port) {
			return true
		}
	}
	return false
}
