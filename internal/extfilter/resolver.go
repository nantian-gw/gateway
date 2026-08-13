package extfilter

import (
	"fmt"
	"strings"

	"golang.org/x/net/http/httpguts"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	"github.com/nantian-gw/gateway/internal/constants"
)

const (
	ConfigMapKind      = "ConfigMap"
	ConfigMapDataKey   = "filter.yaml"
	TypeExtensionRef   = "ExtensionRef"
	TypeDirectResponse = "DirectResponse"

	TargetHTTP Target = "http"
	TargetGRPC Target = "grpc"
)

type Target string

type Ref struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

type Result struct {
	Type     string
	Config   map[string]any
	Resolved bool
	Reason   string
	Message  string
}

type Resolver struct {
	configMapByKey map[string]corev1.ConfigMap
}

type document struct {
	Type            string               `yaml:"type"`
	CORS            *corsFilter          `yaml:"cors,omitempty"`
	HeaderModifier  *headerModifier      `yaml:"headerModifier,omitempty"`
	DirectResponse  *directResponse      `yaml:"directResponse,omitempty"`
	RequestRedirect *requestRedirect     `yaml:"requestRedirect,omitempty"`
	URLRewrite      *urlRewrite          `yaml:"urlRewrite,omitempty"`
	RequestMirror   *requestMirrorFilter `yaml:"requestMirror,omitempty"`
}

type corsFilter struct {
	AllowOrigins     []string `yaml:"allowOrigins,omitempty"`
	AllowMethods     []string `yaml:"allowMethods,omitempty"`
	AllowHeaders     []string `yaml:"allowHeaders,omitempty"`
	ExposeHeaders    []string `yaml:"exposeHeaders,omitempty"`
	AllowCredentials *bool    `yaml:"allowCredentials,omitempty"`
	MaxAge           *int     `yaml:"maxAge,omitempty"`
}

type headerModifier struct {
	Set    []headerOperation `yaml:"set,omitempty"`
	Add    []headerOperation `yaml:"add,omitempty"`
	Remove []string          `yaml:"remove,omitempty"`
}

type headerOperation struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

type directResponse struct {
	StatusCode  int               `yaml:"statusCode,omitempty"`
	Body        string            `yaml:"body,omitempty"`
	ContentType string            `yaml:"contentType,omitempty"`
	Headers     []headerOperation `yaml:"headers,omitempty"`
}

type requestRedirect struct {
	Scheme     string        `yaml:"scheme,omitempty"`
	Hostname   string        `yaml:"hostname,omitempty"`
	Path       *pathModifier `yaml:"path,omitempty"`
	Port       int           `yaml:"port,omitempty"`
	StatusCode int           `yaml:"statusCode,omitempty"`
}

type urlRewrite struct {
	Hostname string        `yaml:"hostname,omitempty"`
	Path     *pathModifier `yaml:"path,omitempty"`
}

type pathModifier struct {
	Type               string  `yaml:"type,omitempty"`
	ReplaceFullPath    *string `yaml:"replaceFullPath,omitempty"`
	ReplacePrefixMatch *string `yaml:"replacePrefixMatch,omitempty"`
}

type requestMirrorFilter struct {
	BackendRef *backendRef `yaml:"backendRef,omitempty"`
	Percent    *int        `yaml:"percent,omitempty"`
	Fraction   *fraction   `yaml:"fraction,omitempty"`
}

