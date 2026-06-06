package ir

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type Snapshot struct {
	ID           string           `json:"id"`
	GeneratedAt  time.Time        `json:"generatedAt"`
	Listeners    []Listener       `json:"listeners"`
	HTTPRoutes   []HTTPRoute      `json:"httpRoutes"`
	GRPCRoutes   []GRPCRoute      `json:"grpcRoutes"`
	StreamRoutes []StreamRoute    `json:"streamRoutes"`
	Backends     []BackendCluster `json:"backends"`
	Secrets      []SecretMaterial `json:"secrets"`
	Workloads    []Workload       `json:"workloads,omitempty"`
}

type Listener struct {
	Name           string            `json:"name"`
	Address        string            `json:"address"`
	Addresses      []string          `json:"addresses,omitempty"`
	Port           uint32            `json:"port"`
	Protocol       string            `json:"protocol"`
	Hostnames      []string          `json:"hostnames,omitempty"`
	AttachedRoutes []string          `json:"attachedRoutes,omitempty"`
	TLS            *TLSConfig        `json:"tls,omitempty"`
	BackendTLS     *BackendTLSConfig `json:"backendTls,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Status         *ListenerStatus   `json:"status,omitempty"`
}

type TLSConfig struct {
	Enabled            bool                `json:"enabled"`
	Passthrough        bool                `json:"passthrough"`
	SecretRefs         []string            `json:"secretRefs,omitempty"`
	SNIHosts           []string            `json:"sniHosts,omitempty"`
	MinVersion         string              `json:"minVersion,omitempty"`
	MaxVersion         string              `json:"maxVersion,omitempty"`
	FrontendValidation *FrontendValidation `json:"frontendValidation,omitempty"`
}

type FrontendValidation struct {
	ClientCAPEMs []string `json:"clientCAPEMs,omitempty"`
	Mode         string   `json:"mode,omitempty"`
}

type BackendTLSConfig struct {
	ClientCertificateRef string `json:"clientCertificateRef,omitempty"`
}

type BackendTLSValidation struct {
	Hostname        string               `json:"hostname,omitempty"`
	UseSystemCAs    bool                 `json:"useSystemCAs,omitempty"`
	CAPEMs          []string             `json:"caPEMs,omitempty"`
	SubjectAltNames []BackendSubjectName `json:"subjectAltNames,omitempty"`
	MinVersion      string               `json:"minVersion,omitempty"`
	MaxVersion      string               `json:"maxVersion,omitempty"`
}

type BackendSubjectName struct {
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

type HTTPRoute struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Hostnames   []string          `json:"hostnames,omitempty"`
	ParentRefs  []ParentRef       `json:"parentRefs,omitempty"`
	Rules       []HTTPRule        `json:"rules,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Status      *RouteStatus      `json:"status,omitempty"`
}

type HTTPRule struct {
	Name               string                    `json:"name,omitempty"`
	Matches            []HTTPMatch               `json:"matches,omitempty"`
	Filters            []Filter                  `json:"filters,omitempty"`
	BackendRefs        []BackendRef              `json:"backendRefs,omitempty"`
	Timeouts           *RouteTimeouts            `json:"timeouts,omitempty"`
	Retry              *RetryPolicy              `json:"retry,omitempty"`
	SessionPersistence *SessionPersistencePolicy `json:"sessionPersistence,omitempty"`
}

type HTTPMatch struct {
	Path        string        `json:"path,omitempty"`
	PathType    string        `json:"pathType,omitempty"`
	Method      string        `json:"method,omitempty"`
	Headers     []HeaderMatch `json:"headers,omitempty"`
	QueryParams []QueryMatch  `json:"queryParams,omitempty"`
}

