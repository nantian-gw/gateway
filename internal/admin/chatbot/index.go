package chatbot

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kindGateway         = "Gateway"
	kindHTTPRoute       = "HTTPRoute"
	kindGRPCRoute       = "GRPCRoute"
	kindTLSRoute        = "TLSRoute"
	kindTCPRoute        = "TCPRoute"
	kindUDPRoute        = "UDPRoute"
	kindService         = "Service"
	kindAIService       = "AIService"
	kindTokenPolicy     = "TokenPolicy"
	kindWasmPlugin      = "WasmPlugin"
	kindBackendLBPolicy = "BackendLBPolicy"
)

// ResourceRef identifies one cluster resource in the index.
type ResourceRef struct {
	Kind      string
	Namespace string
	Name      string
}

func (r ResourceRef) String() string {
	return fmt.Sprintf("%s %s/%s", r.Kind, r.Namespace, r.Name)
}

// IndexEntry is the lightweight index-layer record for one resource.
type IndexEntry struct {
	Ref           ResourceRef
	Summary       string // one-line spec digest
	StatusSummary string // condensed conditions
	Abnormal      bool   // status is not a healthy/ready state
}

// ClusterIndex holds the lightweight index plus the source objects retained
// from the List pass, so renderContext can format details without re-fetching.
type ClusterIndex struct {
	Entries []IndexEntry
	objects map[ResourceRef]client.Object
}
