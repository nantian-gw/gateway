package chatbot

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	aiservice "github.com/nantian-gw/gateway/internal/gatewayexp/aiservice"
	backend "github.com/nantian-gw/gateway/internal/gatewayexp/backend"
	tokenpolicy "github.com/nantian-gw/gateway/internal/gatewayexp/tokenpolicy"
	wasmplugin "github.com/nantian-gw/gateway/internal/gatewayexp/wasmplugin"
)

// renderDetail returns the Markdown body (placed under the "### Kind ns/name"
// header) for a selected resource, expanding troubleshooting-relevant fields
// from the retained typed object. It never fails; a nil or unrecognized object
// falls back to the index entry's one-line summary.
func renderDetail(obj client.Object, entry IndexEntry) string {
	switch o := obj.(type) {
	case *gatewayv1.Gateway:
		return renderGateway(o)
	case *corev1.Service:
		return renderService(o)
	case *gatewayv1.HTTPRoute:
		return renderHTTPRoute(o)
	case *gatewayv1.GRPCRoute:
		return renderGRPCRoute(o)
	case *gatewayv1alpha2.TLSRoute:
		return renderTLSRoute(o)
	case *gatewayv1alpha2.TCPRoute:
		return renderTCPRoute(o)
	case *gatewayv1alpha2.UDPRoute:
		return renderUDPRoute(o)
	case *aiservice.AIService:
		return renderAIService(o)
	case *tokenpolicy.TokenPolicy:
		return renderTokenPolicy(o)
	case *wasmplugin.WasmPlugin:
		return renderWasmPlugin(o)
	case *backend.BackendLBPolicy:
		return renderBackendLBPolicy(o)
	default:
		return fallbackDetail(entry)
	}
}

func fallbackDetail(entry IndexEntry) string {
	var sb strings.Builder
	if entry.Summary != "" {
		sb.WriteString(entry.Summary)
		sb.WriteString("\n")
	}
	writeConditions(&sb, entry.StatusSummary)
	return sb.String()
}

func writeConditions(sb *strings.Builder, s string) {
	if s != "" {
		sb.WriteString("status: ")
		sb.WriteString(s)
		sb.WriteString("\n")
	}
}

func backendLine(sb *strings.Builder, routeNS string, br gatewayv1.BackendRef) {
	ns := routeNS
	if br.Namespace != nil {
		ns = string(*br.Namespace)
	}
	port := int32(0)
	if br.Port != nil {
		port = int32(*br.Port)
	}
	fmt.Fprintf(sb, "    -> %s/%s:%d", sanitizeUntrusted(ns), sanitizeUntrusted(string(br.Name)), port)
	if br.Weight != nil {
		fmt.Fprintf(sb, " weight=%d", *br.Weight)
	}
	sb.WriteString("\n")
}

func renderGateway(gw *gatewayv1.Gateway) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "class=%s\n", sanitizeUntrusted(string(gw.Spec.GatewayClassName)))
	for _, l := range gw.Spec.Listeners {
		host := "-"
		if l.Hostname != nil {
			host = string(*l.Hostname)
		}
		fmt.Fprintf(&sb, "- %s: %d/%s hostname=%s", sanitizeUntrusted(string(l.Name)), l.Port, l.Protocol, sanitizeUntrusted(host))
		if l.TLS != nil {
			mode := "-"
			if l.TLS.Mode != nil {
				mode = string(*l.TLS.Mode)
			}
			fmt.Fprintf(&sb, " tls=%s(certRefs=%d)", mode, len(l.TLS.CertificateRefs))
		}
		sb.WriteString("\n")
	}
	status, _ := summarizeConditions(gw.Status.Conditions)
	writeConditions(&sb, status)
	return sb.String()
}

func renderService(svc *corev1.Service) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "type=%s clusterIP=%s\n", svc.Spec.Type, svc.Spec.ClusterIP)
	for _, p := range svc.Spec.Ports {
		fmt.Fprintf(&sb, "- %s: %d/%s -> %s\n", sanitizeUntrusted(p.Name), p.Port, p.Protocol, sanitizeUntrusted(p.TargetPort.String()))
	}
	return sb.String()
}