type GRPCRoute struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Hostnames   []string          `json:"hostnames,omitempty"`
	ParentRefs  []ParentRef       `json:"parentRefs,omitempty"`
	Rules       []GRPCRule        `json:"rules,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Status      *RouteStatus      `json:"status,omitempty"`
}

type GRPCRule struct {
	Name               string                    `json:"name,omitempty"`
	Matches            []GRPCMatch               `json:"matches,omitempty"`
	Filters            []Filter                  `json:"filters,omitempty"`
	BackendRefs        []BackendRef              `json:"backendRefs,omitempty"`
	SessionPersistence *SessionPersistencePolicy `json:"sessionPersistence,omitempty"`
}

type GRPCMatch struct {
	Service   string        `json:"service,omitempty"`
	Method    string        `json:"method,omitempty"`
	MatchType string        `json:"matchType,omitempty"`
	Headers   []HeaderMatch `json:"headers,omitempty"`
}

type StreamRoute struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Kind        string            `json:"kind"`
	ParentRefs  []ParentRef       `json:"parentRefs,omitempty"`
	Rules       []StreamRule      `json:"rules,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Status      *RouteStatus      `json:"status,omitempty"`
}

type ConditionStatus struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	ObservedGeneration int64     `json:"observedGeneration,omitempty"`
	LastTransitionTime time.Time `json:"lastTransitionTime,omitempty"`
}

type ListenerStatus struct {
	AttachedRoutes int               `json:"attachedRoutes,omitempty"`
	Conditions     []ConditionStatus `json:"conditions,omitempty"`
	Accepted       *ConditionStatus  `json:"accepted,omitempty"`
	Programmed     *ConditionStatus  `json:"programmed,omitempty"`
	ResolvedRefs   *ConditionStatus  `json:"resolvedRefs,omitempty"`
}

type RouteParentStatus struct {
	ControllerName string            `json:"controllerName,omitempty"`
	ParentRef      ParentRef         `json:"parentRef,omitempty"`
	Conditions     []ConditionStatus `json:"conditions,omitempty"`
	Accepted       *ConditionStatus  `json:"accepted,omitempty"`
	ResolvedRefs   *ConditionStatus  `json:"resolvedRefs,omitempty"`
}

type RouteStatus struct {
	Parents []RouteParentStatus `json:"parents,omitempty"`
}

type StreamRule struct {
	Name        string        `json:"name,omitempty"`
	Matches     []StreamMatch `json:"matches,omitempty"`
	BackendRefs []BackendRef  `json:"backendRefs,omitempty"`
}

type TlsRouteMode string

const (
	TlsRouteModePassthrough TlsRouteMode = "Passthrough"
	TlsRouteModeTerminate   TlsRouteMode = "Terminate"
)

type StreamMatch struct {
	Port        uint32       `json:"port,omitempty"`
	SNIHostname string       `json:"sniHostname,omitempty"`
	Mode        TlsRouteMode `json:"mode,omitempty"`
}

type ParentRef struct {
	Group       string `json:"group,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name,omitempty"`
	SectionName string `json:"sectionName,omitempty"`
	Port        uint32 `json:"port,omitempty"`
}

type BackendRef struct {
	Group     string            `json:"group,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Namespace string            `json:"namespace,omitempty"`
	Name      string            `json:"name,omitempty"`
	Port      uint32            `json:"port,omitempty"`
	Weight    uint32            `json:"weight,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Filters   []Filter          `json:"filters,omitempty"`
}

type BackendCluster struct {
	Name                 string                    `json:"name"`
	Namespace            string                    `json:"namespace"`
	Protocol             string                    `json:"protocol"`
	Endpoints            []BackendEndpoint         `json:"endpoints,omitempty"`
	ConnectTimeout       time.Duration             `json:"connectTimeout,omitempty"`
	RequestTimeout       time.Duration             `json:"requestTimeout,omitempty"`
	BackendTLSValidation *BackendTLSValidation     `json:"backendTlsValidation,omitempty"`
	SessionPersistence   *SessionPersistencePolicy `json:"sessionPersistence,omitempty"`
	LoadBalancing        *LoadBalancingPolicy      `json:"loadBalancing,omitempty"`
	Metadata             map[string]string         `json:"metadata,omitempty"`
	AIService            *AIServiceConfig           `json:"aiService,omitempty"`
	TokenPolicy          *TokenPolicyConfig         `json:"tokenPolicy,omitempty"`
	WasmPlugin           *WasmPluginConfig          `json:"wasmPlugin,omitempty"`
}

