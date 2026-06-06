package backendlb

import backendlbv1alpha2 "github.com/nantian-gw/gateway/internal/gatewayapiexperimental/backendlbv1alpha2"

// PolicyPrecedes reports whether a should take precedence over b according to
// the BackendLBPolicy conflict resolution order: older creation timestamp
// first, then lexical name order for ties.
func PolicyPrecedes(a, b backendlbv1alpha2.BackendLBPolicy) bool {
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
