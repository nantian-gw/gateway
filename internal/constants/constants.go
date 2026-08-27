// Package constants provides shared string constants used across the gateway codebase.
// These are extracted to satisfy goconst lint and to avoid scattered string literals.
package constants

// Kubernetes resource kind constants
const (
	KubeService        = "Service"
	KubeSecret         = "Secret"
	KubeConfigMap      = "ConfigMap"
	KubeServiceImport  = "ServiceImport"
	KubeNamespace      = "namespace"
	KubeGateway        = "Gateway"
	KubeHTTPRoute      = "HTTPRoute"
	KubeResource       = "resource"
	KubeReferenceGrant = "ReferenceGrant"
)

// Protocol constants
const (
	ProtocolHTTP  = "HTTP"
	ProtocolGRPC  = "GRPC"
	ProtocolTCP   = "TCP"
	ProtocolUDP   = "UDP"
	ProtocolTLS   = "TLS"
	ProtocolHTTPS = "HTTPS"
)

// String constants
const (
	StrTrue         = "true"
	StrFalse        = "false"
	StrExtensionRef = "ExtensionRef"
	StrAccepted     = "Accepted"
	StrProgrammed   = "Programmed"
)

// HTTP path constants
const (
	PathLivez  = "/livez"
	PathReadyz = "/readyz"
)

// Content type constants
const (
	ContentTypeJSON          = "application/json"
	AuthBearerWhenConfigured = "bearer-when-configured"
)

// Label constants
const (
	LabelApp = "app"
)

// Name constants
const (
	NameSnapshotSyncer = "snapshot-syncer"
)

// Status message constants
const (
	MsgListenerResolved  = "Listener references are resolved"
	MsgGatewayProgrammed = "Gateway is programmed"
	MsgExtensionResolved = "ExtensionRef was resolved"
)

// Common string constants for admin/metrics/status packages
const (
	StrDefault     = "default"
	StrTrueCapital = "True"
	StrName        = "name"
	StrKind        = "kind"
	StrService     = "service"
	StrRoute       = "route"
	StrBackend     = "backend"
	StrHTTP        = "http"
)

// Duration constants
const (
	DefaultTimeout = "30s"
)

// Controller constants
const (
	NameDataplane                    = "nantian-gw-dataplane"
	FullSnapshotRebuild              = "full snapshot rebuild when dependency lookup cannot use the index"
	SnapshotRelevantAnnotationPrefix = "gateway.nantian.dev/"
)
