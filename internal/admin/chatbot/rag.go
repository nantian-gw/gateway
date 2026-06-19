package chatbot

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"golang.org/x/sync/errgroup"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// BuildRAGContext queries the Kubernetes API for the current Gateway API topology
// and returns a concise Markdown-formatted string suitable for prepending to the
// LLM system prompt as dynamic topology context.
//
// It includes:
//   - Managed Gateways (those whose GatewayClass is controlled by controllerName)
//     with their listeners (port, protocol, hostname).
//   - HTTPRoutes and GRPCRoutes (namespace/name, rules, backendRefs).
//   - Available Services (namespace/name, ports).
func BuildRAGContext(ctx context.Context, cl client.Client, controllerName string) (string, error) {
	var (
		gcList   gatewayv1.GatewayClassList
		gwList   gatewayv1.GatewayList
		httpList gatewayv1.HTTPRouteList
		grpcList gatewayv1.GRPCRouteList
		svcList  corev1.ServiceList
	)

	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return cl.List(ctx, &gcList) })
	g.Go(func() error { return cl.List(ctx, &gwList) })
	g.Go(func() error { return cl.List(ctx, &httpList) })
	g.Go(func() error { return cl.List(ctx, &grpcList) })
	g.Go(func() error { return cl.List(ctx, &svcList) })

	if err := g.Wait(); err != nil {
		return "", fmt.Errorf("rag: list resources: %w", err)
	}

	managedClasses := make(map[string]bool, len(gcList.Items))
	for _, gc := range gcList.Items {
		if string(gc.Spec.ControllerName) == controllerName {
			managedClasses[gc.Name] = true
		}
	}

	if len(managedClasses) == 0 {
		return "No managed GatewayClasses found for controller " + controllerName, nil
	}

	var managedGateways []gatewayv1.Gateway
	for _, gw := range gwList.Items {
		if managedClasses[string(gw.Spec.GatewayClassName)] {
			managedGateways = append(managedGateways, gw)
		}
	}

	// ── Format output ────────────────────────────────────────────

	var b strings.Builder

	b.WriteString("## Current Gateway API Topology\n\n")

	// Gateways
	b.WriteString("### Gateways\n\n")
	if len(managedGateways) == 0 {
		b.WriteString("(none)\n\n")
	} else {
		for _, gw := range managedGateways {
			fmt.Fprintf(&b, "- **%s/%s** (class: %s)\n", gw.Namespace, gw.Name, gw.Spec.GatewayClassName)
			if len(gw.Spec.Listeners) > 0 {
				b.WriteString("  Listeners:\n")
				for _, l := range gw.Spec.Listeners {
					hostname := "-"
					if l.Hostname != nil {
						hostname = string(*l.Hostname)
					}
					fmt.Fprintf(&b, "    - `%s`: port=%d proto=%s hostname=%s\n",
						l.Name, l.Port, l.Protocol, hostname)
				}
			}
		}
		b.WriteString("\n")
	}

	// HTTPRoutes
	b.WriteString("### HTTPRoutes\n\n")
	if len(httpList.Items) == 0 {
		b.WriteString("(none)\n\n")
	} else {
		for _, r := range httpList.Items {
			fmt.Fprintf(&b, "- **%s/%s**", r.Namespace, r.Name)
			fmtRouteParents(&b, r.Spec.ParentRefs, r.Namespace)
			b.WriteString("\n")
			for i, rule := range r.Spec.Rules {
				if len(rule.Matches) > 0 {
					for _, m := range rule.Matches {
						path := "/"
						if m.Path != nil && m.Path.Value != nil {
							path = *m.Path.Value
						}
						fmt.Fprintf(&b, "  - rule[%d] match: path=%s\n", i, path)
					}
				} else {
					fmt.Fprintf(&b, "  - rule[%d] match: (all)\n", i)
				}
				for _, br := range rule.BackendRefs {
					ns := r.Namespace
					if br.Namespace != nil {
						ns = string(*br.Namespace)
					}
					port := int32(0)
					if br.Port != nil {
						port = int32(*br.Port)
					}
					fmt.Fprintf(&b, "    → backend: %s/%s (port=%d)\n",
						string(br.Name), ns, port)
				}
			}
		}
		b.WriteString("\n")
	}

	// GRPCRoutes
	b.WriteString("### GRPCRoutes\n\n")
	if len(grpcList.Items) == 0 {
		b.WriteString("(none)\n\n")
	} else {
		for _, r := range grpcList.Items {
			fmt.Fprintf(&b, "- **%s/%s**", r.Namespace, r.Name)
			fmtRouteParents(&b, r.Spec.ParentRefs, r.Namespace)
			b.WriteString("\n")
			for i, rule := range r.Spec.Rules {
				for _, m := range rule.Matches {
					if m.Method != nil {
						svc := "*"
						if m.Method.Service != nil {
							svc = *m.Method.Service
						}
						method := "*"
						if m.Method.Method != nil {
							method = *m.Method.Method
						}
						fmt.Fprintf(&b, "  - rule[%d] match: service=%s method=%s\n", i, svc, method)
					} else {
						fmt.Fprintf(&b, "  - rule[%d] match: (all)\n", i)
					}
				}
				if len(rule.Matches) == 0 {
					fmt.Fprintf(&b, "  - rule[%d] match: (all)\n", i)
				}
				for _, br := range rule.BackendRefs {
					ns := r.Namespace
					if br.Namespace != nil {
						ns = string(*br.Namespace)
					}
					port := int32(0)
					if br.Port != nil {
						port = int32(*br.Port)
					}
					fmt.Fprintf(&b, "    → backend: %s/%s (port=%d)\n",
						string(br.Name), ns, port)
				}
			}
		}
		b.WriteString("\n")
	}

	// Services
	b.WriteString("### Services\n\n")
	if len(svcList.Items) == 0 {
		b.WriteString("(none)\n\n")
	} else {
		for _, svc := range svcList.Items {
			fmt.Fprintf(&b, "- **%s/%s** (type=%s)", svc.Namespace, svc.Name, svc.Spec.Type)
			if len(svc.Spec.Ports) > 0 {
				b.WriteString(" [")
				for j, port := range svc.Spec.Ports {
					if j > 0 {
						b.WriteString(", ")
					}
					fmt.Fprintf(&b, "%d/%s", port.Port, port.Protocol)
				}
				b.WriteString("]")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}

func fmtRouteParents(b *strings.Builder, refs []gatewayv1.ParentReference, defaultNS string) {
	for j, ref := range refs {
		if j > 0 {
			b.WriteString(", ")
		}
		ns := defaultNS
		if ref.Namespace != nil {
			ns = string(*ref.Namespace)
		}
		fmt.Fprintf(b, " → %s/%s", ns, ref.Name)
	}
}
