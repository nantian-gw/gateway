package ir

func (s Snapshot) Clone() *Snapshot {
	out := &Snapshot{
		ID:           s.ID,
		GeneratedAt:  s.GeneratedAt,
		Listeners:    cloneListeners(s.Listeners),
		HTTPRoutes:   cloneHTTPRoutes(s.HTTPRoutes),
		GRPCRoutes:   cloneGRPCRoutes(s.GRPCRoutes),
		StreamRoutes: cloneStreamRoutes(s.StreamRoutes),
		Backends:     cloneBackends(s.Backends),
		Secrets:      append([]SecretMaterial(nil), s.Secrets...),
		Workloads:    append([]Workload(nil), s.Workloads...),
	}
	return out
}

func cloneListeners(items []Listener) []Listener {
	out := make([]Listener, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Addresses = append([]string(nil), item.Addresses...)
		cloned.Hostnames = append([]string(nil), item.Hostnames...)
		cloned.AttachedRoutes = append([]string(nil), item.AttachedRoutes...)
		cloned.Metadata = cloneStringMap(item.Metadata)
		cloned.TLS = cloneTLSConfig(item.TLS)
		cloned.BackendTLS = cloneBackendTLSConfig(item.BackendTLS)
		cloned.Status = cloneListenerStatus(item.Status)
		out = append(out, cloned)
	}
	return out
}

func cloneHTTPRoutes(items []HTTPRoute) []HTTPRoute {
	out := make([]HTTPRoute, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Hostnames = append([]string(nil), item.Hostnames...)
		cloned.ParentRefs = append([]ParentRef(nil), item.ParentRefs...)
		cloned.Rules = cloneHTTPRules(item.Rules)
		cloned.Labels = cloneStringMap(item.Labels)
		cloned.Annotations = cloneStringMap(item.Annotations)
		cloned.Status = cloneRouteStatus(item.Status)
		out = append(out, cloned)
	}
	return out
}

func cloneHTTPRules(items []HTTPRule) []HTTPRule {
	out := make([]HTTPRule, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Matches = cloneHTTPMatches(item.Matches)
		cloned.Filters = cloneFilters(item.Filters)
		cloned.BackendRefs = cloneBackendRefs(item.BackendRefs)
		cloned.Timeouts = cloneRouteTimeouts(item.Timeouts)
		cloned.Retry = cloneRetryPolicy(item.Retry)
		cloned.SessionPersistence = cloneSessionPersistencePolicy(item.SessionPersistence)
		out = append(out, cloned)
	}
	return out
}

func cloneHTTPMatches(items []HTTPMatch) []HTTPMatch {
	out := make([]HTTPMatch, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Headers = append([]HeaderMatch(nil), item.Headers...)
		cloned.QueryParams = append([]QueryMatch(nil), item.QueryParams...)
		out = append(out, cloned)
	}
	return out
}

func cloneGRPCRoutes(items []GRPCRoute) []GRPCRoute {
	out := make([]GRPCRoute, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Hostnames = append([]string(nil), item.Hostnames...)
		cloned.ParentRefs = append([]ParentRef(nil), item.ParentRefs...)
		cloned.Rules = cloneGRPCRules(item.Rules)
		cloned.Labels = cloneStringMap(item.Labels)
		cloned.Annotations = cloneStringMap(item.Annotations)
		cloned.Status = cloneRouteStatus(item.Status)
		out = append(out, cloned)
	}
	return out
}

func cloneGRPCRules(items []GRPCRule) []GRPCRule {
	out := make([]GRPCRule, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Matches = append([]GRPCMatch(nil), item.Matches...)
		cloned.Filters = cloneFilters(item.Filters)
		cloned.BackendRefs = cloneBackendRefs(item.BackendRefs)
		cloned.SessionPersistence = cloneSessionPersistencePolicy(item.SessionPersistence)
		out = append(out, cloned)
	}
	return out
}

func cloneStreamRoutes(items []StreamRoute) []StreamRoute {
	out := make([]StreamRoute, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.ParentRefs = append([]ParentRef(nil), item.ParentRefs...)
		cloned.Rules = cloneStreamRules(item.Rules)
		cloned.Labels = cloneStringMap(item.Labels)
		cloned.Annotations = cloneStringMap(item.Annotations)
		cloned.Status = cloneRouteStatus(item.Status)
		out = append(out, cloned)
	}
	return out
}

func cloneStreamRules(items []StreamRule) []StreamRule {
	out := make([]StreamRule, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Matches = append([]StreamMatch(nil), item.Matches...)
		cloned.BackendRefs = cloneBackendRefs(item.BackendRefs)
		out = append(out, cloned)
	}
	return out
}

