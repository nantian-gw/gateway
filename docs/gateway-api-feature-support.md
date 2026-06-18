# Gateway API v1.5.1 Feature Support

This page summarizes Nantian Gateway's current support status for upstream Gateway API v1.5.1 features that are either:

- part of an upstream `ExtendedFeatures` set, or
- marked with upstream `experimental` channel metadata.

It is intentionally narrower than a full product compatibility guide. Core features such as `Gateway`, `HTTPRoute`, `Mesh`, `ReferenceGrant`, and the base `TLSRoute` feature are not repeated here.

## How To Read This Matrix

- `Default Support` means the feature is advertised by a default runtime where `enableExperimentalGateway=false`.
- `Experimental Runtime Support` means the feature is advertised when `enableExperimentalGateway=true`.
- `Current Nantian Position` summarizes Nantian Gateway's advertised/runtime-gated support position in this repository, derived from:
  - `internal/gatewayapi/supported_features.go`
  - `conformance/conformance_test.go`
  - `.github/workflows/conformance.yml`
  - `scripts/ci/deploy-kind-conformance.sh`

For conformance runs, `ALL_FEATURES=true` expands the suite to the repository-supported feature set, and `CONFORMANCE_EXPERIMENTAL=true` enables the experimental Gateway runtime used by the all-features workflow.

## Support Matrix

| Feature | Upstream Group | Upstream Channel | Default Support | Experimental Runtime Support | Current Nantian Position |
|---|---|---|---|---|---|
| GatewayPort8080 | Gateway ext | standard | Yes | Yes | Supported |
| GatewayStaticAddresses | Gateway ext | standard | Yes | Yes | Supported |
| GatewayHTTPListenerIsolation | Gateway ext | standard | No | No | Not advertised pending conformance |
| GatewayHTTPSListenerDetectMisdirectedRequests | Gateway ext | standard | Yes | Yes | Supported |
| GatewayInfrastructurePropagation | Gateway ext | standard | Yes | Yes | Supported |
| GatewayAddressEmpty | Gateway ext | standard | Yes | Yes | Supported |
| ListenerSet | Gateway ext | standard | No | Yes | Experimental runtime only |
| GatewayBackendClientCertificate | Gateway ext | standard | Yes | Yes | Supported |
| GatewayFrontendClientCertificateValidation | Gateway ext | standard | No | No | Not supported |
| GatewayFrontendClientCertificateValidationInsecureFallback | Gateway ext | standard | No | No | Not supported |
| GRPCRouteNamedRouteRule | GRPCRoute ext | standard | No | No | Not advertised pending conformance |
| HTTPRouteDestinationPortMatching | HTTPRoute ext | experimental | Yes | Yes | Supported |
| HTTPRouteBackendRequestHeaderModification | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteQueryParamMatching | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteMethodMatching | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteResponseHeaderModification | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRoutePortRedirect | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteSchemeRedirect | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRoutePathRedirect | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteHostRewrite | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRoutePathRewrite | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteRequestMirror | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteRequestMultipleMirrors | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteRequestPercentageMirror | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteRequestTimeout | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteBackendTimeout | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteParentRefPort | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteBackendProtocolH2C | HTTPRoute ext | standard | No | No | Not advertised pending conformance |
| HTTPRouteBackendProtocolWebSocket | HTTPRoute ext | standard | No | No | Not advertised pending conformance |
| HTTPRouteNamedRouteRule | HTTPRoute ext | standard | Yes | Yes | Supported |
| HTTPRouteCORS | HTTPRoute ext | standard | No | No | Not advertised pending conformance |
| HTTPRoute303RedirectStatusCode | HTTPRoute ext | standard | No | No | Not advertised pending conformance |
| HTTPRoute307RedirectStatusCode | HTTPRoute ext | standard | No | No | Not advertised pending conformance |
| HTTPRoute308RedirectStatusCode | HTTPRoute ext | standard | No | No | Not advertised pending conformance |
| TLSRouteModeTerminate | TLSRoute ext | standard | No | Yes | Experimental runtime only |
| TLSRouteModeMixed | TLSRoute ext | experimental | No | Yes | Experimental runtime only |
| MeshClusterIPMatching | Mesh ext | standard | Yes | Yes | Supported |
| MeshConsumerRoute | Mesh ext | standard | Yes | Yes | Supported |
| MeshHTTPRouteRewritePath | Mesh ext | standard | Yes | Yes | Supported |
| MeshHTTPRouteSchemeRedirect | Mesh ext | standard | Yes | Yes | Supported |
| MeshHTTPRouteRedirectPort | Mesh ext | standard | Yes | Yes | Supported |
| MeshHTTPRouteRedirectPath | Mesh ext | standard | Yes | Yes | Supported |
| MeshHTTPRouteBackendRequestHeaderModification | Mesh ext | standard | Yes | Yes | Supported |
| MeshHTTPRouteQueryParamMatching | Mesh ext | standard | Yes | Yes | Supported |
| MeshHTTPRouteNamedRouteRule | Mesh ext | standard | Yes | Yes | Supported |
| BackendTLSPolicySANValidation | BackendTLSPolicy ext | standard | No | No | Not advertised pending conformance |
| UDPRoute | UDPRoute | experimental | No | Yes | Experimental runtime only |

## Notes

- `ListenerSet`, `TLSRouteModeTerminate`, `TLSRouteModeMixed`, and `UDPRoute` require `enableExperimentalGateway=true` even though some of them are upstream standard-channel features. That runtime gate reflects Nantian Gateway's current implementation boundary, not just the upstream channel label.
- `HTTPRouteDestinationPortMatching` is an upstream experimental-channel feature, but Nantian Gateway currently advertises it in the default runtime.
- `BackendTLSPolicy`, `GRPCRoute`, and their listed extension features are not advertised until their upstream conformance probes pass.
- The two upstream features that remain unsupported are:
  - `GatewayFrontendClientCertificateValidation`
  - `GatewayFrontendClientCertificateValidationInsecureFallback`

## Nantian-Specific Declarations Outside The Upstream Matrix

Nantian Gateway also declares:

- `TCPRoute`
- `BackendLBSessionPersistence`

These names are part of Nantian Gateway's own supported-feature declarations, but they are not part of the upstream Gateway API v1.5.1 `pkg/features` support matrix summarized on this page.

## Source Of Truth In This Repository

- Upstream group/channel metadata: `sigs.k8s.io/gateway-api@v1.5.1/pkg/features/*.go`
- Supported feature declarations: `internal/gatewayapi/supported_features.go`
- All-features conformance expansion: `conformance/conformance_test.go`
- All-features workflow environment: `.github/workflows/conformance.yml`
- Runtime experimental enablement for Kind conformance: `scripts/ci/deploy-kind-conformance.sh`
