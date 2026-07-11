package chatbot

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// summarizeConditions renders conditions as "Type=Status(Reason)" and reports
// whether any indicates an abnormal state. A condition is abnormal when its
// Status is not True. This is correct for positive-polarity Gateway API
// conditions (Accepted, Programmed, Ready, ResolvedRefs).
func summarizeConditions(conds []metav1.Condition) (string, bool) {
	if len(conds) == 0 {
		return "", false
	}
	parts := make([]string, 0, len(conds))
	abnormal := false
	for _, c := range conds {
		if c.Status != metav1.ConditionTrue {
			abnormal = true
			parts = append(parts, fmt.Sprintf("%s=%s(%s)", c.Type, c.Status, c.Reason))
		} else {
			parts = append(parts, fmt.Sprintf("%s=%s", c.Type, c.Status))
		}
	}
	return strings.Join(parts, ", "), abnormal
}

// summarizeRouteParents summarizes per-parent route status (.status.parents[]).
func summarizeRouteParents(parents []gatewayv1.RouteParentStatus) (string, bool) {
	if len(parents) == 0 {
		return "", false
	}
	var sb strings.Builder
	anyAbnormal := false
	for i, p := range parents {
		s, ab := summarizeConditions(p.Conditions)
		if ab {
			anyAbnormal = true
		}
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(fmt.Sprintf("parent[%s]: %s", p.ParentRef.Name, s))
	}
	return sb.String(), anyAbnormal
}

// summarizeAncestors summarizes per-ancestor policy status (.status.ancestors[]).
func summarizeAncestors(ancestors []gatewayv1.PolicyAncestorStatus) (string, bool) {
	if len(ancestors) == 0 {
		return "", false
	}
	var sb strings.Builder
	anyAbnormal := false
	for i, a := range ancestors {
		s, ab := summarizeConditions(a.Conditions)
		if ab {
			anyAbnormal = true
		}
		if i > 0 {
			sb.WriteString("; ")
		}
		sb.WriteString(fmt.Sprintf("ancestor[%s]: %s", a.AncestorRef.Name, s))
	}
	return sb.String(), anyAbnormal
}