func writeHostnames(sb *strings.Builder, hostnames []gatewayv1.Hostname) {
	if len(hostnames) == 0 {
		return
	}
	hs := make([]string, 0, len(hostnames))
	for _, h := range hostnames {
		hs = append(hs, sanitizeUntrusted(string(h)))
	}
	fmt.Fprintf(sb, "hostnames=[%s]\n", strings.Join(hs, ","))
}

func renderHTTPRoute(r *gatewayv1.HTTPRoute) string {
	var sb strings.Builder
	writeHostnames(&sb, r.Spec.Hostnames)
	for i, rule := range r.Spec.Rules {
		fmt.Fprintf(&sb, "rule[%d]:\n", i)
		for _, m := range rule.Matches {
			if m.Path != nil && m.Path.Type != nil && m.Path.Value != nil {
				fmt.Fprintf(&sb, "  match: path %s=%s\n", *m.Path.Type, sanitizeUntrusted(*m.Path.Value))
			}
			if m.Method != nil {
				fmt.Fprintf(&sb, "  match: method=%s\n", *m.Method)
			}
			for _, h := range m.Headers {
				fmt.Fprintf(&sb, "  match: header %s=%s\n", sanitizeUntrusted(string(h.Name)), sanitizeUntrusted(h.Value))
			}
		}
		for _, br := range rule.BackendRefs {
			backendLine(&sb, r.Namespace, br.BackendRef)
		}
		for _, f := range rule.Filters {
			fmt.Fprintf(&sb, "  filter: %s\n", f.Type)
		}
	}
	status, _ := summarizeRouteParents(r.Status.Parents)
	writeConditions(&sb, status)
	return sb.String()
}

func renderGRPCRoute(r *gatewayv1.GRPCRoute) string {
	var sb strings.Builder
	for i, rule := range r.Spec.Rules {
		fmt.Fprintf(&sb, "rule[%d]:\n", i)
		for _, m := range rule.Matches {
			if m.Method != nil {
				svc, meth := "*", "*"
				if m.Method.Service != nil {
					svc = *m.Method.Service
				}
				if m.Method.Method != nil {
					meth = *m.Method.Method
				}
				fmt.Fprintf(&sb, "  match: service=%s method=%s\n", sanitizeUntrusted(svc), sanitizeUntrusted(meth))
			}
		}
		for _, br := range rule.BackendRefs {
			backendLine(&sb, r.Namespace, br.BackendRef)
		}
	}
	status, _ := summarizeRouteParents(r.Status.Parents)
	writeConditions(&sb, status)
	return sb.String()
}

func renderTLSRoute(r *gatewayv1alpha2.TLSRoute) string {
	var sb strings.Builder
	writeHostnames(&sb, r.Spec.Hostnames)
	for i, rule := range r.Spec.Rules {
		fmt.Fprintf(&sb, "rule[%d]:\n", i)
		for _, br := range rule.BackendRefs {
			backendLine(&sb, r.Namespace, br)
		}
	}
	status, _ := summarizeRouteParents(r.Status.Parents)
	writeConditions(&sb, status)
	return sb.String()
}

func renderTCPRoute(r *gatewayv1alpha2.TCPRoute) string {
	var sb strings.Builder
	for i, rule := range r.Spec.Rules {
		fmt.Fprintf(&sb, "rule[%d]:\n", i)
		for _, br := range rule.BackendRefs {
			backendLine(&sb, r.Namespace, br)
		}
	}
	status, _ := summarizeRouteParents(r.Status.Parents)
	writeConditions(&sb, status)
	return sb.String()
}

func renderUDPRoute(r *gatewayv1alpha2.UDPRoute) string {
	var sb strings.Builder
	for i, rule := range r.Spec.Rules {
		fmt.Fprintf(&sb, "rule[%d]:\n", i)
		for _, br := range rule.BackendRefs {
			backendLine(&sb, r.Namespace, br)
		}
	}
	status, _ := summarizeRouteParents(r.Status.Parents)
	writeConditions(&sb, status)
	return sb.String()
}

