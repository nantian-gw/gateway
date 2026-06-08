package backendlb

import (
	"fmt"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ValidateSessionPersistence validates the SessionPersistence configuration
// of a BackendLBPolicy. It returns nil if the configuration is valid, or an
// error describing the issue.
func ValidateSessionPersistence(sp *gatewayv1.SessionPersistence) error {
	if sp == nil {
		return nil
	}

	if sp.SessionName != nil && len(*sp.SessionName) > 128 {
		return fmt.Errorf("BackendLBPolicy session persistence sessionName must not exceed 128 characters")
	}

	if sp.CookieConfig != nil &&
		sp.CookieConfig.LifetimeType != nil &&
		*sp.CookieConfig.LifetimeType == gatewayv1.PermanentCookieLifetimeType &&
		sp.AbsoluteTimeout == nil {
		return fmt.Errorf("BackendLBPolicy session persistence with Permanent lifetimeType requires absoluteTimeout")
	}

	return nil
}