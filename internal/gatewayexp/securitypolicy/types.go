package securitypolicy

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var GroupVersion = schema.GroupVersion{Group: "gateway.nantian.dev", Version: "v1alpha1"}

type SecurityPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SecurityPolicySpec   `json:"spec,omitempty"`
	Status            SecurityPolicyStatus `json:"status,omitempty"`
}

type SecurityPolicySpec struct {
	TargetRefs []gatewayv1.LocalPolicyTargetReference `json:"targetRefs"`
	AuthN      *SecurityAuthNConfig                   `json:"authn,omitempty"`
	AuthZ      *SecurityAuthZConfig                   `json:"authz,omitempty"`
	CORS       *SecurityCORSConfig                    `json:"cors,omitempty"`
	RateLimit  []RateLimitRule                         `json:"rateLimit,omitempty"`
	IP         *SecurityIPConfig                       `json:"ip,omitempty"`
}

type SecurityPolicyStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type SecurityAuthNConfig struct {
	JWT       *JwtAuthConfig    `json:"jwt,omitempty"`
	OIDC      *OIDCConfig       `json:"oidc,omitempty"`
	BasicAuth *BasicAuthConfig  `json:"basicAuth,omitempty"`
}

type SecurityAuthZConfig struct {
	External *ExternalAuthConfig `json:"external,omitempty"`
}

type SecurityCORSConfig struct {
	AllowOrigins     []string `json:"allowOrigins,omitempty"`
	AllowMethods     []string `json:"allowMethods,omitempty"`
	AllowHeaders     []string `json:"allowHeaders,omitempty"`
	ExposeHeaders    []string `json:"exposeHeaders,omitempty"`
	AllowCredentials bool     `json:"allowCredentials,omitempty"`
	MaxAge           int32    `json:"maxAge,omitempty"`
}

type RateLimitRule struct {
	Scope              string `json:"scope,omitempty"`
	RequestsPerSecond  uint32 `json:"requestsPerSecond,omitempty"`
	Burst              uint32 `json:"burst,omitempty"`
	KeyType            string `json:"keyType,omitempty"`
	OnLimit            string `json:"onLimit,omitempty"`
	KeyHeaderName      string `json:"keyHeaderName,omitempty"`
}

type SecurityIPConfig struct {
	AllowCIDRs []string `json:"allowCIDRs,omitempty"`
	DenyCIDRs  []string `json:"denyCIDRs,omitempty"`
}

type JwtAuthConfig struct {
	Issuers []JwtIssuer `json:"issuers,omitempty"`
}

type JwtIssuer struct {
	Issuer           string            `json:"issuer,omitempty"`
	JwksURL          string            `json:"jwksUrl,omitempty"`
	Audience         string            `json:"audience,omitempty"`
	HeaderName       string            `json:"headerName,omitempty"`
	TokenPrefix      string            `json:"tokenPrefix,omitempty"`
	ClaimsToHeaders  map[string]string `json:"claimsToHeaders,omitempty"`
	CacheTTLSecs     int32             `json:"cacheTtlSecs,omitempty"`
}

type OIDCConfig struct {
	ProviderAuthorizationURL string   `json:"providerAuthorizationUrl,omitempty"`
	ProviderTokenURL         string   `json:"providerTokenUrl,omitempty"`
	ProviderJwksURL          string   `json:"providerJwksUrl,omitempty"`
	ProviderUserinfoURL      string   `json:"providerUserinfoUrl,omitempty"`
	ClientID                 string   `json:"clientId,omitempty"`
	ClientSecretRef          string   `json:"clientSecretRef,omitempty"`
	CallbackPath             string   `json:"callbackPath,omitempty"`
	Scopes                   []string `json:"scopes,omitempty"`
	RedirectURL              string   `json:"redirectUrl,omitempty"`
	SessionSigningKeyRef     string   `json:"sessionSigningKeyRef,omitempty"`
	SessionCookieName        string   `json:"sessionCookieName,omitempty"`
	SessionTTLSecs           int32    `json:"sessionTtlSecs,omitempty"`
}

type BasicAuthConfig struct {
	HtpasswdRef string `json:"htpasswdRef,omitempty"`
	Bcrypt      bool   `json:"bcrypt,omitempty"`
	Realm       string `json:"realm,omitempty"`
}

type ExternalAuthConfig struct {
	Protocol          string              `json:"protocol,omitempty"`
	BackendRef        *ExternalAuthBackend `json:"backendRef,omitempty"`
	HTTP              *ExternalAuthHTTP    `json:"http,omitempty"`
	GRPC              *ExternalAuthGRPC    `json:"grpc,omitempty"`
	ForwardBodyMaxSize int32              `json:"forwardBodyMaxSize,omitempty"`
}

type ExternalAuthBackend struct {
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
	Port      uint32 `json:"port,omitempty"`
}

type ExternalAuthHTTP struct {
	PathPrefix   string   `json:"pathPrefix,omitempty"`
	HeadersToAdd []string `json:"headersToAdd,omitempty"`
}