type BackendEndpoint struct {
	Address string `json:"address"`
	Port    uint32 `json:"port"`
	Healthy bool   `json:"healthy"`
	Zone    string `json:"zone,omitempty"`
}

type HeaderMatch struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	MatchType string `json:"matchType,omitempty"`
}

type QueryMatch struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	MatchType string `json:"matchType,omitempty"`
}

type Filter struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

type RouteTimeouts struct {
	Request        *time.Duration `json:"request,omitempty"`
	BackendRequest *time.Duration `json:"backendRequest,omitempty"`
}

type RetryPolicy struct {
	Codes    []uint32       `json:"codes,omitempty"`
	Attempts uint32         `json:"attempts,omitempty"`
	Backoff  *time.Duration `json:"backoff,omitempty"`
}

type SessionPersistencePolicy struct {
	SessionName     string         `json:"sessionName,omitempty"`
	Type            string         `json:"type,omitempty"`
	AbsoluteTimeout *time.Duration `json:"absoluteTimeout,omitempty"`
	IdleTimeout     *time.Duration `json:"idleTimeout,omitempty"`
	Cookie          *CookieConfig  `json:"cookie,omitempty"`
}

type CookieConfig struct {
	LifetimeType string `json:"lifetimeType,omitempty"`
}

type LoadBalancingPolicy struct {
	Type           string                `json:"type,omitempty"`
	ConsistentHash *ConsistentHashPolicy `json:"consistentHash,omitempty"`
}

type ConsistentHashPolicy struct {
	KeyType    string `json:"keyType,omitempty"`
	HeaderName string `json:"headerName,omitempty"`
}

type SecretMaterial struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	CertPEM   string `json:"certPEM,omitempty"`
	KeyPEM    string `json:"keyPEM,omitempty"`
}

type Workload struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
}

func (s *Snapshot) Normalize() error {
	s.GeneratedAt = s.GeneratedAt.UTC()
	s.sort()

	digest, err := s.computeDigest()
	if err != nil {
		return err
	}
	s.ID = digest

	return nil
}

func (s *Snapshot) sort() {
	sort.Slice(s.Listeners, func(i, j int) bool {
		return compareListeners(s.Listeners[i], s.Listeners[j]) < 0
	})
	for idx := range s.Listeners {
		normalizeListener(&s.Listeners[idx])
	}

	sort.Slice(s.HTTPRoutes, func(i, j int) bool {
		return compareNamespacedResources(
			s.HTTPRoutes[i].Namespace,
			s.HTTPRoutes[i].Name,
			s.HTTPRoutes[j].Namespace,
			s.HTTPRoutes[j].Name,
		) < 0
	})
	for idx := range s.HTTPRoutes {
		normalizeHTTPRoute(&s.HTTPRoutes[idx])
	}

	sort.Slice(s.GRPCRoutes, func(i, j int) bool {
		return compareNamespacedResources(
			s.GRPCRoutes[i].Namespace,
			s.GRPCRoutes[i].Name,
			s.GRPCRoutes[j].Namespace,
			s.GRPCRoutes[j].Name,
		) < 0
	})
	for idx := range s.GRPCRoutes {
		normalizeGRPCRoute(&s.GRPCRoutes[idx])
	}

	sort.Slice(s.StreamRoutes, func(i, j int) bool {
		return compareStreamRoutes(s.StreamRoutes[i], s.StreamRoutes[j]) < 0
	})
	for idx := range s.StreamRoutes {
		normalizeStreamRoute(&s.StreamRoutes[idx])
	}

	sort.Slice(s.Backends, func(i, j int) bool {
		return compareBackends(s.Backends[i], s.Backends[j]) < 0
	})
	for idx := range s.Backends {
		normalizeBackend(&s.Backends[idx])
	}

	sort.Slice(s.Secrets, func(i, j int) bool {
		return compareNamespacedResources(
			s.Secrets[i].Namespace,
			s.Secrets[i].Name,
			s.Secrets[j].Namespace,
			s.Secrets[j].Name,
		) < 0
	})

	sort.Slice(s.Workloads, func(i, j int) bool {
		if value := compareNamespacedResources(
			s.Workloads[i].Namespace,
			s.Workloads[i].Name,
			s.Workloads[j].Namespace,
			s.Workloads[j].Name,
		); value != 0 {
			return value < 0
		}
		return s.Workloads[i].IP < s.Workloads[j].IP
	})
}