func renderAIService(s *aiservice.AIService) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "provider=%s model=%s", sanitizeUntrusted(s.Spec.Provider), sanitizeUntrusted(s.Spec.Model))
	if s.Spec.Format != "" {
		fmt.Fprintf(&sb, " format=%s", sanitizeUntrusted(s.Spec.Format))
	}
	if s.Spec.Endpoint != "" {
		fmt.Fprintf(&sb, " endpoint=%s", sanitizeUntrusted(s.Spec.Endpoint))
	}
	if s.Spec.Auth.Type != "" {
		fmt.Fprintf(&sb, " auth=%s", sanitizeUntrusted(s.Spec.Auth.Type))
	}
	if s.Spec.Timeout != "" {
		fmt.Fprintf(&sb, " timeout=%s", sanitizeUntrusted(s.Spec.Timeout))
	}
	sb.WriteString("\n")
	status, _ := summarizeConditions(s.Status.Conditions)
	writeConditions(&sb, status)
	return sb.String()
}

func renderTokenPolicy(p *tokenpolicy.TokenPolicy) string {
	var sb strings.Builder
	for _, tr := range p.Spec.TargetRefs {
		fmt.Fprintf(&sb, "targetRef=%s/%s\n", sanitizeUntrusted(string(tr.Kind)), sanitizeUntrusted(string(tr.Name)))
	}
	fmt.Fprintf(&sb, "tpm=%d rpm=%d burst=%.2f onLimit=%s scope=%s\n",
		p.Spec.TokensPerMinute, p.Spec.RequestsPerMinute, p.Spec.Burst, sanitizeUntrusted(p.Spec.OnLimit), sanitizeUntrusted(p.Spec.Scope))
	status, _ := summarizeConditions(p.Status.Conditions)
	writeConditions(&sb, status)
	return sb.String()
}

func renderWasmPlugin(p *wasmplugin.WasmPlugin) string {
	var sb strings.Builder
	src := "-"
	switch {
	case p.Spec.Wasm.URL != "":
		src = p.Spec.Wasm.URL
	case p.Spec.Wasm.Inline != "":
		src = "inline"
	case p.Spec.Wasm.ConfigMap != nil:
		src = "configMap/" + p.Spec.Wasm.ConfigMap.Name
	}
	fmt.Fprintf(&sb, "source=%s\n", sanitizeUntrusted(src))
	hooks := make([]string, 0, len(p.Spec.Hooks))
	for _, h := range p.Spec.Hooks {
		hooks = append(hooks, string(h))
	}
	fmt.Fprintf(&sb, "hooks=[%s]\n", strings.Join(hooks, ","))
	fmt.Fprintf(&sb, "sandbox: maxMemoryBytes=%d maxExecutionTimeMs=%d allowNetwork=%t allowFileSystem=%t\n",
		p.Spec.Sandbox.MaxMemoryBytes, p.Spec.Sandbox.MaxExecutionTimeMs, p.Spec.Sandbox.AllowNetwork, p.Spec.Sandbox.AllowFileSystem)
	for _, tr := range p.Spec.TargetRefs {
		fmt.Fprintf(&sb, "targetRef=%s/%s\n", sanitizeUntrusted(string(tr.Kind)), sanitizeUntrusted(string(tr.Name)))
	}
	status, _ := summarizeConditions(p.Status.Conditions)
	writeConditions(&sb, status)
	return sb.String()
}

func renderBackendLBPolicy(p *backend.BackendLBPolicy) string {
	var sb strings.Builder
	for _, tr := range p.Spec.TargetRefs {
		fmt.Fprintf(&sb, "targetRef=%s/%s\n", sanitizeUntrusted(string(tr.Kind)), sanitizeUntrusted(string(tr.Name)))
	}
	if p.Spec.LoadBalancing != nil && p.Spec.LoadBalancing.Type != nil {
		fmt.Fprintf(&sb, "lb=%s", *p.Spec.LoadBalancing.Type)
		if p.Spec.LoadBalancing.ConsistentHash != nil && p.Spec.LoadBalancing.ConsistentHash.KeyType != nil {
			fmt.Fprintf(&sb, " keyType=%s", *p.Spec.LoadBalancing.ConsistentHash.KeyType)
		}
		sb.WriteString("\n")
	}
	if p.Spec.SessionPersistence != nil {
		sb.WriteString("sessionPersistence=enabled\n")
	}
	if p.Spec.CircuitBreaker != nil && p.Spec.CircuitBreaker.MaxInflightRequests != nil {
		fmt.Fprintf(&sb, "circuitBreaker.maxInflightRequests=%d\n", *p.Spec.CircuitBreaker.MaxInflightRequests)
	}
	status, _ := summarizeAncestors(p.Status.Ancestors)
	writeConditions(&sb, status)
	return sb.String()
}
