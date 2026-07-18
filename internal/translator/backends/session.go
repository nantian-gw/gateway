package backends

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func RuleSessionPersistence(
	routeKind string,
	routeNamespace string,
	routeName string,
	ruleIndex int,
	source *gatewayv1.SessionPersistence,
) *ir.SessionPersistencePolicy {
	return buildSessionPersistence(
		DefaultRouteSessionName(routeKind, routeNamespace, routeName, ruleIndex),
		source,
	)
}

func BackendSessionPersistence(
	policyNamespace string,
	policyName string,
	source *gatewayv1.SessionPersistence,
) *ir.SessionPersistencePolicy {
	return buildSessionPersistence(
		defaultBackendSessionName(policyNamespace, policyName),
		source,
	)
}

func buildSessionPersistence(
	defaultSessionName string,
	source *gatewayv1.SessionPersistence,
) *ir.SessionPersistencePolicy {
	if source == nil {
		return nil
	}

	out := &ir.SessionPersistencePolicy{
		SessionName: defaultSessionName,
		Type:        string(gatewayv1.CookieBasedSessionPersistence),
	}

	if source.SessionName != nil && strings.TrimSpace(*source.SessionName) != "" {
		out.SessionName = strings.TrimSpace(*source.SessionName)
	}
	if source.Type != nil {
		out.Type = string(*source.Type)
	}
	if source.AbsoluteTimeout != nil {
		if duration, err := time.ParseDuration(string(*source.AbsoluteTimeout)); err == nil {
			out.AbsoluteTimeout = &duration
		}
	}
	if source.IdleTimeout != nil {
		if duration, err := time.ParseDuration(string(*source.IdleTimeout)); err == nil {
			out.IdleTimeout = &duration
		}
	}
	if out.Type == string(gatewayv1.CookieBasedSessionPersistence) {
		out.Cookie = &ir.CookieConfig{
			LifetimeType: string(gatewayv1.SessionCookieLifetimeType),
		}
		if source.CookieConfig != nil && source.CookieConfig.LifetimeType != nil {
			out.Cookie.LifetimeType = string(*source.CookieConfig.LifetimeType)
		}
	}

	return out
}

func DefaultRouteSessionName(routeKind string, routeNamespace string, routeName string, ruleIndex int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%d", routeKind, routeNamespace, routeName, ruleIndex)))
	return "nantian-gw-" + strings.ToLower(routeKind) + "-" + hex.EncodeToString(sum[:6])
}

func defaultBackendSessionName(policyNamespace string, policyName string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("backendlb:%s:%s", policyNamespace, policyName)))
	return "nantian-gw-backendlb-" + hex.EncodeToString(sum[:6])
}