func normalizeListener(listener *Listener) {
	sort.Strings(listener.Addresses)
	sort.Strings(listener.Hostnames)
	sort.Strings(listener.AttachedRoutes)
	if listener.Status != nil {
		normalizeListenerStatus(listener.Status)
	}
}

func normalizeListenerStatus(status *ListenerStatus) {
	sort.Slice(status.Conditions, func(i, j int) bool {
		return compareConditions(status.Conditions[i], status.Conditions[j]) < 0
	})
}

func normalizeHTTPRoute(route *HTTPRoute) {
	sort.Strings(route.Hostnames)
	sort.Slice(route.ParentRefs, func(i, j int) bool {
		return compareParentRefs(route.ParentRefs[i], route.ParentRefs[j]) < 0
	})
	for idx := range route.Rules {
		normalizeHTTPRule(&route.Rules[idx])
	}
	if route.Status != nil {
		normalizeRouteStatus(route.Status)
	}
}

func normalizeHTTPRule(rule *HTTPRule) {
	for idx := range rule.Matches {
		normalizeHTTPMatch(&rule.Matches[idx])
	}
}

func normalizeHTTPMatch(match *HTTPMatch) {
	sort.Slice(match.Headers, func(i, j int) bool {
		return compareHeaderMatches(match.Headers[i], match.Headers[j]) < 0
	})
	sort.Slice(match.QueryParams, func(i, j int) bool {
		return compareQueryMatches(match.QueryParams[i], match.QueryParams[j]) < 0
	})
}

func normalizeGRPCRoute(route *GRPCRoute) {
	sort.Strings(route.Hostnames)
	sort.Slice(route.ParentRefs, func(i, j int) bool {
		return compareParentRefs(route.ParentRefs[i], route.ParentRefs[j]) < 0
	})
	for idx := range route.Rules {
		normalizeGRPCRule(&route.Rules[idx])
	}
	if route.Status != nil {
		normalizeRouteStatus(route.Status)
	}
}

func normalizeGRPCRule(rule *GRPCRule) {
	for idx := range rule.Matches {
		normalizeGRPCMatch(&rule.Matches[idx])
	}
}

func normalizeGRPCMatch(match *GRPCMatch) {
	sort.Slice(match.Headers, func(i, j int) bool {
		return compareHeaderMatches(match.Headers[i], match.Headers[j]) < 0
	})
}

func normalizeStreamRoute(route *StreamRoute) {
	sort.Slice(route.ParentRefs, func(i, j int) bool {
		return compareParentRefs(route.ParentRefs[i], route.ParentRefs[j]) < 0
	})
	if route.Status != nil {
		normalizeRouteStatus(route.Status)
	}
}