func cloneBackends(items []BackendCluster) []BackendCluster {
	out := make([]BackendCluster, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Endpoints = append([]BackendEndpoint(nil), item.Endpoints...)
		cloned.BackendTLSValidation = cloneBackendTLSValidation(item.BackendTLSValidation)
		cloned.SessionPersistence = cloneSessionPersistencePolicy(item.SessionPersistence)
		cloned.LoadBalancing = cloneLoadBalancingPolicy(item.LoadBalancing)
		cloned.Metadata = cloneStringMap(item.Metadata)
		out = append(out, cloned)
	}
	return out
}

func cloneFilters(items []Filter) []Filter {
	out := make([]Filter, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Config = cloneStringAnyMap(item.Config)
		out = append(out, cloned)
	}
	return out
}

func cloneBackendRefs(items []BackendRef) []BackendRef {
	out := make([]BackendRef, 0, len(items))
	for _, item := range items {
		cloned := item
		cloned.Metadata = cloneStringMap(item.Metadata)
		cloned.Filters = cloneFilters(item.Filters)
		out = append(out, cloned)
	}
	return out
}

func cloneTLSConfig(item *TLSConfig) *TLSConfig {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.SecretRefs = append([]string(nil), item.SecretRefs...)
	cloned.SNIHosts = append([]string(nil), item.SNIHosts...)
	cloned.FrontendValidation = cloneFrontendValidation(item.FrontendValidation)
	return &cloned
}

func cloneFrontendValidation(item *FrontendValidation) *FrontendValidation {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.ClientCAPEMs = append([]string(nil), item.ClientCAPEMs...)
	return &cloned
}

func cloneBackendTLSConfig(item *BackendTLSConfig) *BackendTLSConfig {
	if item == nil {
		return nil
	}
	cloned := *item
	return &cloned
}

func cloneListenerStatus(item *ListenerStatus) *ListenerStatus {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Conditions = append([]ConditionStatus(nil), item.Conditions...)
	cloned.Accepted = cloneConditionStatus(item.Accepted)
	cloned.Programmed = cloneConditionStatus(item.Programmed)
	cloned.ResolvedRefs = cloneConditionStatus(item.ResolvedRefs)
	return &cloned
}

func cloneRouteStatus(item *RouteStatus) *RouteStatus {
	if item == nil {
		return nil
	}
	cloned := &RouteStatus{
		Parents: make([]RouteParentStatus, 0, len(item.Parents)),
	}
	for _, parent := range item.Parents {
		clonedParent := parent
		clonedParent.Conditions = append([]ConditionStatus(nil), parent.Conditions...)
		clonedParent.Accepted = cloneConditionStatus(parent.Accepted)
		clonedParent.ResolvedRefs = cloneConditionStatus(parent.ResolvedRefs)
		cloned.Parents = append(cloned.Parents, clonedParent)
	}
	return cloned
}

func cloneConditionStatus(item *ConditionStatus) *ConditionStatus {
	if item == nil {
		return nil
	}
	cloned := *item
	return &cloned
}

func cloneRouteTimeouts(item *RouteTimeouts) *RouteTimeouts {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.Request != nil {
		value := *item.Request
		cloned.Request = &value
	}
	if item.BackendRequest != nil {
		value := *item.BackendRequest
		cloned.BackendRequest = &value
	}
	return &cloned
}

func cloneRetryPolicy(item *RetryPolicy) *RetryPolicy {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.Codes = append([]uint32(nil), item.Codes...)
	if item.Backoff != nil {
		value := *item.Backoff
		cloned.Backoff = &value
	}
	return &cloned
}

func cloneSessionPersistencePolicy(item *SessionPersistencePolicy) *SessionPersistencePolicy {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.AbsoluteTimeout != nil {
		value := *item.AbsoluteTimeout
		cloned.AbsoluteTimeout = &value
	}
	if item.IdleTimeout != nil {
		value := *item.IdleTimeout
		cloned.IdleTimeout = &value
	}
	if item.Cookie != nil {
		cookie := *item.Cookie
		cloned.Cookie = &cookie
	}
	return &cloned
}

func cloneLoadBalancingPolicy(item *LoadBalancingPolicy) *LoadBalancingPolicy {
	if item == nil {
		return nil
	}
	cloned := *item
	if item.ConsistentHash != nil {
		hash := *item.ConsistentHash
		cloned.ConsistentHash = &hash
	}
	return &cloned
}

func cloneBackendTLSValidation(item *BackendTLSValidation) *BackendTLSValidation {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.CAPEMs = append([]string(nil), item.CAPEMs...)
	cloned.SubjectAltNames = append([]BackendSubjectName(nil), item.SubjectAltNames...)
	return &cloned
}

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