type ExternalAuthGRPC struct {
	GRPCService string `json:"grpcService,omitempty"`
}

type SecurityPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SecurityPolicy `json:"items"`
}

func AddToScheme(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &SecurityPolicy{}, &SecurityPolicyList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

func (in *SecurityPolicy) DeepCopyObject() runtime.Object {
	if in == nil { return nil }
	return in.DeepCopy()
}

func (in *SecurityPolicy) DeepCopy() *SecurityPolicy {
	if in == nil { return nil }
	out := new(SecurityPolicy)
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		for i := range in.Status.Conditions {
			in.Status.Conditions[i].DeepCopyInto(&out.Status.Conditions[i])
		}
	}
	return out
}

func (in *SecurityPolicyList) DeepCopyObject() runtime.Object {
	if in == nil { return nil }
	return in.DeepCopy()
}

func (in *SecurityPolicyList) DeepCopy() *SecurityPolicyList {
	if in == nil { return nil }
	out := new(SecurityPolicyList)
	*out = *in
	if in.Items != nil {
		out.Items = make([]SecurityPolicy, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

func (in *SecurityPolicySpec) DeepCopyInto(out *SecurityPolicySpec) {
	*out = *in
	if in.TargetRefs != nil {
		out.TargetRefs = make([]gatewayv1.LocalPolicyTargetReference, len(in.TargetRefs))
		copy(out.TargetRefs, in.TargetRefs)
	}
	if in.AuthN != nil {
		an := *in.AuthN
		out.AuthN = &an
		if an.JWT != nil {
			jwt := *an.JWT
			out.AuthN.JWT = &jwt
			if jwt.Issuers != nil {
				out.AuthN.JWT.Issuers = make([]JwtIssuer, len(jwt.Issuers))
				for i := range jwt.Issuers {
					out.AuthN.JWT.Issuers[i] = jwt.Issuers[i]
					if jwt.Issuers[i].ClaimsToHeaders != nil {
						ch := make(map[string]string, len(jwt.Issuers[i].ClaimsToHeaders))
						for k, v := range jwt.Issuers[i].ClaimsToHeaders { ch[k] = v }
						out.AuthN.JWT.Issuers[i].ClaimsToHeaders = ch
					}
				}
			}
		}
		if an.OIDC != nil {
			oidc := *an.OIDC
			out.AuthN.OIDC = &oidc
			if oidc.Scopes != nil {
				out.AuthN.OIDC.Scopes = make([]string, len(oidc.Scopes))
				copy(out.AuthN.OIDC.Scopes, oidc.Scopes)
			}
		}
		if an.BasicAuth != nil { ba := *an.BasicAuth; out.AuthN.BasicAuth = &ba }
	}
	if in.AuthZ != nil {
		az := *in.AuthZ
		out.AuthZ = &az
		if az.External != nil {
			ext := *az.External
			out.AuthZ.External = &ext
			if ext.BackendRef != nil { br := *ext.BackendRef; out.AuthZ.External.BackendRef = &br }
			if ext.HTTP != nil { h := *ext.HTTP; out.AuthZ.External.HTTP = &h }
			if ext.GRPC != nil { g := *ext.GRPC; out.AuthZ.External.GRPC = &g }
		}
	}
	if in.CORS != nil {
		c := *in.CORS
		out.CORS = &c
		copySlice := func(s []string) []string {
			if s == nil { return nil }
			d := make([]string, len(s))
			copy(d, s)
			return d
		}
		c.AllowOrigins = copySlice(c.AllowOrigins)
		c.AllowMethods = copySlice(c.AllowMethods)
		c.AllowHeaders = copySlice(c.AllowHeaders)
		c.ExposeHeaders = copySlice(c.ExposeHeaders)
		out.CORS = &c
	}
	if in.RateLimit != nil {
		out.RateLimit = make([]RateLimitRule, len(in.RateLimit))
		copy(out.RateLimit, in.RateLimit)
	}
	if in.IP != nil {
		ip := *in.IP
		out.IP = &ip
		if ip.AllowCIDRs != nil {
			out.IP.AllowCIDRs = make([]string, len(ip.AllowCIDRs))
			copy(out.IP.AllowCIDRs, ip.AllowCIDRs)
		}
		if ip.DenyCIDRs != nil {
			out.IP.DenyCIDRs = make([]string, len(ip.DenyCIDRs))
			copy(out.IP.DenyCIDRs, ip.DenyCIDRs)
		}
	}
}

func (in *SecurityPolicy) DeepCopyInto(out *SecurityPolicy) {
	*out = *in
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	if in.Status.Conditions != nil {
		out.Status.Conditions = make([]metav1.Condition, len(in.Status.Conditions))
		for i := range in.Status.Conditions {
			in.Status.Conditions[i].DeepCopyInto(&out.Status.Conditions[i])
		}
	}
}