func normalizeRouteStatus(status *RouteStatus) {
	sort.Slice(status.Parents, func(i, j int) bool {
		return compareRouteParentStatuses(status.Parents[i], status.Parents[j]) < 0
	})
	for idx := range status.Parents {
		sort.Slice(status.Parents[idx].Conditions, func(i, j int) bool {
			return compareConditions(
				status.Parents[idx].Conditions[i],
				status.Parents[idx].Conditions[j],
			) < 0
		})
	}
}

func normalizeBackend(backend *BackendCluster) {
	sort.Slice(backend.Endpoints, func(i, j int) bool {
		return compareBackendEndpoints(backend.Endpoints[i], backend.Endpoints[j]) < 0
	})
}

func compareListeners(left Listener, right Listener) int {
	if value := compareStrings(left.Name, right.Name); value != 0 {
		return value
	}
	if value := compareStrings(left.Address, right.Address); value != 0 {
		return value
	}
	if left.Port != right.Port {
		if left.Port < right.Port {
			return -1
		}
		return 1
	}
	return compareStrings(left.Protocol, right.Protocol)
}

func compareBackends(left BackendCluster, right BackendCluster) int {
	if value := compareNamespacedResources(left.Namespace, left.Name, right.Namespace, right.Name); value != 0 {
		return value
	}
	return compareStrings(left.Protocol, right.Protocol)
}

func compareBackendEndpoints(left BackendEndpoint, right BackendEndpoint) int {
	if value := compareStrings(left.Address, right.Address); value != 0 {
		return value
	}
	if left.Port != right.Port {
		if left.Port < right.Port {
			return -1
		}
		return 1
	}
	if left.Healthy != right.Healthy {
		if !left.Healthy {
			return -1
		}
		return 1
	}
	return compareStrings(left.Zone, right.Zone)
}

func compareStreamRoutes(left StreamRoute, right StreamRoute) int {
	if value := compareNamespacedResources(left.Namespace, left.Name, right.Namespace, right.Name); value != 0 {
		return value
	}
	return compareStrings(left.Kind, right.Kind)
}

func compareRouteParentStatuses(left RouteParentStatus, right RouteParentStatus) int {
	if value := compareStrings(left.ControllerName, right.ControllerName); value != 0 {
		return value
	}
	return compareParentRefs(left.ParentRef, right.ParentRef)
}

func compareParentRefs(left ParentRef, right ParentRef) int {
	if value := compareStrings(left.Group, right.Group); value != 0 {
		return value
	}
	if value := compareStrings(left.Kind, right.Kind); value != 0 {
		return value
	}
	if value := compareStrings(left.Namespace, right.Namespace); value != 0 {
		return value
	}
	if value := compareStrings(left.Name, right.Name); value != 0 {
		return value
	}
	if value := compareStrings(left.SectionName, right.SectionName); value != 0 {
		return value
	}
	if left.Port != right.Port {
		if left.Port < right.Port {
			return -1
		}
		return 1
	}
	return 0
}

func compareConditions(left ConditionStatus, right ConditionStatus) int {
	if value := compareStrings(left.Type, right.Type); value != 0 {
		return value
	}
	if value := compareStrings(left.Status, right.Status); value != 0 {
		return value
	}
	if value := compareStrings(left.Reason, right.Reason); value != 0 {
		return value
	}
	if value := compareStrings(left.Message, right.Message); value != 0 {
		return value
	}
	if left.ObservedGeneration != right.ObservedGeneration {
		if left.ObservedGeneration < right.ObservedGeneration {
			return -1
		}
		return 1
	}
	leftTime := left.LastTransitionTime.UTC().UnixNano()
	rightTime := right.LastTransitionTime.UTC().UnixNano()
	if leftTime != rightTime {
		if leftTime < rightTime {
			return -1
		}
		return 1
	}
	return 0
}

func compareHeaderMatches(left HeaderMatch, right HeaderMatch) int {
	if value := compareStrings(left.Name, right.Name); value != 0 {
		return value
	}
	if value := compareStrings(left.MatchType, right.MatchType); value != 0 {
		return value
	}
	return compareStrings(left.Value, right.Value)
}

