package lbpolicy

import backendlb "github.com/nantian-gw/gateway/internal/gatewayexp/backendlb"

// PolicyPrecedes reports whether a should take precedence over b according to
// the BackendLBPolicy conflict resolution order: older creation timestamp
// first, then lexical name order for ties.
func PolicyPrecedes(a, b backendlb.BackendLBPolicy) bool {
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
