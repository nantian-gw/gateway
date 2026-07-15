package loadbalancing

import backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"

// PolicyPrecedes reports whether a should take precedence over b according to
// the BackendLBPolicy conflict resolution order: older creation timestamp
// first, then lexical name order for ties.
func PolicyPrecedes(a, b backend.BackendLBPolicy) bool {
	if a.CreationTimestamp.Time.Before(b.CreationTimestamp.Time) {
		return true
	}
	if b.CreationTimestamp.Time.Before(a.CreationTimestamp.Time) {
		return false
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.Namespace < b.Namespace
}