func compareQueryMatches(left QueryMatch, right QueryMatch) int {
	if value := compareStrings(left.Name, right.Name); value != 0 {
		return value
	}
	if value := compareStrings(left.MatchType, right.MatchType); value != 0 {
		return value
	}
	return compareStrings(left.Value, right.Value)
}

func compareNamespacedResources(
	leftNamespace string,
	leftName string,
	rightNamespace string,
	rightName string,
) int {
	if value := compareStrings(leftNamespace, rightNamespace); value != 0 {
		return value
	}
	return compareStrings(leftName, rightName)
}

func compareStrings(left string, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func (s Snapshot) Digest() (string, error) {
	if s.ID != "" {
		return s.ID, nil
	}

	return s.computeDigest()
}

func (s Snapshot) computeDigest() (string, error) {
	copy := s.snapshotForDigest()

	raw, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (s Snapshot) snapshotForDigest() Snapshot {
	out := Snapshot{
		Listeners:    make([]Listener, len(s.Listeners)),
		HTTPRoutes:   make([]HTTPRoute, len(s.HTTPRoutes)),
		GRPCRoutes:   make([]GRPCRoute, len(s.GRPCRoutes)),
		StreamRoutes: make([]StreamRoute, len(s.StreamRoutes)),
		Backends:     append([]BackendCluster(nil), s.Backends...),
		Secrets:      append([]SecretMaterial(nil), s.Secrets...),
		Workloads:    append([]Workload(nil), s.Workloads...),
	}

	for idx, listener := range s.Listeners {
		listener.Status = nil
		out.Listeners[idx] = listener
	}

	for idx, route := range s.HTTPRoutes {
		route.Status = nil
		out.HTTPRoutes[idx] = route
	}

	for idx, route := range s.GRPCRoutes {
		route.Status = nil
		out.GRPCRoutes[idx] = route
	}

	for idx, route := range s.StreamRoutes {
		route.Status = nil
		out.StreamRoutes[idx] = route
	}

	return out
}

type AIServiceConfig struct {
	Provider string        `json:"provider"`
	Format   string        `json:"format,omitempty"`
	Model    string        `json:"model"`
	Auth     AIServiceAuth `json:"auth,omitempty"`
	Timeout  time.Duration `json:"timeout,omitempty"`
}

type AIServiceAuth struct {
	Type      string `json:"type,omitempty"`
	SecretRef string `json:"secretRef,omitempty"`
	Header    string `json:"header,omitempty"`
}

type TokenPolicyConfig struct {
	TokensPerMinute   uint64  `json:"tokensPerMinute,omitempty"`
	TokensPerHour     uint64  `json:"tokensPerHour,omitempty"`
	RequestsPerMinute uint64  `json:"requestsPerMinute,omitempty"`
	Scope             string  `json:"scope,omitempty"`
	Burst             float64 `json:"burst,omitempty"`
	OnLimit           string  `json:"onLimit,omitempty"`
}

type WasmPluginConfig struct {
	Name       string            `json:"name,omitempty"`
	Namespace  string            `json:"namespace,omitempty"`
	WasmBytes  []byte            `json:"wasmBytes,omitempty"`
	SHA256     string            `json:"sha256,omitempty"`
	Hooks      []string          `json:"hooks,omitempty"`
	ConfigJSON string            `json:"configJson,omitempty"`
	Sandbox    WasmSandboxConfig `json:"sandbox,omitempty"`
}

type WasmSandboxConfig struct {
	MaxMemoryBytes     uint64 `json:"maxMemoryBytes,omitempty"`
	MaxExecutionTimeMs uint64 `json:"maxExecutionTimeMs,omitempty"`
	AllowNetwork       bool   `json:"allowNetwork,omitempty"`
	AllowFileSystem    bool   `json:"allowFileSystem,omitempty"`
}
