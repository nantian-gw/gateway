package backendtls

import gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

// PolicyPrecedes reports whether a should take precedence over b according to
// the BackendTLSPolicy conflict resolution order: older creation timestamp
// first, then lexical name order for ties.
func PolicyPrecedes(a, b gatewayv1alpha3.BackendTLSPolicy) bool {
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
