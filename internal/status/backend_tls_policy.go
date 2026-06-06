package status

import (
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	backendTLSPolicyConditionResolvedRefs  = "ResolvedRefs"
	backendTLSPolicyReasonResolvedRefs     = "ResolvedRefs"
	backendTLSPolicyReasonInvalidCACertRef = "InvalidCACertificateRef"
	backendTLSPolicyReasonInvalidKind      = "InvalidKind"
	backendTLSPolicyReasonNoValidCACert    = "NoValidCACertificate"
)

type backendTLSPolicyEvaluation struct {
	ancestors []gatewayv1.PolicyAncestorStatus
}

type backendTLSPolicySpecEvaluation struct {
	generation        int64
	valid             bool
	claimKeys         []string
	targetBackendKeys []string
	fallbackAncestors []gatewayv1.ParentReference
	acceptedCondition conditionSpec
	resolvedCondition conditionSpec
}
