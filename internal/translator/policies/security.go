package policies

import (
	"github.com/nantian-gw/gateway/internal/gatewayexp/securitypolicy"
	"github.com/nantian-gw/gateway/internal/ir"
)

// TranslateSecurityPolicy translates a single SecurityPolicy CRD to IR SecurityPolicyConfig.
func TranslateSecurityPolicy(policy securitypolicy.SecurityPolicy) ir.SecurityPolicyConfig {
	out := ir.SecurityPolicyConfig{}
	if policy.Spec.AuthN != nil {
		out.AuthN = translateAuthN(policy.Spec.AuthN)
	}
	if policy.Spec.AuthZ != nil {
		out.AuthZ = translateAuthZ(policy.Spec.AuthZ)
	}
	if policy.Spec.CORS != nil {
		out.CORS = translateCORS(policy.Spec.CORS)
	}
	if len(policy.Spec.RateLimit) > 0 {
		out.RateLimit = make([]ir.RateLimitRule, len(policy.Spec.RateLimit))
		for i, r := range policy.Spec.RateLimit {
			out.RateLimit[i] = ir.RateLimitRule{
				Scope:             r.Scope,
				RequestsPerSecond: r.RequestsPerSecond,
				Burst:             r.Burst,
				KeyType:           r.KeyType,
				KeyHeaderName:     r.KeyHeaderName,
				OnLimit:           r.OnLimit,
			}
		}
	}
	if policy.Spec.IP != nil {
		out.IP = &ir.SecurityIPConfig{
			AllowCIDRs: policy.Spec.IP.AllowCIDRs,
			DenyCIDRs:  policy.Spec.IP.DenyCIDRs,
		}
	}
	return out
}

func translateAuthN(in *securitypolicy.SecurityAuthNConfig) *ir.SecurityAuthNConfig {
	if in == nil {
		return nil
	}
	out := &ir.SecurityAuthNConfig{}
	if in.JWT != nil {
		j := &ir.JwtAuthConfig{}
		if len(in.JWT.Issuers) > 0 {
			iss := in.JWT.Issuers[0]
			j.Issuer = iss.Issuer
			j.JwksURL = iss.JwksURL
		if iss.Audience != "" {
			j.Audience = iss.Audience
		}
			j.HeaderName = iss.HeaderName
			j.TokenPrefix = iss.TokenPrefix
			j.CacheTTLSecs = iss.CacheTTLSecs
		}
		out.JWT = j
	}
	if in.OIDC != nil {
		out.OIDC = &ir.OIDCConfig{
			ProviderAuthorizationURL: in.OIDC.ProviderAuthorizationURL,
			ProviderTokenURL:         in.OIDC.ProviderTokenURL,
			ProviderJwksURL:          in.OIDC.ProviderJwksURL,
			ProviderUserinfoURL:      in.OIDC.ProviderUserinfoURL,
			ClientID:                 in.OIDC.ClientID,
			ClientSecretRef:          in.OIDC.ClientSecretRef,
			CallbackPath:             in.OIDC.CallbackPath,
			Scopes:                   in.OIDC.Scopes,
			RedirectURL:              in.OIDC.RedirectURL,
			SessionSigningKeyRef:     in.OIDC.SessionSigningKeyRef,
			SessionCookieName:        in.OIDC.SessionCookieName,
			SessionTTLSecs:           in.OIDC.SessionTTLSecs,
		}
	}
	if in.BasicAuth != nil {
		out.BasicAuth = &ir.BasicAuthConfig{
			HtpasswdRef: in.BasicAuth.HtpasswdRef,
			Bcrypt:      in.BasicAuth.Bcrypt,
			Realm:       in.BasicAuth.Realm,
		}
	}
	return out
}

func translateAuthZ(in *securitypolicy.SecurityAuthZConfig) *ir.SecurityAuthZConfig {
	if in == nil || in.External == nil {
		return nil
	}
	e := in.External
	ext := &ir.ExternalAuthConfig{
		Protocol:           e.Protocol,
		ForwardBodyMaxSize: e.ForwardBodyMaxSize,
	}
	if e.BackendRef != nil {
		ext.BackendRef = &ir.BackendRef{
			Namespace: e.BackendRef.Namespace,
			Name:      e.BackendRef.Name,
			Port:      e.BackendRef.Port,
		}
	}
	if e.HTTP != nil {
		ext.HTTP = &ir.ExternalHTTPAuth{
			PathPrefix:   e.HTTP.PathPrefix,
			HeadersToAdd: e.HTTP.HeadersToAdd,
		}
	}
	if e.GRPC != nil {
		ext.GRPC = &ir.ExternalGRPCAuth{
			GRPCService: e.GRPC.GRPCService,
		}
	}
	return &ir.SecurityAuthZConfig{External: ext}
}

func translateCORS(in *securitypolicy.SecurityCORSConfig) *ir.SecurityCORSConfig {
	if in == nil {
		return nil
	}
	return &ir.SecurityCORSConfig{
		AllowOrigins:     in.AllowOrigins,
		AllowMethods:     in.AllowMethods,
		AllowHeaders:     in.AllowHeaders,
		ExposeHeaders:    in.ExposeHeaders,
		AllowCredentials: in.AllowCredentials,
		MaxAge:           in.MaxAge,
	}
}

// TranslateSecurityPolicies translates a batch of SecurityPolicy CRDs to IR configs.
func TranslateSecurityPolicies(policies []securitypolicy.SecurityPolicy) (listenerPolicies, routePolicies, backendPolicies map[string]ir.SecurityPolicyConfig) {
	listenerPolicies = make(map[string]ir.SecurityPolicyConfig)
	routePolicies = make(map[string]ir.SecurityPolicyConfig)
	backendPolicies = make(map[string]ir.SecurityPolicyConfig)
	for _, p := range policies {
		cfg := TranslateSecurityPolicy(p)
		for _, ref := range p.Spec.TargetRefs {
			key := string(ref.Name)
			switch {
			case ref.Group == "gateway.networking.k8s.io" && ref.Kind == "Gateway":
				listenerPolicies[key] = cfg
			case ref.Group == "gateway.networking.k8s.io" && (ref.Kind == "HTTPRoute" || ref.Kind == "GRPCRoute"):
				routePolicies[key] = cfg
			case ref.Group == "" && ref.Kind == "Service":
				backendPolicies[key] = cfg
			}
		}
	}
	return
}
