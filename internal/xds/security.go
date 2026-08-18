package xds

import (
	"strings"

	controlv1 "github.com/nantian-gw/proto/gateway/control/v1"
	"github.com/nantian-gw/gateway/internal/ir"
)

func toProtoSecurityPolicy(cfg *ir.SecurityPolicyConfig) *controlv1.SecurityPolicyConfig {
	if cfg == nil {
		return nil
	}
	out := &controlv1.SecurityPolicyConfig{}
	if cfg.AuthN != nil {
		out.Authn = toProtoAuthN(cfg.AuthN)
	}
	if cfg.AuthZ != nil {
		out.Authz = toProtoAuthZ(cfg.AuthZ)
	}
	if cfg.CORS != nil {
		out.Cors = toProtoCORS(cfg.CORS)
	}
	if len(cfg.RateLimit) > 0 {
		out.RateLimit = make([]*controlv1.RateLimitRule, len(cfg.RateLimit))
		for i, r := range cfg.RateLimit {
			out.RateLimit[i] = &controlv1.RateLimitRule{
				Scope:             toProtoRateLimitScope(r.Scope),
				RequestsPerSecond: r.RequestsPerSecond,
				Burst:             r.Burst,
				KeyType:           r.KeyType,
				OnLimit:           toProtoRateLimitAction(r.OnLimit),
				KeyHeaderName:     r.KeyHeaderName,
			}
		}
	}
	if cfg.IP != nil {
		out.Ip = &controlv1.SecurityIPConfig{
			AllowCidrs: cfg.IP.AllowCIDRs,
			DenyCidrs:  cfg.IP.DenyCIDRs,
		}
	}
	return out
}

func toProtoAuthN(cfg *ir.SecurityAuthNConfig) *controlv1.SecurityAuthNConfig {
	if cfg == nil {
		return nil
	}
	out := &controlv1.SecurityAuthNConfig{}
	if cfg.JWT != nil {
		out.Jwt = &controlv1.JwtAuthConfig{
			Issuers: []*controlv1.JwtIssuer{{
				Issuer:        cfg.JWT.Issuer,
				JwksUrl:       cfg.JWT.JwksURL,
				Audience:      cfg.JWT.Audience,
				HeaderName:    cfg.JWT.HeaderName,
				TokenPrefix:   cfg.JWT.TokenPrefix,
				CacheTtlSecs:  cfg.JWT.CacheTTLSecs,
			}},
		}
		for _, ch := range cfg.JWT.ClaimsToHeader {
			out.Jwt.Issuers[0].ClaimsToHeaders[ch.Claim] = ch.Header
		}
	}
	if cfg.OIDC != nil {
		out.Oidc = &controlv1.OIDCConfig{
			ProviderAuthorizationUrl: cfg.OIDC.ProviderAuthorizationURL,
			ProviderTokenUrl:         cfg.OIDC.ProviderTokenURL,
			ProviderJwksUrl:          cfg.OIDC.ProviderJwksURL,
			ProviderUserinfoUrl:      cfg.OIDC.ProviderUserinfoURL,
			ClientId:                 cfg.OIDC.ClientID,
			ClientSecretRef:          cfg.OIDC.ClientSecretRef,
			CallbackPath:             cfg.OIDC.CallbackPath,
			Scopes:                   cfg.OIDC.Scopes,
			RedirectUrl:              cfg.OIDC.RedirectURL,
			SessionSigningKeyRef:     cfg.OIDC.SessionSigningKeyRef,
			SessionCookieName:        cfg.OIDC.SessionCookieName,
			SessionTtlSecs:           cfg.OIDC.SessionTTLSecs,
		}
	}
	if cfg.BasicAuth != nil {
		out.BasicAuth = &controlv1.BasicAuthConfig{
			HtpasswdRef: cfg.BasicAuth.HtpasswdRef,
			Bcrypt:      cfg.BasicAuth.Bcrypt,
			Realm:       cfg.BasicAuth.Realm,
		}
	}
	return out
}

func toProtoAuthZ(cfg *ir.SecurityAuthZConfig) *controlv1.SecurityAuthZConfig {
	if cfg == nil || cfg.External == nil {
		return nil
	}
	e := cfg.External
	ext := &controlv1.ExternalAuthConfig{
		Protocol:           toProtoExternalAuthTransport(e.Protocol),
		ForwardBodyMaxSize: e.ForwardBodyMaxSize,
	}
	if e.BackendRef != nil {
		ext.BackendRef = &controlv1.BackendRef{
			Namespace: e.BackendRef.Namespace,
			Name:      e.BackendRef.Name,
			Port:      e.BackendRef.Port,
		}
	}
	if e.HTTP != nil {
		ext.Http = &controlv1.ExternalAuthHTTP{
			PathPrefix:   e.HTTP.PathPrefix,
			HeadersToAdd: e.HTTP.HeadersToAdd,
		}
	}
	if e.GRPC != nil {
		ext.Grpc = &controlv1.ExternalAuthGRPC{
			GrpcService: e.GRPC.GRPCService,
		}
	}
	return &controlv1.SecurityAuthZConfig{External: ext}
}

func toProtoCORS(cfg *ir.SecurityCORSConfig) *controlv1.SecurityCORSConfig {
	if cfg == nil {
		return nil
	}
	return &controlv1.SecurityCORSConfig{
		AllowOrigins:     cfg.AllowOrigins,
		AllowMethods:     cfg.AllowMethods,
		AllowHeaders:     cfg.AllowHeaders,
		ExposeHeaders:    cfg.ExposeHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge,
	}
}

func toProtoRateLimitScope(scope string) controlv1.RateLimitScope {
	switch strings.ToLower(scope) {
	case "global":
		return controlv1.RateLimitScope_RATE_LIMIT_SCOPE_GLOBAL
	case "listener":
		return controlv1.RateLimitScope_RATE_LIMIT_SCOPE_LISTENER
	case "route":
		return controlv1.RateLimitScope_RATE_LIMIT_SCOPE_ROUTE
	case "backend":
		return controlv1.RateLimitScope_RATE_LIMIT_SCOPE_BACKEND
	default:
		return controlv1.RateLimitScope_RATE_LIMIT_SCOPE_UNSPECIFIED
	}
}

func toProtoRateLimitAction(action string) controlv1.RateLimitAction {
	switch strings.ToLower(action) {
	case "reject":
		return controlv1.RateLimitAction_RATE_LIMIT_ACTION_REJECT
	case "queue":
		return controlv1.RateLimitAction_RATE_LIMIT_ACTION_QUEUE
	default:
		return controlv1.RateLimitAction_RATE_LIMIT_ACTION_UNSPECIFIED
	}
}

func toProtoExternalAuthTransport(proto string) controlv1.ExternalAuthTransport {
	switch strings.ToLower(proto) {
	case "http":
		return controlv1.ExternalAuthTransport_EXTERNAL_AUTH_TRANSPORT_HTTP
	case "grpc":
		return controlv1.ExternalAuthTransport_EXTERNAL_AUTH_TRANSPORT_GRPC
	default:
		return controlv1.ExternalAuthTransport_EXTERNAL_AUTH_TRANSPORT_UNSPECIFIED
	}
}
