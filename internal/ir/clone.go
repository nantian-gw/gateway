package ir

// Cloneable is a constraint for types that can be deep-copied.
type Cloneable[T any] interface {
	Clone() T
}

// cloneSlice returns a deep copy of items.
func cloneSlice[T Cloneable[T]](items []T) []T {
	if items == nil {
		return nil
	}
	result := make([]T, len(items))
	for i, item := range items {
		// Explicit interface conversion required for Go type parameter method call.
		var c Cloneable[T] = item
		result[i] = c.Clone()
	}
	return result
}

// clonePointer returns a deep copy of item, or nil if item is nil.
func clonePointer[T Cloneable[T]](item *T) *T {
	if item == nil {
		return nil
	}
	cloned := (*item).Clone()
	return &cloned
}

// Snapshot.Clone returns a deep copy of the snapshot.
func (s Snapshot) Clone() *Snapshot {
	out := &Snapshot{
		ID:           s.ID,
		GeneratedAt:  s.GeneratedAt,
		Listeners:    cloneSlice(s.Listeners),
		HTTPRoutes:   cloneSlice(s.HTTPRoutes),
		GRPCRoutes:   cloneSlice(s.GRPCRoutes),
		StreamRoutes: cloneSlice(s.StreamRoutes),
		Backends:     cloneSlice(s.Backends),
		Secrets:      append([]SecretMaterial(nil), s.Secrets...),
		Workloads:    append([]Workload(nil), s.Workloads...),
	}
	return out
}

// Clone methods for value types used in slices.

func (l Listener) Clone() Listener {
	cloned := l
	cloned.Addresses = append([]string(nil), l.Addresses...)
	cloned.Hostnames = append([]string(nil), l.Hostnames...)
	cloned.AttachedRoutes = append([]string(nil), l.AttachedRoutes...)
	cloned.Metadata = cloneStringMap(l.Metadata)
	cloned.TLS = clonePointer(l.TLS)
	cloned.BackendTLS = clonePointer(l.BackendTLS)
	cloned.Status = clonePointer(l.Status)
	return cloned
}

func (r HTTPRoute) Clone() HTTPRoute {
	cloned := r
	cloned.Hostnames = append([]string(nil), r.Hostnames...)
	cloned.ParentRefs = append([]ParentRef(nil), r.ParentRefs...)
	cloned.Rules = cloneSlice(r.Rules)
	cloned.Labels = cloneStringMap(r.Labels)
	cloned.Annotations = cloneStringMap(r.Annotations)
	cloned.Status = clonePointer(r.Status)
	cloned.RoutePolicy = clonePointer(r.RoutePolicy)
	return cloned
}

func (r HTTPRule) Clone() HTTPRule {
	cloned := r
	cloned.Matches = cloneSlice(r.Matches)
	cloned.Filters = cloneSlice(r.Filters)
	cloned.BackendRefs = cloneSlice(r.BackendRefs)
	cloned.Timeouts = clonePointer(r.Timeouts)
	cloned.Retry = clonePointer(r.Retry)
	cloned.SessionPersistence = clonePointer(r.SessionPersistence)
	cloned.RoutePolicy = clonePointer(r.RoutePolicy)
	return cloned
}

func (m HTTPMatch) Clone() HTTPMatch {
	cloned := m
	cloned.Headers = append([]HeaderMatch(nil), m.Headers...)
	cloned.QueryParams = append([]QueryMatch(nil), m.QueryParams...)
	return cloned
}

func (r GRPCRoute) Clone() GRPCRoute {
	cloned := r
	cloned.Hostnames = append([]string(nil), r.Hostnames...)
	cloned.ParentRefs = append([]ParentRef(nil), r.ParentRefs...)
	cloned.Rules = cloneSlice(r.Rules)
	cloned.Labels = cloneStringMap(r.Labels)
	cloned.Annotations = cloneStringMap(r.Annotations)
	cloned.Status = clonePointer(r.Status)
	cloned.RoutePolicy = clonePointer(r.RoutePolicy)
	return cloned
}

func (r GRPCRule) Clone() GRPCRule {
	cloned := r
	cloned.Matches = cloneSlice(r.Matches)
	cloned.Filters = cloneSlice(r.Filters)
	cloned.BackendRefs = cloneSlice(r.BackendRefs)
	cloned.SessionPersistence = clonePointer(r.SessionPersistence)
	cloned.RoutePolicy = clonePointer(r.RoutePolicy)
	return cloned
}

func (m GRPCMatch) Clone() GRPCMatch {
	cloned := m
	cloned.Headers = append([]HeaderMatch(nil), m.Headers...)
	return cloned
}

func (r StreamRoute) Clone() StreamRoute {
	cloned := r
	cloned.ParentRefs = append([]ParentRef(nil), r.ParentRefs...)
	cloned.Rules = cloneSlice(r.Rules)
	cloned.Labels = cloneStringMap(r.Labels)
	cloned.Annotations = cloneStringMap(r.Annotations)
	cloned.Status = clonePointer(r.Status)
	cloned.RoutePolicy = clonePointer(r.RoutePolicy)
	return cloned
}

func (r StreamRule) Clone() StreamRule {
	cloned := r
	cloned.Matches = append([]StreamMatch(nil), r.Matches...)
	cloned.BackendRefs = cloneSlice(r.BackendRefs)
	return cloned
}