type backendRef struct {
	Group     string `yaml:"group,omitempty"`
	Kind      string `yaml:"kind,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
	Name      string `yaml:"name,omitempty"`
	Port      int    `yaml:"port,omitempty"`
}

type fraction struct {
	Numerator   int `yaml:"numerator,omitempty"`
	Denominator int `yaml:"denominator,omitempty"`
}

func NewResolver(configMaps []corev1.ConfigMap) Resolver {
	index := make(map[string]corev1.ConfigMap, len(configMaps))
	for _, item := range configMaps {
		index[namespacedName(item.Namespace, item.Name)] = item
	}
	return Resolver{configMapByKey: index}
}

func RefFromLocalRef(namespace string, ref *gatewayv1.LocalObjectReference) Ref {
	if ref == nil {
		return Ref{Namespace: namespace}
	}
	return Ref{
		Group:     string(ref.Group),
		Kind:      string(ref.Kind),
		Namespace: namespace,
		Name:      string(ref.Name),
	}
}

func (r Resolver) Resolve(ref Ref, target Target) Result {
	base := unresolved(ref, string(gatewayv1.RouteReasonUnsupportedValue), "ExtensionRef could not be resolved")
	if ref.Group != "" {
		base.Reason = string(gatewayv1.RouteReasonInvalidKind)
		base.Message = "ExtensionRef group is not supported"
		return base
	}
	if strings.TrimSpace(ref.Kind) != ConfigMapKind {
		base.Reason = string(gatewayv1.RouteReasonInvalidKind)
		base.Message = "ExtensionRef kind is not supported"
		return base
	}
	if strings.TrimSpace(ref.Name) == "" {
		base.Message = "ExtensionRef name must not be empty"
		return base
	}

	configMap, ok := r.configMapByKey[namespacedName(ref.Namespace, ref.Name)]
	if !ok {
		base.Reason = string(gatewayv1.RouteReasonBackendNotFound)
		base.Message = "ExtensionRef ConfigMap was not found"
		return base
	}

	doc, err := decodeDocument(configMap)
	if err != nil {
		base.Message = err.Error()
		return base
	}

	switch doc.Type {
	case "CORS":
		if target != TargetHTTP {
			base.Message = "CORS ExtensionRef is only supported for HTTP routes"
			return base
		}
		config, err := corsConfig(doc.CORS)
		if err != nil {
			base.Message = err.Error()
			return base
		}
		return Result{
			Type:     "CORS",
			Config:   withRefMetadata(config, ref),
			Resolved: true,
			Reason:   string(gatewayv1.RouteReasonResolvedRefs),
			Message:  constants.MsgExtensionResolved,
		}
	case "RequestHeaderModifier":
		config, err := headerModifierConfig(doc.Type, doc.HeaderModifier)
		if err != nil {
			base.Message = err.Error()
			return base
		}
		return Result{
			Type:     string(gatewayv1.HTTPRouteFilterRequestHeaderModifier),
			Config:   withRefMetadata(config, ref),
			Resolved: true,
			Reason:   string(gatewayv1.RouteReasonResolvedRefs),
			Message:  constants.MsgExtensionResolved,
		}
	case "ResponseHeaderModifier":
		config, err := headerModifierConfig(doc.Type, doc.HeaderModifier)
		if err != nil {
			base.Message = err.Error()
			return base
		}
		return Result{
			Type:     string(gatewayv1.HTTPRouteFilterResponseHeaderModifier),
			Config:   withRefMetadata(config, ref),
			Resolved: true,
			Reason:   string(gatewayv1.RouteReasonResolvedRefs),
			Message:  constants.MsgExtensionResolved,
		}
	case "RequestRedirect":
		if target != TargetHTTP {
			base.Message = "RequestRedirect ExtensionRef is only supported for HTTP routes"
			return base
		}
		config, err := requestRedirectConfig(doc.RequestRedirect)
		if err != nil {
			base.Message = err.Error()
			return base
		}
		return Result{
			Type:     string(gatewayv1.HTTPRouteFilterRequestRedirect),
			Config:   withRefMetadata(config, ref),
			Resolved: true,
			Reason:   string(gatewayv1.RouteReasonResolvedRefs),
			Message:  constants.MsgExtensionResolved,
		}
	case "URLRewrite":
		if target != TargetHTTP {
			base.Message = "URLRewrite ExtensionRef is only supported for HTTP routes"
			return base
		}
		config, err := urlRewriteConfig(doc.URLRewrite)
		if err != nil {
			base.Message = err.Error()
			return base
		}
		return Result{
			Type:     string(gatewayv1.HTTPRouteFilterURLRewrite),
			Config:   withRefMetadata(config, ref),
			Resolved: true,
			Reason:   string(gatewayv1.RouteReasonResolvedRefs),
			Message:  constants.MsgExtensionResolved,
		}
	case "RequestMirror":
		config, err := requestMirrorConfig(doc.RequestMirror, ref.Namespace)
		if err != nil {
			base.Message = err.Error()
			return base
		}
		return Result{
			Type:     string(gatewayv1.HTTPRouteFilterRequestMirror),
			Config:   withRefMetadata(config, ref),
			Resolved: true,
			Reason:   string(gatewayv1.RouteReasonResolvedRefs),
			Message:  constants.MsgExtensionResolved,
		}
	case TypeDirectResponse:
		if target != TargetHTTP {
			base.Message = "DirectResponse ExtensionRef is only supported for HTTP routes"
			return base
		}
		config, err := directResponseConfig(doc.DirectResponse, ref)
		if err != nil {
			base.Message = err.Error()
			return base
		}
		return Result{
			Type:     TypeExtensionRef,
			Config:   config,
			Resolved: true,
			Reason:   string(gatewayv1.RouteReasonResolvedRefs),
			Message:  constants.MsgExtensionResolved,
		}
	default:
		base.Message = fmt.Sprintf("unsupported ExtensionRef filter type %q", doc.Type)
		return base
	}
}

func corsConfig(item *corsFilter) (map[string]any, error) {
	if item == nil {
		return nil, fmt.Errorf("CORS ExtensionRef must define cors")
	}

	config := map[string]any{}
	if allowOrigins, err := stringItems("allowOrigins", item.AllowOrigins); err != nil {
		return nil, err
	} else if len(allowOrigins) > 0 {
		config["allowOrigins"] = allowOrigins
	}
	if allowMethods, err := stringItems("allowMethods", item.AllowMethods); err != nil {
		return nil, err
	} else if len(allowMethods) > 0 {
		config["allowMethods"] = allowMethods
	}
	if allowHeaders, err := stringItems("allowHeaders", item.AllowHeaders); err != nil {
		return nil, err
	} else if len(allowHeaders) > 0 {
		config["allowHeaders"] = allowHeaders
	}
	if exposeHeaders, err := stringItems("exposeHeaders", item.ExposeHeaders); err != nil {
		return nil, err
	} else if len(exposeHeaders) > 0 {
		config["exposeHeaders"] = exposeHeaders
	}
	if item.AllowCredentials != nil {
		config["allowCredentials"] = *item.AllowCredentials
	}
	if item.MaxAge != nil {
		if *item.MaxAge < 0 {
			return nil, fmt.Errorf("CORS ExtensionRef maxAge must be greater than or equal to 0")
		}
		config["maxAge"] = *item.MaxAge
	}
	if len(config) == 0 {
		return nil, fmt.Errorf("CORS ExtensionRef must define at least one CORS field")
	}

	return config, nil
}

func decodeDocument(configMap corev1.ConfigMap) (*document, error) {
	raw := strings.TrimSpace(configMap.Data[ConfigMapDataKey])
	if raw == "" {
		return nil, fmt.Errorf("ExtensionRef ConfigMap does not contain %s", ConfigMapDataKey)
	}

	var doc document
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("ExtensionRef ConfigMap contains invalid %s: %w", ConfigMapDataKey, err)
	}
	doc.Type = strings.TrimSpace(doc.Type)
	if doc.Type == "" {
		return nil, fmt.Errorf("ExtensionRef ConfigMap %s must set type", ConfigMapDataKey)
	}

	return &doc, nil
}

func unresolved(ref Ref, reason string, message string) Result {
	return Result{
		Type: TypeExtensionRef,
		Config: map[string]any{
			"resolved":     false,
			"message":      message,
			"extensionRef": refMetadata(ref),
		},
		Resolved: false,
		Reason:   reason,
		Message:  message,
	}
}

func headerModifierConfig(filterType string, modifier *headerModifier) (map[string]any, error) {
	if modifier == nil {
		return nil, fmt.Errorf("%s ExtensionRef must define headerModifier", filterType)
	}

	config := map[string]any{}
	if set, err := headerOperations(filterType, "set", modifier.Set); err != nil {
		return nil, err
	} else if len(set) > 0 {
		config["set"] = set
	}
	if add, err := headerOperations(filterType, "add", modifier.Add); err != nil {
		return nil, err
	} else if len(add) > 0 {
		config["add"] = add
	}
	if remove, err := removeHeaders(filterType, modifier.Remove); err != nil {
		return nil, err
	} else if len(remove) > 0 {
		config["remove"] = remove
	}
	if len(config) == 0 {
		return nil, fmt.Errorf("%s ExtensionRef must define at least one header operation", filterType)
	}
	return config, nil
}

func directResponseConfig(item *directResponse, ref Ref) (map[string]any, error) {
	if item == nil {
		return nil, fmt.Errorf("DirectResponse ExtensionRef must define directResponse")
	}

	statusCode := 500
	switch {
	case item.StatusCode == 0:
	case item.StatusCode >= 100 && item.StatusCode <= 599:
		statusCode = item.StatusCode
	default:
		return nil, fmt.Errorf("DirectResponse ExtensionRef statusCode must be between 100 and 599")
	}

	payload := map[string]any{
		"statusCode": statusCode,
	}

	config := map[string]any{
		"resolved":       true,
		"extensionType":  TypeDirectResponse,
		"directResponse": payload,
		"extensionRef":   refMetadata(ref),
	}
	if body := strings.TrimSpace(item.Body); body != "" {
		payload["body"] = item.Body
	}
	if contentType := strings.TrimSpace(item.ContentType); contentType != "" {
		payload["contentType"] = contentType
	}
	if headers, err := headerOperations(TypeDirectResponse, "headers", item.Headers); err != nil {
		return nil, err
	} else if len(headers) > 0 {
		payload["headers"] = headers
	}
	return config, nil
}

func requestRedirectConfig(item *requestRedirect) (map[string]any, error) {
	if item == nil {
		return nil, fmt.Errorf("RequestRedirect ExtensionRef must define requestRedirect")
	}

	config := map[string]any{}
	if scheme := strings.TrimSpace(item.Scheme); scheme != "" {
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("RequestRedirect ExtensionRef scheme must be http or https")
		}
		config["scheme"] = scheme
	}
	if hostname := strings.TrimSpace(item.Hostname); hostname != "" {
		config["hostname"] = hostname
	}
	if item.Path != nil {
		path, err := pathModifierConfig(item.Path)
		if err != nil {
			return nil, err
		}
		config["path"] = path
	}
	if item.Port > 0 {
		config["port"] = item.Port
	}
	if item.StatusCode > 0 {
		if item.StatusCode != 301 && item.StatusCode != 302 {
			return nil, fmt.Errorf("RequestRedirect ExtensionRef statusCode must be 301 or 302")
		}
		config["statusCode"] = item.StatusCode
	}
	if len(config) == 0 {
		return nil, nil
	}
	return config, nil
}

func urlRewriteConfig(item *urlRewrite) (map[string]any, error) {
	if item == nil {
		return nil, fmt.Errorf("URLRewrite ExtensionRef must define urlRewrite")
	}

	config := map[string]any{}
	if hostname := strings.TrimSpace(item.Hostname); hostname != "" {
		config["hostname"] = hostname
	}
	if item.Path != nil {
		path, err := pathModifierConfig(item.Path)
		if err != nil {
			return nil, err
		}
		config["path"] = path
	}
	if len(config) == 0 {
		return nil, nil
	}
	return config, nil
}

func requestMirrorConfig(item *requestMirrorFilter, defaultNamespace string) (map[string]any, error) {
	if item == nil || item.BackendRef == nil {
		return nil, fmt.Errorf("RequestMirror ExtensionRef must define requestMirror.backendRef")
	}

	name := strings.TrimSpace(item.BackendRef.Name)
	if name == "" {
		return nil, fmt.Errorf("RequestMirror ExtensionRef backendRef.name must not be empty")
	}
	if item.BackendRef.Port <= 0 {
		return nil, fmt.Errorf("RequestMirror ExtensionRef backendRef.port must be greater than 0")
	}

	namespace := strings.TrimSpace(item.BackendRef.Namespace)
	if namespace == "" {
		namespace = defaultNamespace
	}

	group := strings.TrimSpace(item.BackendRef.Group)
	kind := strings.TrimSpace(item.BackendRef.Kind)
	switch {
	case group == "" && (kind == "" || kind == "Service"):
	case group == "multicluster.x-k8s.io" && kind == "ServiceImport":
	default:
		return nil, fmt.Errorf(
			"RequestMirror ExtensionRef backendRef supports only Service and ServiceImport targets, got group=%q kind=%q",
			group,
			kind,
		)
	}
	if item.Percent != nil && item.Fraction != nil {
		return nil, fmt.Errorf("RequestMirror ExtensionRef must not set both percent and fraction")
	}
	if item.Percent != nil && (*item.Percent < 0 || *item.Percent > 100) {
		return nil, fmt.Errorf("RequestMirror ExtensionRef percent must be between 0 and 100")
	}

	backendRef := map[string]any{
		"namespace": namespace,
		"name":      name,
		"port":      item.BackendRef.Port,
	}
	if group != "" {
		backendRef["group"] = group
	}
	if kind != "" {
		backendRef["kind"] = kind
	}

	config := map[string]any{
		"backendRef": backendRef,
	}
	if item.Percent != nil {
		config["percent"] = *item.Percent
	}
	if item.Fraction != nil {
		if item.Fraction.Numerator < 0 {
			return nil, fmt.Errorf("RequestMirror ExtensionRef fraction.numerator must be greater than or equal to 0")
		}
		denominator := item.Fraction.Denominator
		if denominator <= 0 {
			return nil, fmt.Errorf("RequestMirror ExtensionRef fraction.denominator must be greater than 0")
		}
		if item.Fraction.Numerator > denominator {
			return nil, fmt.Errorf("RequestMirror ExtensionRef fraction.numerator must be less than or equal to denominator")
		}
		config["fraction"] = map[string]any{
			"numerator":   item.Fraction.Numerator,
			"denominator": denominator,
		}
	}
	return config, nil
}

func pathModifierConfig(item *pathModifier) (map[string]any, error) {
	if item == nil {
		return nil, nil
	}

	config := map[string]any{}
	modifierType := strings.TrimSpace(item.Type)
	switch modifierType {
	case "ReplaceFullPath", "ReplacePrefixMatch":
		config["type"] = modifierType
	case "":
	default:
		return nil, fmt.Errorf("ExtensionRef path.type uses unsupported value %q", item.Type)
	}
	if item.ReplaceFullPath != nil {
		if modifierType != "ReplaceFullPath" {
			return nil, fmt.Errorf("ExtensionRef path.replaceFullPath requires type ReplaceFullPath")
		}
		config["replaceFullPath"] = *item.ReplaceFullPath
	}
	if item.ReplacePrefixMatch != nil {
		if modifierType != "ReplacePrefixMatch" {
			return nil, fmt.Errorf("ExtensionRef path.replacePrefixMatch requires type ReplacePrefixMatch")
		}
		config["replacePrefixMatch"] = *item.ReplacePrefixMatch
	}
	switch modifierType {
	case "ReplaceFullPath":
		if item.ReplaceFullPath == nil {
			return nil, fmt.Errorf("ExtensionRef path.replaceFullPath must be specified when type is ReplaceFullPath")
		}
	case "ReplacePrefixMatch":
		if item.ReplacePrefixMatch == nil {
			return nil, fmt.Errorf("ExtensionRef path.replacePrefixMatch must be specified when type is ReplacePrefixMatch")
		}
	case "":
		if item.ReplaceFullPath == nil && item.ReplacePrefixMatch == nil {
			return nil, fmt.Errorf("ExtensionRef path.type must be specified")
		}
	}
	if len(config) == 0 {
		return nil, fmt.Errorf("ExtensionRef path.type must be specified")
	}
	return config, nil
}

func headerOperations(filterType string, field string, items []headerOperation) ([]any, error) {
	out := make([]any, 0, len(items))
	for idx, item := range items {
		name := strings.TrimSpace(item.Name)
		if err := validateHeaderName(name); err != nil {
			return nil, fmt.Errorf("%s ExtensionRef %s[%d].name %w", filterType, field, idx, err)
		}
		out = append(out, map[string]any{
			"name":  name,
			"value": item.Value,
		})
	}
	return out, nil
}

func removeHeaders(filterType string, items []string) ([]any, error) {
	out := make([]any, 0, len(items))
	for idx, item := range items {
		item = strings.TrimSpace(item)
		if err := validateHeaderName(item); err != nil {
			return nil, fmt.Errorf("%s ExtensionRef remove[%d] %w", filterType, idx, err)
		}
		out = append(out, item)
	}
	return out, nil
}

func stringItems(field string, items []string) ([]any, error) {
	out := make([]any, 0, len(items))
	for idx, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, fmt.Errorf("CORS ExtensionRef %s[%d] must not be empty", field, idx)
		}
		out = append(out, item)
	}
	return out, nil
}

func validateHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("must not be empty")
	}
	if !httpguts.ValidHeaderFieldName(name) {
		return fmt.Errorf("must be a valid HTTP header name")
	}
	return nil
}

func withRefMetadata(config map[string]any, ref Ref) map[string]any {
	if config == nil {
		config = map[string]any{}
	}
	config["extensionRef"] = refMetadata(ref)
	return config
}

func refMetadata(ref Ref) map[string]any {
	return map[string]any{
		"group":     ref.Group,
		"kind":      ref.Kind,
		"namespace": ref.Namespace,
		"name":      ref.Name,
	}
}

func namespacedName(namespace string, name string) string {
	return namespace + "/" + name
}
