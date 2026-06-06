package grpcserver

import (
	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
	"github.com/nantian-gw/gateway/internal/ir"
)

func toProtoSessionPersistence(item *ir.SessionPersistencePolicy) *controlv1.SessionPersistence {
	if item == nil {
		return nil
	}

	out := &controlv1.SessionPersistence{
		SessionName:     item.SessionName,
		Type:            toProtoSessionType(item.Type),
		AbsoluteTimeout: durationOrNil(item.AbsoluteTimeout),
		IdleTimeout:     durationOrNil(item.IdleTimeout),
		Cookie:          toProtoCookieConfig(item.Cookie),
	}

	return out
}

func toProtoCookieConfig(item *ir.CookieConfig) *controlv1.CookieConfig {
	if item == nil {
		return nil
	}

	return &controlv1.CookieConfig{
		LifetimeType: toProtoCookieLifetimeType(item.LifetimeType),
	}
}

func toProtoSessionType(value string) controlv1.SessionPersistenceType {
	switch value {
	case "Header":
		return controlv1.SessionPersistenceType_SESSION_PERSISTENCE_TYPE_HEADER
	case "Cookie":
		return controlv1.SessionPersistenceType_SESSION_PERSISTENCE_TYPE_COOKIE
	default:
		return controlv1.SessionPersistenceType_SESSION_PERSISTENCE_TYPE_UNSPECIFIED
	}
}

func toProtoCookieLifetimeType(value string) controlv1.CookieLifetimeType {
	switch value {
	case "Permanent":
		return controlv1.CookieLifetimeType_COOKIE_LIFETIME_TYPE_PERMANENT
	case "Session":
		return controlv1.CookieLifetimeType_COOKIE_LIFETIME_TYPE_SESSION
	default:
		return controlv1.CookieLifetimeType_COOKIE_LIFETIME_TYPE_UNSPECIFIED
	}
}