func (b BackendCluster) Clone() BackendCluster {
	cloned := b
	cloned.Endpoints = append([]BackendEndpoint(nil), b.Endpoints...)
	cloned.BackendTLSValidation = clonePointer(b.BackendTLSValidation)
	cloned.SessionPersistence = clonePointer(b.SessionPersistence)
	cloned.LoadBalancing = clonePointer(b.LoadBalancing)
	cloned.Metadata = cloneStringMap(b.Metadata)
	cloned.CircuitBreaker = clonePointer(b.CircuitBreaker)
	cloned.AIService = clonePointer(b.AIService)
	cloned.TokenPolicy = clonePointer(b.TokenPolicy)
	cloned.WasmPlugin = clonePointer(b.WasmPlugin)
	return cloned
}

func (f Filter) Clone() Filter {
	cloned := f
	cloned.Config = cloneStringAnyMap(f.Config)
	return cloned
}

func (r BackendRef) Clone() BackendRef {
	cloned := r
	cloned.Metadata = cloneStringMap(r.Metadata)
	cloned.Filters = cloneSlice(r.Filters)
	return cloned
}

// Clone methods for pointer types.

func (c TLSConfig) Clone() TLSConfig {
	cloned := c
	cloned.SecretRefs = append([]string(nil), c.SecretRefs...)
	cloned.SNIHosts = append([]string(nil), c.SNIHosts...)
	cloned.FrontendValidation = clonePointer(c.FrontendValidation)
	return cloned
}

func (v FrontendValidation) Clone() FrontendValidation {
	cloned := v
	cloned.ClientCAPEMs = append([]string(nil), v.ClientCAPEMs...)
	return cloned
}

func (c BackendTLSConfig) Clone() BackendTLSConfig {
	return c
}

func (s ListenerStatus) Clone() ListenerStatus {
	cloned := s
	cloned.Conditions = append([]ConditionStatus(nil), s.Conditions...)
	cloned.Accepted = clonePointer(s.Accepted)
	cloned.Programmed = clonePointer(s.Programmed)
	cloned.ResolvedRefs = clonePointer(s.ResolvedRefs)
	return cloned
}

func (s RouteStatus) Clone() RouteStatus {
	cloned := RouteStatus{
		Parents: make([]RouteParentStatus, len(s.Parents)),
	}
	for i, parent := range s.Parents {
		clonedParent := parent
		clonedParent.Conditions = append([]ConditionStatus(nil), parent.Conditions...)
		clonedParent.Accepted = clonePointer(parent.Accepted)
		clonedParent.ResolvedRefs = clonePointer(parent.ResolvedRefs)
		cloned.Parents[i] = clonedParent
	}
	return cloned
}

func (c ConditionStatus) Clone() ConditionStatus {
	return c
}

func (t RouteTimeouts) Clone() RouteTimeouts {
	cloned := t
	if t.Request != nil {
		value := *t.Request
		cloned.Request = &value
	}
	if t.BackendRequest != nil {
		value := *t.BackendRequest
		cloned.BackendRequest = &value
	}
	return cloned
}

func (p RetryPolicy) Clone() RetryPolicy {
	cloned := p
	cloned.Codes = append([]uint32(nil), p.Codes...)
	if p.Backoff != nil {
		value := *p.Backoff
		cloned.Backoff = &value
	}
	return cloned
}

func (p SessionPersistencePolicy) Clone() SessionPersistencePolicy {
	cloned := p
	if p.AbsoluteTimeout != nil {
		value := *p.AbsoluteTimeout
		cloned.AbsoluteTimeout = &value
	}
	if p.IdleTimeout != nil {
		value := *p.IdleTimeout
		cloned.IdleTimeout = &value
	}
	if p.Cookie != nil {
		cookie := *p.Cookie
		cloned.Cookie = &cookie
	}
	return cloned
}

func (p LoadBalancingPolicy) Clone() LoadBalancingPolicy {
	cloned := p
	if p.ConsistentHash != nil {
		hash := *p.ConsistentHash
		cloned.ConsistentHash = &hash
	}
	return cloned
}

func (v BackendTLSValidation) Clone() BackendTLSValidation {
	cloned := v
	cloned.CAPEMs = append([]string(nil), v.CAPEMs...)
	cloned.SubjectAltNames = append([]BackendSubjectName(nil), v.SubjectAltNames...)
	return cloned
}

// Clone methods for types that were previously not cloned (pre-existing bug fix).

func (c RoutePolicyConfig) Clone() RoutePolicyConfig {
	cloned := c
	cloned.Timeout = clonePointer(c.Timeout)
	cloned.BodyLimit = clonePointer(c.BodyLimit)
	cloned.Proxy = clonePointer(c.Proxy)
	cloned.Connection = clonePointer(c.Connection)
	return cloned
}

func (c RouteTimeoutConfig) Clone() RouteTimeoutConfig {
	return c
}

func (c RouteBodyLimitConfig) Clone() RouteBodyLimitConfig {
	return c
}

func (c RouteProxyConfig) Clone() RouteProxyConfig {
	return c
}

func (c RouteConnectionConfig) Clone() RouteConnectionConfig {
	return c
}

func (c AIServiceConfig) Clone() AIServiceConfig {
	return c
}

func (a AIServiceAuth) Clone() AIServiceAuth {
	return a
}

func (c TokenPolicyConfig) Clone() TokenPolicyConfig {
	return c
}

func (c WasmPluginConfig) Clone() WasmPluginConfig {
	cloned := c
	cloned.WasmBytes = append([]byte(nil), c.WasmBytes...)
	cloned.Hooks = append([]string(nil), c.Hooks...)
	return cloned
}

func (c WasmSandboxConfig) Clone() WasmSandboxConfig {
	return c
}

func (c CircuitBreakerConfig) Clone() CircuitBreakerConfig {
	return c
}

// cloneStringMap returns a shallow copy of a string map.
func cloneStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]string, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

// cloneStringAnyMap returns a shallow copy of a string-to-any map.
func cloneStringAnyMap(items map[string]any) map[string]any {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]any, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}