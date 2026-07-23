package shared

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/internal/ir"
)

func ListenerStatusSummary(statuses []gatewayv1.ListenerStatus, name gatewayv1.SectionName) *ir.ListenerStatus {
	for _, item := range statuses {
		if item.Name != name {
			continue
		}

		out := &ir.ListenerStatus{
			AttachedRoutes: int(item.AttachedRoutes),
			Conditions:     ConvertConditions(item.Conditions),
		}
		out.Accepted = FindConditionSummary(out.Conditions, string(gatewayv1.ListenerConditionAccepted))
		out.Programmed = FindConditionSummary(out.Conditions, string(gatewayv1.ListenerConditionProgrammed))
		out.ResolvedRefs = FindConditionSummary(out.Conditions, string(gatewayv1.ListenerConditionResolvedRefs))
		if out.AttachedRoutes == 0 && len(out.Conditions) == 0 {
			return nil
		}
		return out
	}

	return nil
}

func RouteStatusSummary(parents []gatewayv1.RouteParentStatus, defaultNamespace string) *ir.RouteStatus {
	if len(parents) == 0 {
		return nil
	}

	out := &ir.RouteStatus{
		Parents: make([]ir.RouteParentStatus, 0, len(parents)),
	}
	for _, item := range parents {
		parent := ir.RouteParentStatus{
			ControllerName: string(item.ControllerName),
			ParentRef:      routeParentRef(item.ParentRef, defaultNamespace),
			Conditions:     ConvertConditions(item.Conditions),
		}
		parent.Accepted = FindConditionSummary(parent.Conditions, string(gatewayv1.RouteConditionAccepted))
		parent.ResolvedRefs = FindConditionSummary(parent.Conditions, string(gatewayv1.RouteConditionResolvedRefs))
		out.Parents = append(out.Parents, parent)
	}

	return out
}

func ConvertConditions(conditions []metav1.Condition) []ir.ConditionStatus {
	if len(conditions) == 0 {
		return nil
	}

	out := make([]ir.ConditionStatus, 0, len(conditions))
	for _, condition := range conditions {
		out = append(out, ir.ConditionStatus{
			Type:               condition.Type,
			Status:             string(condition.Status),
			Reason:             condition.Reason,
			Message:            condition.Message,
			ObservedGeneration: condition.ObservedGeneration,
			LastTransitionTime: condition.LastTransitionTime.UTC(),
		})
	}

	return out
}

func FindConditionSummary(conditions []ir.ConditionStatus, target string) *ir.ConditionStatus {
	for _, condition := range conditions {
		if condition.Type != target {
			continue
		}
		item := condition
		return &item
	}
	return nil
}

func routeParentRef(parent gatewayv1.ParentReference, defaultNamespace string) ir.ParentRef {
	out := ir.ParentRef{
		Name:        string(parent.Name),
		SectionName: summaryStringValue(parent.SectionName),
	}
	if parent.Group != nil {
		out.Group = string(*parent.Group)
	}
	if parent.Kind != nil {
		out.Kind = string(*parent.Kind)
	}
	if parent.Namespace != nil {
		out.Namespace = string(*parent.Namespace)
	} else {
		out.Namespace = defaultNamespace
	}
	if parent.Port != nil {
		out.Port = uint32(*parent.Port) //nolint:gosec
	}
	return out
}

func summaryStringValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
