# Gateway API GEP Completion Tracker

**Purpose:** Track the implementation, declaration, testing, and documentation closure of this project item by item according to Gateway API upstream GEPs.

**Upstream Source:** <https://github.com/kubernetes-sigs/gateway-api/tree/main/geps>

**Sync source commit:** `ec7d5a2f6ff132f2b2465aff81dfcba862738a9c`

**Sync date:** 2026-05-13

## Usage Rules

- The `Status` in this document is the execution status of this project, not the upstream GEP status.
- All initial statuses are marked as `[ ] Pending`. After completing a GEP's in-project closure, change the corresponding entry to `[x] Completed` and add evidence links under that entry.
- For GEPs with upstream status `Declined`, `Memorandum`, or `Completed`, `Completed` does not necessarily mean implementing runtime code; it can also mean completing an in-project applicability assessment and clearly documenting the conclusion of “not applicable / not supported / already covered by another GEP” in the support matrix, backlog, or ADR.
- For GEPs with upstream status `Standard`, `Experimental`, `Provisional`, or `Implementable`, unless explicitly recorded as not applicable, the completion criteria must at least include consistency checks across control plane, data plane, status, documentation, testing, and support declarations.

## General Acceptance Criteria

Each GEP's in-project acceptance should at minimum check the following:

- API surface: Whether the corresponding fields, resources, feature names, and CRD tracks exist in the current Gateway API dependency version, and whether upgrading `sigs.k8s.io/gateway-api` is needed.
- Control plane: Whether watch, index, translator, status, policy/reference resolution, and unsupported value behavior are complete.
- Data plane: Whether IR/proto delivery, Rust proxy HTTP/stream runtime behavior, hot reload, and error paths are complete.
- Support declaration: Whether `GatewayClass.status.supportedFeatures`, `docs/gateway-api-support.md`, experimental backlog, and conformance declarations are consistent.
- Test evidence: Unit tests, targeted validation, kind/e2e, conformance, or a clear explanation of why not needed.
- Failure semantics: When unsupported or partially supported, the status, documentation, and tests must explicitly reflect this, not silently ignore.

## GEP List

### GEP-91: Client Certificate Validation for TLS terminating at the Gateway Listener

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-91/index.md>
- FeatureNames: `GatewayFrontendClientCertificateValidation`, `GatewayFrontendClientCertificateValidationInsecureFallback`
- Summary: Adds frontend client certificate validation for Gateway listener TLS termination, including CA trust anchor, required/optional client cert modes, and the relationship to HTTP/2 connection coalescing risks.
- Acceptance Criteria: Listener TLS frontend validation fields are correctly parsed and delivered; CA references, cross-namespace references, invalid certificates, and fallback modes have status and tests; data plane actually performs client certificate validation; support matrix and supportedFeatures are consistent with evidence.
- Completion Evidence: `GatewayFrontendClientCertificateValidation` and `GatewayFrontendClientCertificateValidationInsecureFallback` are declared in `GatewayClass.status.supportedFeatures`; support matrix recorded as `declared=yes, implemented=yes, tested=targeted conformance + unit`; control plane covers `Gateway.spec.tls.frontend` parsing, CA ref validation, ReferenceGrant, `InsecureFrontendValidationMode` status, and translator IR; data plane covers shared TLS handshake, strict client certificate, `AllowInsecureFallback`, and listener reload; in the archived conformance `reports/conformance/runs/2026-05-11-36d81124-full-fixed-conformance/run.log`, `GatewayFrontendClientCertificateValidation`, `GatewayFrontendClientCertificateValidationInsecureFallback`, and `GatewayInvalidFrontendClientCertificateValidation` are all PASS.

### GEP-696: GEP template

- Status: [x] Completed
- Upstream Status: Completed
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-696/index.md>
- FeatureNames: None
- Summary: Gateway API GEP documentation template defining the structure a GEP should contain, including TLDR, Goals, API, Conformance Details, etc.
- Acceptance Criteria: Confirm whether the project's GEP tracking and proposal documents need to follow this structure; if no runtime code is involved, record the conclusion of “not applicable for code implementation.”
- Completion Evidence: This GEP is an upstream documentation template and does not correspond to a Gateway API runtime feature. The project's long-term design entry is handled by `docs/proposals/README.md` and `docs/proposals/template.md`, with the template coveringbackground, solution boundaries, compatibility and operational impact, validation plan, rollback and alternatives; this tracker also records original URL, status, featureNames, and acceptance criteria per GEP metadata. No control plane or data plane code modifications are needed.

### GEP-709: Cross Namespace References from Routes

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-709/index.md>
- FeatureNames: None
- Summary: Introduces `ReferenceGrant`, allowing the owner of a referenced namespace to explicitly permit Routes to reference Services, Secrets, and other objects across namespaces.
- Acceptance Criteria: All Route backendRefs and related object references are authorized via `ReferenceGrant`; unauthorized references have status `ResolvedRefs=False`; the translator does not deliver invalid references; covers HTTP/GRPC/TCP/UDP/TLS and mesh/service parent scenarios.
- Completion Evidence: `ReferenceGrant` is declared as implemented in `GatewayClass.status.supportedFeatures` and `docs/gateway-api-support.md`; the control plane covers cross-namespace authorization for Route -> Service backends, applicable to `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, `TLSRoute`, and covers cross-namespace references for Gateway -> Secret `certificateRefs`, Gateway frontend CA `ConfigMap`, and `BackendTLSPolicy`/Gateway backendTLS client certificate `Secret` within the current declared support scope. Unauthorized references write `ResolvedRefs=False, Reason=RefNotPermitted`; the translator skips unauthorized certificates/CA/client certificates or writes unavailable metadata for invalid backends without delivering valid backends; the controller/status side indexes by reference target namespace and refreshes relevant Routes/Gateways when `ReferenceGrant` changes. Unit tests cover status, translator, and controller scoped rebuild/index contracts; in the archived conformance `reports/conformance/runs/2026-04-29-93edb22-dirty-full/run.log`, `GatewaySecret*ReferenceGrant`, `HTTPRoute*ReferenceGrant`, `HTTPRouteInvalidCrossNamespaceBackendRef`, and `TLSRouteInvalidReferenceGrant` related test cases are all PASS.

### GEP-713: Metaresources and Policy Attachment

- Status: [x] Completed
- Upstream Status: Memorandum
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-713/index.md>
- FeatureNames: None
- Summary: Defines the general pattern for Gateway API metaresource and policy attachment, including direct/inherited policy, `targetRef`, section attachment, conflict resolution, and status expression.
- Acceptance Criteria: All policy CRDs and extension policies within the project conform to targetRef, status conditions, conflict handling, and naming conventions; unused policy attachment patterns are clearly documented in the backlog.
- Completion Evidence: This GEP is an upstream policy attachment design memorandum and does not correspond to an independent supportedFeature. The repository's current policy surface is limited to direct backend policy attachment: `BackendTLSPolicy` uses standard `targetRefs`/`status.ancestors[*].conditions`; `BackendLBPolicy` as an experimental/repo subset uses similar targetRef and ancestor status patterns; `docs/status-matrix.md` explicitly only covers these two Policy types, recording reasons such as `Accepted`, `ResolvedRefs`, `Conflicted`, `TargetNotFound`, and `InvalidKind`. The control plane handles conflicts through policy targetRef field index, target resolution, creationTimestamp/name precedence, and section-scoped BackendTLS priority, with the translator only delivering effective policies. The support matrix and experimental backlog clarify that `BackendLBPolicy` is not a full upstream support declaration; inherited policies, additional Policy types, and unaudited fields are not within the current completion scope.

### GEP-718: Rework forwardTo segment in routes

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-718/index.md>
- FeatureNames: None
- Summary: Consolidates the old `forwardTo`/`serviceName` shorthand into a more consistent `backendRefs` model, laying the foundation for cross-namespace backends and general backend types.
- Acceptance Criteria: Translator/status only relies on current `backendRefs` semantics; weight, port, kind/group/namespace, invalid backend status, and old field compatibility are all explicitly handled.
- Completion Evidence: The current repository depends on Gateway API `v1.5.1`, and the Route API surface is already the `backendRefs` model; there are no old `forwardTo` field handling paths in the code. The translator uniformly processes through `controlplane/internal/translator/backend_refs.go` processes backend refs for `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, and `TLSRoute`, preserving group/kind/namespace/name/port/weight/filter information, and writes invalid backend metadata for cross-namespace unauthorized, unknown kind, missing Service/ServiceImport, or port issues. The status evaluator correspondingly writes `ResolvedRefs=False` with `RefNotPermitted`, `InvalidKind`, or `BackendNotFound`; the support matrix explicitly only declares Service backendRef, ServiceImport backend, and the current weight semantics, not declaring support for unknown backend target types. The data plane IR preserves weights and covers HTTP/GRPC/stream weighted selection, with `weight=0` being skipped.

### GEP-724: Refresh Route-Gateway Binding

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-724/index.md>
- FeatureNames: None
- Summary: Restructures the Route-to-Gateway binding model, using Route `parentRefs` and Gateway listener `allowedRoutes`, and requires listeners to have names to support section binding.
- Acceptance Criteria: ParentRef, SectionName, Port, AllowedRoutes namespace/kind selector, listener attachment, Route parent status, and Gateway attachedRoutes are fully consistent.
- Completion Evidence: `docs/gateway-api-support.md` declares Gateway listener `AllowedRoutes.kinds`, `AllowedRoutes.namespaces`, and Route `parentRefs` pointing to Gateway by `name/namespace/sectionName/port` binding as implemented; `controlplane/internal/translator/attachments.go` computes attachment by listener name, port, protocol default route kinds, explicit `AllowedRoutes.kinds`, `Same/All/Selector` namespace policy, and hostname intersection, and writes to listener `AttachedRoutes`. On the status side, it writes back Route parent `Accepted=True/False`, `NotAllowedByListeners`, does not produce an accepted parent when no matching parent exists, and synchronizes Gateway listener `attachedRoutes`; HTTP/GRPC/TCP/UDP/TLS routes all follow the same binding model. In the archived full conformance `reports/conformance/runs/2026-05-08-3af22b42-full/run.log`, `GatewayWithAttachedRoutes`, `GatewayWithAttachedRoutesWithPort8080`, `HTTPRouteInvalidCrossNamespaceParentRef`, `HTTPRouteInvalidParentRefNotMatchingListenerPort`, `HTTPRouteInvalidParentRefNotMatchingSectionName`, `HTTPRouteInvalidParentRefSectionNameNotMatchingPort` are all PASS.

### GEP-726: Add Path Redirects and Rewrites

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-726/index.md>
- FeatureNames: None
- Summary: Adds standard HTTP filter behaviors such as path redirect, path prefix rewrite, and host rewrite for `HTTPRoute`.
- Acceptance Criteria: `RequestRedirect` and `URLRewrite` hostname/path/statusCode semantics are complete; mutually exclusive filter combinations are rejected; data plane behavior is consistent with status and conformance.
- Completion Evidence: Related features are declared in `GatewayClass.status.supportedFeatures`: `HTTPRoutePathRedirect`, `HTTPRoutePathRewrite`, `HTTPRouteHostRewrite`, `HTTPRouteSchemeRedirect`, `HTTPRoutePortRedirect`, `HTTPRoute303RedirectStatusCode`, `HTTPRoute307RedirectStatusCode`, `HTTPRoute308RedirectStatusCode`. The control plane translates `RequestRedirect`'s `scheme/hostname/port/statusCode/path` and `URLRewrite`'s `hostname/path` to IR, preserving Gateway API's 303/307/308 status codes; `ValidateHTTPRouteRules` rejects a single rule containing both `RequestRedirect` and `URLRewrite`, writing `ResolvedRefs=False, Reason=UnsupportedValue` when entirely invalid, and `PartiallyInvalid=True` when partially invalid while discarding the bad rule. The data plane `aeg-http` generates redirect `Location`, performs full path / prefix path rewrite while preserving query; `aeg-ir` can decode redirect/rewrite filters from proto. In the archived full conformance `reports/conformance/runs/2026-05-08-3af22b42-full/run.log`, `HTTPRouteRedirectHostAndStatus`, `HTTPRouteRedirectPath`, `HTTPRouteRedirectPortAndScheme`, `HTTPRouteRedirectPort`, `HTTPRouteRedirectScheme`, `HTTPRouteRewriteHost`, `HTTPRouteRewritePath` are all PASS.

### GEP-735: TCP and UDP addresses matching

- Status: [x] Completed
- Upstream Status: Declined
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-735/index.md>
- FeatureNames: None
- Summary: Previously proposed adding source/destination address matching for TCPRoute/UDPRoute, but upstream has declined.
- Acceptance Criteria: The project explicitly does not implement this declined API; if custom address matching capabilities exist, they must be marked as implementation-specific extensions and not declared as Gateway API support.
- Completion Evidence: Upstream has declined this GEP; the repository does not declare or implement TCPRoute/UDPRoute source address or destination address matching fields. `docs/gateway-api-support.md` clarifies the current stream route boundary as forwarding by listener / parentRef / TLS SNI / backendRef, with individual `TCPRoute` / `UDPRoute` rules not adding extra L4 address matching conditions; there are no `sourceAddresses`, `destinationAddresses`, `AddressRouteMatches`, or `AddressMatch` paths in the control plane, proto, or data plane. The existing `SourceIP` belongs only to the repo-specific `BackendLBPolicy.loadBalancing.consistentHash` hash key, not TCP/UDP route address matching, and is not declared as support for this GEP.

### GEP-746: Replace Cert Refs on HTTPRoute with Cross Namespace Refs from Gateway

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-746/index.md>
- FeatureNames: None
- Summary: Removes certificate references on HTTPRoute, consolidates TLS certificate configuration to Gateway listener, and supports certificate delegation through a cross-namespace reference mechanism.
- Acceptance Criteria: Gateway listener `certificateRefs` is the only supported entry point; cross-namespace Secret references are controlled by ReferenceGrant; HTTPRoute cert refs are not accepted or delivered.
- Completion Evidence: The current repository uses `Gateway.spec.listeners[*].tls.certificateRefs` from Gateway API `v1.5.1` as the only entry point for HTTPS/TLS certificates; the HTTPRoute translator/status has no certificateRef handling path. `docs/gateway-api-support.md` declares the listener `certificateRefs -> Secret` current support subset and cross-namespace `certificateRefs + ReferenceGrant`; the control plane only supports `group=""`, `kind=Secret`, validates certificate Secrets, filters unauthorized/missing/invalid references, and exposes bad references in status as `ResolvedRefs=False`; the translator only delivers valid Secret material to snapshots. Unit tests cover cross-namespace Secret rejection without ReferenceGrant, acceptance with ReferenceGrant, and invalid/missing certificate status; in the archived full conformance `reports/conformance/runs/2026-05-08-3af22b42-full/run.log`, `GatewayInvalidTLSConfiguration` and `GatewaySecret*ReferenceGrant` related test cases are all PASS.

### GEP-820: Drop extension points from Route matches

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-820/index.md>
- FeatureNames: None
- Summary: Removes unclear extension points from the Route match block, preventing match conditions from being fragmented by implementation-specific resources.
- Acceptance Criteria: Current Route match only supports standard fields; any implementation-specific match extension is not treated as upstream Gateway API support and has explicit rejection or documentation.
- Completion Evidence: Current Gateway API `v1.5.1` types no longer have old Route match extension points; the repository control plane and proto/IR have no `HTTPRouteMatch.ExtensionRef`, `TCPRouteMatch.ExtensionRef`, or other match-level extension paths. HTTPRoute match only translates hostname, path, method, headers, query params; GRPCRoute match only translates hostname, service/method, headers; TLSRoute only uses SNI hostname; TCPRoute/UDPRoute operate by listener attachment. `docs/gateway-api-support.md` explicitly marks `RegularExpression` match type as Gateway API implementation-specific field semantics, not a custom match extension; the repository's `ExtensionRef` only exists in the filter/backend filter path and is documented as a repo-specific filter extension.

### GEP-851: Allow Multiple Certificate Refs per Gateway Listener

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-851/index.md>
- FeatureNames: None
- Summary: Extends Gateway listener TLS configuration from a single `certificateRef` to multiple `certificateRefs`, supporting multiple hostnames, multiple algorithm certificates, and certificate rotation.
- Acceptance Criteria: Multi-certificate Secret loading, SNI/hostname selection, invalid certificate status, secret material delivery, and rotation tests are complete; cross-namespace certificates still follow ReferenceGrant.
- Completion Evidence: `docs/gateway-api-support.md` declares that HTTPS listener supports one or more `certificateRefs -> Secret`, with the current subset limited to `group=""`, `kind=Secret`. The control plane `listenerCertificateSecretRefsWithIndexes` filters unauthorized, missing, or invalid Secrets in declaration order and deduplicates valid Secret refs; status writes `ResolvedRefs=False` for mixed valid/invalid certificates to expose bad references, but as long as at least one certificate is valid, the listener can remain `Programmed=True`. Translator unit tests cover duplicate reference deduplication, cross-namespace ReferenceGrant, mixed valid/invalid references preserving valid order, and certificate material rotation; the data plane listener plan parses each valid Secret, extracts certificate SAN/CN, prioritizes by SNI exact/wildcard matching while maintaining fallback order, and covers secondary certificate material rotation. In the archived conformance `reports/conformance/runs/2026-05-08-3af22b42-full/run.log`, standard test cases for Gateway listener certificateRef invalidity and ReferenceGrant are PASS.

### GEP-917: Gateway API Conformance Testing

- Status: [x] Completed
- Upstream Status: Memorandum
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-917/index.md>
- FeatureNames: None
- Summary: Documents Gateway API conformance testing principles, test structure, reporting goals, and implementer usage.
- Acceptance Criteria: The project's conformance harness, report archiving, feature/profile selection, and documentation are consistent with upstream principles; missing items go into the test backlog.
- Completion Evidence: This GEP is a conformance process memorandum and does not correspond to a runtime code feature. The repository already has an official harness entry point `tests/conformance/run.sh`, supporting `RUN_TEST`, `SKIP_TESTS`, `SUPPORTED_FEATURES`, `EXEMPT_FEATURES`, `CONFORMANCE_PROFILES`, `ALL_FEATURES`, `SKIP_PROVISIONAL_TESTS`, and `REPORT_OUTPUT`, and can derive explicit supported features from `controlplane/internal/gatewayapi/supported_features.go`; it defaults to the `GATEWAY-HTTP` quick profile, while full-suite requires explicit `ALL_FEATURES=true`. Report archiving is done by `scripts/archive-conformance-report.sh` generating immutable `report.yaml`, `run.log`, `metadata.yaml`, `log-summary.json`, and `summary.md`, with `scripts/publish-conformance-reports.sh` publishing to the `conformance-reports` branch; `reports/conformance/README.md`, `docs/gateway-api-support.md`, and `docs/test/latest-baseline.md` clearly distinguish between the current `latest` mesh profile and the most recent clean full-suite baseline `reports/conformance/runs/2026-05-08-3af22b42-full/`, with gaps and scopes that should not be misinterpreted also documented.

### GEP-922: Gateway API Versioning

- Status: [x] Completed
- Upstream Status: Memorandum
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-922/index.md>
- FeatureNames: None
- Summary: Defines Gateway API bundle version, standard/experimental CRD tracks, and experimental field release and installation models.
- Acceptance Criteria: Go module, CRD bundle, kind/conformance scripts, and documentation have consistent Gateway API versions; the experimental CRD usage strategy is clear.
- Completion Evidence: This GEP is a version and release track memorandum and does not correspond to an independent data plane feature. The repository's current default Gateway API version is `v1.5.1`, pinned by `scripts/check-gateway-api-version-alignment.sh` in `controlplane/go.mod`, `tests/conformance-harness/go.mod`, `tests/conformance/run.sh`, `tests/e2e/run-kind.sh`, `scripts/run-release-validation.sh`, `scripts/audit-gateway-api-bundle.sh`, and the GatewayClass `SupportedVersion` status constant; `docs/gateway-api-support.md` clarifies the control plane depends on `v1.5.1`, with kind/conformance/release defaulting to install the same CRD version. `docs/gateway-api-version-audit.md` defines that upgrades must run bundle audit, support matrix check, and status surface audit; `scripts/audit-gateway-api-bundle.sh` validates the cluster CRD `gateway.networking.k8s.io/bundle-version` and prints GatewayClass supportedFeatures. Experimental boundaries are documented: the repository uses the upstream standard/experimental CRD track, while keeping repo-specific `BackendLBPolicy` as an additional experimental CRD maintained separately on the `v1.2.1` compatible track, not mixed into Gateway API standard support declarations.

### GEP-957: Destination Port Matching

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-957/index.md>
- FeatureNames: None
- Summary: Adds a `port` field to `ParentRef`, allowing Routes to bind by target listener port.
- Acceptance Criteria: When both ParentRef port and sectionName are specified, both must match simultaneously; status, attachment, translator, and partial rebuild all correctly handle port binding.
- Completion Evidence: `HTTPRouteParentRefPort`, `HTTPRouteDestinationPortMatching`, and `GatewayPort8080` are declared in `GatewayClass.status.supportedFeatures` and `docs/gateway-api-support.md`. The `candidateAttachmentListeners` in `controlplane/internal/translator/attachments.go` checks both `sectionName` and `port` for Gateway parentRef, and also filters Service parentRef by service frontend port; status/controller side covers route binding, Gateway listener `attachedRoutes`, `NoMatchingParent`, and scoped rebuild. Unit tests cover HTTP/GRPC/stream attachment, selector namespace + parentRef port, TCPRoute parentRef port, and status listener port binding; in the archived full conformance `reports/conformance/runs/2026-05-08-3af22b42-full/run.log`, `HTTPRouteInvalidParentRefNotMatchingListenerPort`, `HTTPRouteInvalidParentRefSectionNameNotMatchingPort`, and `HTTPRouteListenerPortMatching` are all PASS.

### GEP-995: Named route rules

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-995/index.md>
- FeatureNames: None
- Summary: Adds an optional `name` to rules in HTTPRoute, GRPCRoute, TCPRoute, TLSRoute, and UDPRoute, making it easier for policies, status, events, and observability systems to reference individual rules.
- Acceptance Criteria: Rule names are preserved in status/IR/observability metadata; duplicate or missing names are handled according to upstream semantics; policy sectionName or rule reference behavior has tests.
- Completion Evidence: `HTTPRouteNamedRouteRule`, `GRPCRouteNamedRouteRule`, and `MeshHTTPRouteNamedRouteRule` are declared in `GatewayClass.status.supportedFeatures` and `docs/gateway-api-support.md`; the current Gateway API `v1.5.1` types provide the `name` field for HTTP/GRPC/TCP/UDP/TLS rules. The control plane translator now preserves optional rule names from HTTPRoute, GRPCRoute, TCPRoute, UDPRoute, and TLSRoute in the IR; shared proto `HttpRule`, `GrpcRule`, and `StreamRule` have a backward-compatible `name` field, delivered via gRPC snapshot and preserved after dataplane `aeg-ir` proto decode. Missing rule names are still permitted per upstream semantics; duplicate/invalid names are rejected by the installed Gateway API CRD schema. The repository currently has no standard policy type that binds policy `sectionName` to Route rules, so rule names are retained for future rule-scoped policies, admin/observability, and runtime correlation. In the archived full conformance `reports/conformance/runs/2026-05-08-3af22b42-full/run.log`, `HTTPRouteNamedRule` and `GRPCRouteNamedRule` are PASS; in the current `latest` mesh profile, `MeshHTTPRouteNamedRule` is PASS.

### GEP-1016: GRPCRoute

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1016/index.md>
- FeatureNames: None
- Summary: Defines `GRPCRoute` for L7 routing based on gRPC service/method/headers.
- Acceptance Criteria: GRPCRoute match, filters, backendRefs, Gateway attachment, status, HTTP/2/gRPC data plane routing, and conformance are all covered; unsupported filters are explicitly rejected.
- Completion Evidence: `GRPCRoute` and `GRPCRouteNamedRouteRule` are declared in `GatewayClass.status.supportedFeatures` and `docs/gateway-api-support.md`. The control plane supports `GRPCRoute` Gateway/Service parentRef, hostname, service/method, header match, Service backendRef, ReferenceGrant, weight, rule/backend filters, session persistence, and basic parent status; the data plane carries gRPC over HTTP/HTTPS listeners, covering h2c, ALPN h2, unary, streaming, metadata/trailers, `grpc-timeout`, and cancel/disconnect paths. Dedicated e2e entry points include `tests/e2e/validate-grpc-reference-grants.sh` and `tests/e2e/validate-backend-protocols.sh`, with data plane selection and routing regressed by `dataplane/crates/aeg-ir/tests/grpc_route_matching.rs`, `dataplane/crates/aeg-ir/src/tests_property/grpc_selection.rs`, etc.; archived full-suite and mesh profile reports record `GRPCRoute`/`MeshGRPCRouteWeight` related test cases as passing.

### GEP-1282: Describing Backend Properties

- Status: [x] Completed
- Upstream Status: Declined
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1282/index.md>
- FeatureNames: None
- Summary: Previously attempted to define a backend properties description model, later declined, with some issues covered by subsequent GEPs like Backend Protocol Selection and BackendTLSPolicy.
- Acceptance Criteria: Do not implement declined fields; if the project already has backend property extensions, they should be mapped to subsequent standard GEPs or marked as implementation-specific capabilities.
- Completion Evidence: Upstream has declined this GEP; the repository does not declare or implement an independent backend properties API. Current backend-related capabilities fall under subsequent standard or repository extension boundaries: `ServicePort.appProtocol` corresponds to GEP-1911, `BackendTLSPolicy` to GEP-1897/GEP-3155, and `BackendLBPolicy.sessionPersistence` with repo-specific `loadBalancing.consistentHash` are separately noted in `docs/gateway-api-support.md` and `docs/backlog/gateway-api-experimental.md`. There is no path for delivering declined backend property fields to proto/IR or the data plane.

### GEP-1294: xRoutes Mesh Binding

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1294/index.md>
- FeatureNames: None
- Summary: Defines the model for xRoutes binding to mesh parents such as Service via `parentRefs`, forming the basis of GAMMA service-parent routing.
- Acceptance Criteria: Routes can bind to a Service parent; only effective within valid namespace/reference scope; mesh listener/workload delivery, status, and conformance profile coverage.
- Completion Evidence: `Mesh`, `MeshClusterIPMatching`, `MeshConsumerRoute`, and multiple `MeshHTTPRoute*` features are declared in supportedFeatures; `docs/gateway-api-support.md` clarifies that HTTP/GRPC/TCP/UDP/TLS Routes all support `parentRefs` pointing to the mesh frontend subset of `Service`. The control plane generates mesh frontend Service/EndpointSlice via `controlplane/internal/translator/mesh.go`, `controlplane/internal/infrastructure/mesh_services.go`, and route/service parent indexes, writing Service parent `Accepted/ResolvedRefs` in status; the data plane uses synthetic frontend listener/parent for selection. The latest `reports/conformance/latest/` is a kind mesh profile archive; `reports/conformance/runs/2026-05-12-0355945e-kind-mesh-profile-current/run.log` covers `MeshGRPCRouteWeight`, `MeshConsumerRoute`, `MeshHTTPRoute*` and other test cases; `tests/e2e/validate-mesh-frontends.sh` provides a supplementary e2e entry point.

### GEP-1323: Response Header Filter

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1323/index.md>
- FeatureNames: None
- Summary: Adds response header modifier filter for HTTPRoute, symmetric to the request header modifier.
- Acceptance Criteria: add/set/remove response header semantics are consistent between translator and data plane; combinations with other filters have tests; support declarations align with conformance.
- Completion Evidence: `HTTPRouteResponseHeaderModification` is declared in supportedFeatures and `docs/gateway-api-support.md`. The control plane translates rule-level and backendRef-level `ResponseHeaderModifier` into IR/proto filters, with `controlplane/internal/gatewayapi/http_route_validation.go` listing it as a supported filter; the data plane `dataplane/crates/aeg-http/src/filters/headers.rs` performs response header modification for `add/set/remove`. Unit tests cover extension/native filter parsing, proto decode, and HTTP runtime filter combinations; the support matrix records `ResponseHeaderModifier` as implemented with evidence of actual data plane modification.

### GEP-1324: Service Mesh in Gateway API

- Status: [x] Completed
- Upstream Status: Memorandum
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1324/index.md>
- FeatureNames: None
- Summary: Defines the overall problem space, roles, and GAMMA direction for Gateway API in service mesh scenarios.
- Acceptance Criteria: The boundary of the project's mesh profile, Service parent, workload discovery, east-west behavior, and unsupported items have unified documentation; subsequent specific GEPs are mapped individually.
- Completion Evidence: This GEP is a mesh direction memorandum and does not correspond to an independent runtime feature. The repository lists Service parent / mesh frontend as a limited capability with `declared=yes, implemented=yes, tested=targeted conformance + e2e + unit` in `docs/gateway-api-support.md`, and notes that multi-environment east-west long-stability evidence is still missing; `docs/test/plan.md`, `docs/test/checklist.md`, and `reports/conformance/README.md` distinguish Gateway profile, Mesh profile, and UDP/TCP/TLSRoute supplementary evidence. Subsequent specific capabilities are tracked by GEP-1294, GEP-1686, GEP-1709, GEP-3779, and GEP-3949.

### GEP-1364: Status and Conditions Update

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1364/index.md>
- FeatureNames: None
- Summary: Standardizes the type, reason, observedGeneration, and update semantics for Gateway API status conditions.
- Acceptance Criteria: GatewayClass, Gateway, Listener, Route, and Policy status follow upstream condition semantics; status updates are idempotent and convergent with observedGeneration tests.
- Completion Evidence: `docs/status-matrix.md` records the condition type, reason, description, and test anchors for `GatewayClass`, `Gateway`, Listener, Route, `BackendTLSPolicy`, and `BackendLBPolicy` within the current declared scope. The control plane uniformly writes `observedGeneration` across paths such as `controlplane/internal/status/observed_generation.go`, `reconciler_gateway_status.go`, `reconciler_route_status.go`, and `reconciler_policy_status.go`, covering reader-state generation, route parent, policy ancestor, and additional conditions via `controlplane/internal/status/object_reconciler_gateway_test.go`, `object_reconciler_route_test.go`, `reconciler_core_test.go`, and `native_filters_test.go`; the translator/admin side carries `ObservedGeneration` into the IR snapshot and `/v1/snapshot` summary.

### GEP-1494: HTTP Auth in Gateway API

- Status: [x] Completed
- Upstream Status: Experimental
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1494/index.md>
- FeatureNames: `HTTPRouteExtAuth`, `HTTPRouteExtAuthGRPC`, `HTTPRouteExtAuthHTTP`, `HTTPRouteExtAuthForwardBody`
- Summary: Defines external authentication/authorization configuration for north-south HTTP/GRPC traffic, with upstream currently focusing on the HTTPRoute ExternalAuth filter.
- Acceptance Criteria: For full capability, auth backend reference resolution, HTTP/gRPC ext_authz calls, body/header forwarding, fail-close policy, BackendTLS combination, status, and conformance must be complete; for subset only, unsupported fields must be clearly identified and the full feature not declared.
- Completion Evidence: The current implementation covers the HTTP and GRPC auth backend subset of `HTTPRoute ExternalAuth`, but does not declare `HTTPRouteExtAuth*` supportedFeatures. The control plane allows rule-level `protocol: HTTP` / `protocol: GRPC` and translates `backendRef`, HTTP `path` / allowed headers / allowed response headers, GRPC `allowedHeaders`, and `forwardBody.maxSize`; backendRef-level `ExternalAuth` still produces `HTTPRoute rule 1 uses unsupported ExternalAuth filter`. The data plane calls the HTTP auth backend or Envoy ext_authz-compatible gRPC `Authorization/Check` before forwarding, with allow passing through, deny returning to the client, connection/RPC/protocol errors failing closed, covering HTTP mandatory `Authorization` forwarding, HTTP allowed response header injection, GRPC header filtering, gRPC allow/deny/unavailable, HTTP/GRPC auth body forwarding when `forwardBody.maxSize > 0`, backend body replay after allow, and overflow `413` without calling auth/backend. Tests cover `controlplane/internal/gatewayapi/http_route_validation_test.go`, `controlplane/internal/translator/translator_test.go`, `dataplane/crates/aeg-ir/tests/extension_filters/external_auth.rs`, `dataplane/crates/aeg-http/src/runtime/tests_http1/external_auth.rs`, `dataplane/crates/aeg-http/src/filters/tests/redirect_and_support/supported_filters.rs`, and the `dataplane/crates/aeg-proto/proto/envoy/service/auth/v3/external_auth.proto` generation path; Kind/conformance and ExternalAuth with BackendTLSPolicy combination evidence is still missing.

### GEP-1619: Session Persistence

- Status: [x] Completed
- Upstream Status: Experimental
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1619/index.md>
- FeatureNames: None
- Summary: Standardizes session persistence configuration based on cookies or similar mechanisms, applicable to backends or route rules.
- Acceptance Criteria: Session persistence field parsing, IR delivery, cookie/TTL/absolute timeout/hash behavior, conflict priority, and data plane tests are complete; unsupported fields are explicitly rejected.
- Completion Evidence: `HTTPRoute.sessionPersistence`, `GRPCRoute.sessionPersistence`, and `BackendLBPolicy.sessionPersistence` are documented in `docs/gateway-api-support.md` and `docs/backlog/gateway-api-experimental.md` as implemented experimental subsets, but there is no independent upstream supportedFeature to declare. The control plane translates `Cookie` / `Header` transport, absolute/idle timeout, and backend/route-level priority via `controlplane/internal/translator/session_persistence.go` and `controlplane/internal/grpcserver/session_persistence.go`; the data plane `dataplane/crates/aeg-http/src/session.rs` uses stateless signed tokens to maintain backend affinity, falling back to normal selection when the target is unavailable. Tests cover `dataplane/crates/aeg-ir/tests/session_persistence/`, `dataplane/crates/aeg-http/src/session/tests/`, admin summary, and secret rotation e2e entry points `tests/e2e/validate-session-persistence.sh` and `validate-session-persistence-rotation-lib.sh`; documentation clarifies that production multi-replica deployments must configure a stable shared secret.

### GEP-1651: Gateway Routability

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1651/index.md>
- FeatureNames: None
- Summary: Allows users to express the routable scope of a Gateway, such as public/private/cluster or implementation-specific scopes.
- Acceptance Criteria: Gateway routability configuration, defaults, implementation-specific extensions, status address/condition, and documentation boundaries are clear; when not implemented, must not imply support for cluster-local/private Gateway.
- Completion Evidence: The current implementation does not implement an independent Gateway routability API or declare public/private/cluster-local Gateway scope. The reachability boundaries supported by the repository are documented in the `Gateway.spec.addresses`, `Gateway.status.addresses`, and `Gateway.spec.infrastructure` sections of `docs/gateway-api-support.md`: only the current subset of `IPAddress`/`Hostname` addresses, per-Gateway Service exposure parameters, and status address derivation are declared; there are no cluster-local/private routability fields, policies, or supportedFeatures. Unimplemented scopes are constrained by support matrix gaps and the GEP tracker, avoiding misinterpreting `Service.type` or externalIPs as full routability support.

### GEP-1686: Mesh conformance testing plan

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1686/index.md>
- FeatureNames: None
- Summary: Defines the scope, decomposition, and test scenarios for GAMMA mesh conformance testing.
- Acceptance Criteria: The project can run relevant conformance by mesh profile; reports distinguish between Gateway and Mesh profiles; missing tests are recorded in the backlog.
- Completion Evidence: `tests/conformance/run.sh` supports parameters such as `CONFORMANCE_PROFILES`, `SUPPORTED_FEATURES`, `ALL_FEATURES`, `RUN_TEST`, and `SKIP_TESTS`, and can derive the explicit feature set from the control plane feature source. `reports/conformance/latest/` currently points to the `2026-05-12-0355945e-kind-mesh-profile-current` kind mesh profile; `docs/gateway-api-support.md` clarifies that this latest is not the full-suite baseline and records the mesh profile separately from the clean full-suite baseline. `reports/conformance/README.md` describes evidence boundaries for mesh, UDPRoute, TCPRoute, etc.; `docs/test/plan.md` and `docs/test/checklist.md` document supplementary validation requirements for mesh/service parent.

### GEP-1709: Conformance Profiles

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1709/index.md>
- FeatureNames: None
- Summary: Defines Gateway API conformance profiles and reporting mechanisms, allowing implementations to run and declare conformance by profile/feature.
- Acceptance Criteria: Conformance scripts can select profiles and supportedFeatures; generate archivable reports; documentation describes current profile coverage and gaps.
- Completion Evidence: `tests/conformance/run.sh` can select profiles via `CONFORMANCE_PROFILES`, control the feature set via `SUPPORTED_FEATURES`/`EXEMPT_FEATURES`, and with `ALL_FEATURES=true` expands the current declarations from `controlplane/internal/gatewayapi/supported_features.go`. Report archiving is done by `scripts/archive-conformance-report.sh` generating `report.yaml`, `run.log`, `metadata.yaml`, `log-summary.json`, `summary.md`, with `scripts/publish-conformance-reports.sh` publishing external reports. `docs/gateway-api-support.md`, `reports/conformance/README.md`, and `docs/test/latest-baseline.md` clarify the differences between the current latest mesh profile, the most recent clean full-suite baseline, and the release full-suite gate.

### GEP-1713: ListenerSets (Standard Mechanism to Merge Gateways)

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1713/index.md>
- FeatureNames: `ListenerSet`
- Summary: Introduces `ListenerSet`, allowing multiple groups of listeners to be merged into a single Gateway for cross-namespace listener attachment and shared Gateway.
- Acceptance Criteria: Implement ListenerSet watch/index/status, listener merge/conflict, certificate/ref resolution, Route attachment, IR delivery, and conformance; must not declare `ListenerSet` when not implemented.
- Completion Evidence: The current implementation does not implement or declare `ListenerSet`. `controlplane/internal/gatewayapi/supported_features.go` does not include `ListenerSet`; `docs/gateway-api-support.md` explicitly lists ListenerSet as lacking watch, translator, status, and dataplane attachment closure in the unsupported/gap section; `docs/backlog/gateway-api-experimental.md` lists it as a deferred item requiring independent design and targeted conformance before expansion. Current listener merging/conflict is limited to within a single `Gateway.spec.listeners` and does not silently apply cross-resource ListenerSet semantics to Gateways.

### GEP-1731: HTTPRoute Retries

- Status: [x] Completed
- Upstream Status: Experimental
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1731/index.md>
- FeatureNames: `SupportHTTPRouteRetry`, `SupportHTTPRouteRetryBackendTimeout`, `SupportHTTPRouteRetryBackoff`, `SupportHTTPRouteRetryCodes`, `SupportHTTPRouteRetryConnectionError`
- Summary: Defines retry configuration for HTTPRoute based on status codes, connection errors, attempts, backoff, and backend timeout.
- Acceptance Criteria: Retries field parsing, defaults, interaction with timeouts, IR/proto, Rust proxy retry behavior, status, and featureNames/conformance are complete; unsupported sub-fields must not be declared.
- Completion Evidence: The current implementation covers the repository subset of `HTTPRoute.retry`, but Gateway API `v1.5.1` has no corresponding declarable `pkg/features` name, so it is not added to supportedFeatures. The control plane IR/proto includes `HttpRouteRetry`, with the translator parsing attempts, codes, backoff, and connection-error semantics; the data plane `dataplane/crates/aeg-http/src/proxy.rs` combines `RetryPolicy`, retry budget, and status code/connection failure triggers for retries, while preserving streaming/cancel not mis-retried regressions. `docs/gateway-api-support.md` and `docs/backlog/gateway-api-experimental.md` record it as an implemented experimental subset; `docs/test/regression-index.md` fixes streaming timeout/retry regression commands; finer-grained upstream `SupportHTTPRouteRetry*` featureNames do not exist in the current dependency version and are not externally declared.

### GEP-1742: HTTPRoute Timeouts

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1742/index.md>
- FeatureNames: None
- Summary: Defines portable HTTP timeout configuration for HTTPRoute, including request/backend timeout semantics.
- Acceptance Criteria: Duration format, request/backend timeout parsing, data plane enforcement, interaction with retries, and status tests are complete.
- Completion Evidence: `HTTPRouteRequestTimeout` and `HTTPRouteBackendTimeout` are declared in supportedFeatures and `docs/gateway-api-support.md`. The control plane `controlplane/internal/translator/timeouts.go` parses Gateway API duration and writes to `ir.RouteTimeouts`, delivered as protobuf duration via `controlplane/internal/grpcserver/snapshot_proto.go`; the data plane `aeg-ir` decodes it and `dataplane/crates/aeg-http/src/proxy.rs` applies request/backend peer timeout. Regression evidence includes control plane timeout unit tests, `dataplane/crates/aeg-ir` proto decode unit tests, `aeg-http` upstream timeout/streaming/cancel tests, and streamable HTTP timeout regression commands documented in `docs/test/regression-index.md`.

### GEP-1748: Gateway API Interaction with Multi-Cluster Services

- Status: [x] Completed
- Upstream Status: Experimental
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1748/index.md>
- FeatureNames: None
- Summary: Defines the interaction between Gateway API and Multi-Cluster Services API, allowing Routes to forward to ServiceImport.
- Acceptance Criteria: `ServiceImport` backendRef, endpoint aggregation, ReferenceGrant, status, translator/backends, and e2e/documentation are complete; graceful degradation when MCS CRD is absent.
- Completion Evidence: `docs/gateway-api-support.md` records `multicluster.x-k8s.io/ServiceImport` backendRef as implemented. The control plane admin API, translator backend ref, ReferenceGrant/status, partial rebuild, and policy target all recognize `ServiceImport`; relevant code anchors include `controlplane/internal/translator/backend_refs.go`, `controlplane/internal/controller/syncer_partial_rebuild_service_test.go`, and `controlplane/internal/admin/resource_api_special_kinds_test.go`. The data plane IR/proto handles ServiceImport backends as normal backend selection targets, with `dataplane/crates/aeg-ir/tests/invalid_backend_refs/serviceimport.rs` and session/backend policy tests covering parsing boundaries. Without MCS CRD, scheme/list/watch boundaries and admin/resource kind tests ensure missing objects are not silently treated as regular Services.

### GEP-1762: In Cluster Gateway Deployments

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1762/index.md>
- FeatureNames: None
- Summary: Provides guidance on deployment, scaling, naming, resource management, and behavior for in-cluster Gateway implementations.
- Acceptance Criteria: Controller/dataplane deployment model, Gateway infrastructure resources, owner refs, rolling update, manual deployment, and docs align with this GEP.
- Completion Evidence: The repository is currently positioned as an in-cluster Gateway implementation, with `deploy/kubernetes/base/`, `configs/`, `docs/architecture.md`, `docs/design.md`, and `docs/user/operations.md` documenting controlplane/dataplane deployment, admin/metrics, xDS, TLS/mTLS, and per-Gateway infrastructure models. The control plane `controlplane/internal/infrastructure/` handles per-Gateway Service, EndpointSlice, NetworkPolicy, ownerReference, and metadata drift, with tests including `reconciler_core_test.go`, `service_ownerrefs_test.go`, `frontend_endpoint_slice_ownerrefs_test.go`, and `reconciler_cleanup_test.go`. `tests/e2e/run-kind.sh` is the in-cluster smoke entry point; release/security scripts and support matrix records still lack longer production-grade multi-environment evidence.

### GEP-1767: CORS Filter

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1767/index.md>
- FeatureNames: `HTTPRouteCORS`
- Summary: Adds a standard CORS filter for HTTPRoute to handle cross-origin requests and preflight responses.
- Acceptance Criteria: CORS field parsing, preflight short-circuit, response headers, origin/method/header/mode validation, data plane tests, and supportedFeatures are consistent.
- Completion Evidence: `HTTPRouteCORS` is declared in supportedFeatures and `docs/gateway-api-support.md`. The control plane `controlplane/internal/translator/translator_filters.go` and extension filter resolver parse native/ExtensionRef CORS fields, with `controlplane/internal/gatewayapi/http_route_validation.go` listing CORS as a supported filter; the data plane `dataplane/crates/aeg-http/src/filters/cors.rs` and `proxy/request.rs` handle preflight short-circuit, origin/method/header validation, response header writing, and wildcard/credentials boundaries. Tests cover `dataplane/crates/aeg-http/src/filters/tests/cors/`, `dataplane/crates/aeg-ir/src/tests_proto/route_filters/cors.rs`, and control plane extension CORS validation; the support matrix records the current field scope.

### GEP-1867: Per-Gateway Infrastructure

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1867/index.md>
- FeatureNames: None
- Summary: Adds `infrastructure`-related configuration on Gateway, allowing implementation-specific or standardized infrastructure parameters for individual Gateways.
- Acceptance Criteria: Gateway infrastructure parameter references, status validation, deployment resource rendering, error states, and documentation boundaries are complete; unsupported parameters must not be silently ignored.
- Completion Evidence: `GatewayInfrastructurePropagation` is declared in supportedFeatures; `docs/gateway-api-support.md` records the current `Gateway.spec.infrastructure` subset: `labels/annotations` propagated to per-Gateway Service/EndpointSlice, `GatewayClass.spec.parametersRef` providing default Service parameters, and `Gateway.spec.infrastructure.parametersRef` covering Service type, traffic policy, IP family, LB-related fields, etc., with ownership, parametersRef, and effective hash annotations written to derived resources. Invalid/missing/unsupported parametersRef writes `Gateway Accepted=False, Reason=InvalidParameters` and prevents misconfiguration from being silently treated as successful; tests cover `controlplane/internal/infrastructure/reconciler_parameters_test.go`, `reconciler_gateway_services_test.go`, `reconciler_ownership_test.go`, and `controlplane/internal/status/reconciler_infrastructure_parameters_test.go`. Broader infrastructure orchestration capabilities remain explicitly undeclared.

### GEP-1897: BackendTLSPolicy - Explicit Backend TLS Connection Configuration

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1897/index.md>
- FeatureNames: None
- Summary: Defines TLS origination and certificate validation configuration from Gateway to backend, with BackendTLSPolicy as the core resource.
- Acceptance Criteria: BackendTLSPolicy targetRef, CA/SAN, hostname, ReferenceGrant, status, IR/proto, and data plane backend TLS validation are complete; cross-namespace and conflict semantics have tests.
- Completion Evidence: `BackendTLSPolicy` is declared in supportedFeatures, support matrix, and conformance profile; the control plane reads/updates status through the `gateway.networking.k8s.io/v1` compatibility access layer, supporting `Service`/`ServiceImport` targets, `validation.hostname`, system CA, same-namespace `ConfigMap/ca.crt` CA bundle, Hostname/URI SAN, conflict priority, and partial valid reference preservation. The translator/proto/dataplane delivers backend TLS configuration to `aeg-http`, with the data plane performing TLS origination and certificate/SAN validation on HTTPS/GRPCS upstream. Test anchors include `controlplane/internal/status/reconciler_backend_tls_validation_test.go`, `reconciler_backend_tls_precedence_test.go`, `controlplane/internal/translator/backend_tls_policy.go` related unit tests, `dataplane/crates/aeg-ir/tests/backend_tls.rs`, `dataplane/crates/aeg-http` backend TLS runtime tests, and `tests/e2e/validate-tls-asset-rotation.sh`; archived full-suite conformance includes BackendTLSPolicy related passing results.

### GEP-1911: Backend Protocol Selection

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-1911/index.md>
- FeatureNames: None
- Summary: Uses Kubernetes Service/EndpointSlice `appProtocol` to express the application protocol supported by the backend, avoiding implementations inferring the protocol independently.
- Acceptance Criteria: Backend appProtocol is read and mapped to behaviors such as HTTP/2/gRPC/WebSocket/TLS; unknown protocol status is clear; data plane uses the protocol selection result.
- Completion Evidence: `HTTPRouteBackendProtocolH2C` and `HTTPRouteBackendProtocolWebSocket` are declared in supportedFeatures, with the current subset documented in the `ServicePort.appProtocol` section of `docs/gateway-api-support.md`. The control plane reads `ServicePort.appProtocol` and recognizes `kubernetes.io/h2c`/`h2c`, `kubernetes.io/ws`/`ws`, `grpc`/`grpcs`, `http`/`https`, falling back to `ServicePort.protocol`/default HTTP behavior when unset; the translator delivers the result to the backend cluster. The data plane covers h2c prior knowledge, WebSocket upgrade, HTTP/1.1 and H2C backend coexistence, and GRPCS/HTTPS backend TLS combinations; relevant tests include `controlplane/internal/translator/translator_test.go`, `tests/e2e/validate-backend-protocols.sh`, and `dataplane/crates/aeg-http/src/proxy/tests/backend_protocol/`.

### GEP-2162: Supported features in GatewayClass Status

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2162/index.md>
- FeatureNames: None
- Summary: Publishes supported Gateway API features in `GatewayClass.status.supportedFeatures` for UX, conformance, and tooling.
- Acceptance Criteria: supportedFeatures static/dynamic sources are clear, sorting is stable, only features with evidence are declared; support matrix, audit tools, and conformance profile are consistent.
- Completion Evidence: `controlplane/internal/gatewayapi/supported_features.go` is the single feature source, with `SupportedFeatureNames()` stably sorted and used by `GatewayClass.status.supportedFeatures`, `cmd/gateway-api-support`, and `tests/conformance/run.sh ALL_FEATURES=true`. `controlplane/internal/gatewayapi/supported_features_test.go` pins the feature set and ordering; `scripts/update-gateway-api-support.sh` auto-generates the supportedFeatures table for `docs/gateway-api-support.md`; `scripts/audit-gateway-api-bundle.sh` prints/audits cluster GatewayClass supportedFeatures. The current feature count is consistent with the support matrix; unimplemented features like ListenerSet, TLSRoute terminate/mixed, and ExternalAuth are not in the declaration set.

### GEP-2257: Gateway API Duration Format

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2257/index.md>
- FeatureNames: None
- Summary: Standardizes Gateway API duration string format as the basis for fields like HTTPRoute timeouts.
- Acceptance Criteria: Duration parser/validator follows upstream regex and Go duration subset; invalid value status is clear; all GEPs using duration share the same semantics.
- Completion Evidence: Current duration usage is concentrated in HTTPRoute timeouts, retry backoff, and session persistence. The control plane reuses the duration field from Gateway API Go types and converts to `time.Duration` in `controlplane/internal/translator/timeouts.go`, `session_persistence.go`, and retry translation paths, then delivers via `google.protobuf.Duration`; the CRD schema handles first-layer format validation, while the translator/status layer only processes objects already accepted by the apiserver and exposes unsupported filter/value through Gateway API conditions. `docs/gateway-api-support.md` records the current support boundaries for timeouts, retry, and session persistence separately, avoiding over-declaration for future fields that do not yet use duration.

### GEP-2627: DNS configuration for Gateway API

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2627/index.md>
- FeatureNames: None
- Summary: Defines DNS configuration and status for Gateway API, allowing Gateways to express domain names, DNS provider selection, and DNS provision status.
- Acceptance Criteria: If implemented, must cover DNS configuration API, provider selection, status, errors, and multi-provider scenarios; if not implemented, documentation clarifies DNS is handled by external controllers.
- Completion Evidence: The current implementation does not implement the Gateway API DNS configuration API or declare DNS provider/provision status. `docs/gateway-api-support.md` limits `Gateway.spec.addresses`/`status.addresses` to IPAddress/Hostname address publishing and per-Gateway Service derived addresses, not including DNS provider selection, record management, or DNS conditions; deployment and operations documentation defaults to DNS being handled outside the cluster or by an independent DNS controller. The repository has no DNS CRD/watch, IR/proto, or data plane DNS configuration paths.

### GEP-2643: TLS based passthrough Route / TLSRoute

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2643/index.md>
- FeatureNames: `TLSRoute`, `TLSRouteModeTerminate`, `TLSRouteModeMixed`
- Summary: Defines SNI-based TLSRoute covering passthrough, and describes the extension direction for terminate/mixed.
- Acceptance Criteria: TLSRoute passthrough, terminate, and mixed are each independently implemented and declared; SNI route selection, listener protocol/mode, status, stream data-plane, and conformance coverage.
- Completion Evidence: The current implementation only declares and implements `TLSRoute` passthrough, not declaring `TLSRouteModeTerminate` or `TLSRouteModeMixed`. The control plane translates `TLSRoute` hostnames, Gateway/Service parentRef, Service backendRef, ReferenceGrant, and weights; the data plane `aeg-shared-tls`/`aeg-stream` prereads ClientHello and selects passthrough backends by SNI exact/wildcard matching; the HTTPS listener handles HTTP/GRPC routing after TLS termination. `docs/gateway-api-support.md` and `docs/backlog/gateway-api-experimental.md` clarify terminate/mixed as deferred; tests cover `dataplane/crates/aeg-shared-tls`, `dataplane/crates/aeg-stream`, `dataplane/crates/aeg-ir` TLSRoute selection, kind smoke, and targeted conformance evidence.

### GEP-2644: TCPRoute - Add support for routing based on a TCP port

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2644/index.md>
- FeatureNames: `TCPRoute`
- Summary: Defines TCPRoute for pure TCP workloads, binding by Gateway listener port and forwarding to TCP backends.
- Acceptance Criteria: TCPRoute watch/status/translator, stream runtime, backend refs, weights, listener attachment, and conformance coverage; documentation clarifies when advanced TCP match is not supported.
- Completion Evidence: The repository has implemented `TCPRoute` watcher/translator/status/stream runtime, but Gateway API `v1.5.1` has no `SupportTCPRoute` feature or official TCPRoute conformance test cases, so it is not declared in supportedFeatures. `docs/gateway-api-support.md` clarifies TCPRoute as `declared=no, implemented=yes, tested=kind smoke + unit`, forwarding to Service backends by listener/parentRef/port, supporting ReferenceGrant and weights, without extra advanced TCP matching. Test entry points include `tests/e2e/run-kind.sh` with `tcp-echo` success and `tcp-missing` missing-backend failure paths, `scripts/check-stream-route-test-coverage.sh`, `dataplane/crates/aeg-stream/src/tcp/tests/`, and `dataplane/crates/aeg-ir/tests_stream/`.

### GEP-2645: UDPRoute - Add support for routing based on a UDP port

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2645/index.md>
- FeatureNames: `UDPRoute`
- Summary: Defines UDPRoute for UDP workloads, binding by Gateway listener port and forwarding to UDP backends.
- Acceptance Criteria: UDPRoute watch/status/translator, stream runtime, session/flow behavior, backend refs, weights, listener attachment, and conformance coverage.
- Completion Evidence: `UDPRoute` is declared in supportedFeatures and `docs/gateway-api-support.md`. The control plane supports UDP listener attachment, Gateway/Service parentRef, Service backendRef, ReferenceGrant, weights, and basic status; the data plane `aeg-stream` UDP runtime maintains a client/upstream session/flow registry, admission/budget, and weighted backend selection. Official Gateway API `v1.5.1` conformance includes UDPRoute, with test cases recorded as passing in `reports/conformance/latest/run.log` and `reports/conformance/README.md`; kind smoke covers `udp-coredns` success and `udp-missing` failure paths; data plane tests cover `dataplane/crates/aeg-stream/src/udp/tests/` and `dataplane/crates/aeg-ir/tests_stream/stream_weighted.rs`.

### GEP-2648: Direct Policy Attachment

- Status: [x] Completed
- Upstream Status: Declined
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2648/index.md>
- FeatureNames: None
- Summary: Previously split the direct policy attachment pattern, later merged back into GEP-713 and marked obsolete/declined.
- Acceptance Criteria: Project policy evaluation follows GEP-713; if GEP-2648 is referenced, it is only as historical context, not as an independent supported feature.
- Completion Evidence: Upstream has declined/obsoleted this GEP; the repository does not treat GEP-2648 as an independent supportedFeature. Current direct policy attachment conclusions are consolidated under GEP-713: `BackendTLSPolicy` and `BackendLBPolicy` use targetRef/ancestor status patterns, with conflict and status semantics documented in `docs/status-matrix.md` and `docs/gateway-api-support.md`. There are no APIs, features, protos, or data plane behaviors named after GEP-2648.

### GEP-2649: Inherited Policy Attachment

- Status: [x] Completed
- Upstream Status: Declined
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2649/index.md>
- FeatureNames: None
- Summary: Previously split the inherited policy attachment pattern, later merged back into GEP-713 and marked obsolete/declined.
- Acceptance Criteria: If defaults/overrides, hierarchical propagation, and conflict semantics for inherited policy need to be implemented, they should return to GEP-713 or a specific policy GEP; this declined GEP must not be independently declared.
- Completion Evidence: Upstream has declined/obsoleted this GEP; the repository currently does not implement inherited policy attachment or declare any corresponding feature. `docs/gateway-api-support.md` explicitly limits the current policy surface to the direct targetRef subset of `BackendTLSPolicy` and `BackendLBPolicy`, with inherited defaults/overrides, hierarchical propagation, and additional Policy types not in the current completion scope; if needed later, they should return to GEP-713 or a specific policy GEP for redesign.

### GEP-2659: Document and improve the GEP process

- Status: [x] Completed
- Upstream Status: Accepted
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2659/index.md>
- FeatureNames: None
- Summary: Improves the GEP process, adding relationships, metadata schema, new status, and RFC2119 usage conventions.
- Acceptance Criteria: This tracker uses number/name/status/featureNames/relationships from metadata; future sync scripts or manual processes can discover new/changed GEPs.
- Completion Evidence: This tracker records entries by upstream GEP number/name/status/FeatureNames/Original URL/Summary/Acceptance Criteria, with in-project conclusions saved in `Completion Evidence`; the top of the document records the upstream sync commit `ec7d5a2f6ff132f2b2465aff81dfcba862738a9c` and sync date `2026-05-13`. There is currently no automatic sync script, but the manual sync process is fixed in the “Usage Rules” and “General Acceptance Criteria” sections; subsequent new/changed GEPs are discovered by re-checking the upstream `geps/` directory against the incomplete items in this table.

### GEP-2722: Goals and UX for gwctl

- Status: [x] Completed
- Upstream Status: Memorandum
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2722/index.md>
- FeatureNames: None
- Summary: Defines UX goals for the `gwctl` CLI to better display Gateway API resources, policy attachment, and status compared to kubectl.
- Acceptance Criteria: The project decides whether to provide or be compatible with gwctl output; if not implementing a local CLI, document the boundaries handled by upstream gwctl or dashboard/admin API.
- Completion Evidence: This GEP is a CLI UX memorandum; the repository currently does not provide a local `gwctl` implementation or declare gwctl-compatible output formats. In-project resource browsing and UX are handled by the control plane admin API, the `dashboard/` Next.js/React administration console, `docs/contracts/admin-api-contract.md`, and `docs/contracts/admin-api-surface.json`; `docs/gateway-api-support.md` records the admin API can manage GatewayClass/Gateway/HTTPRoute/GRPCRoute/TCPRoute/UDPRoute/TLSRoute/ServiceImport/BackendLBPolicy/BackendTLSPolicy/ReferenceGrant. If upstream gwctl support is needed later, it should be pursued as a separate CLI/API compatibility effort.

### GEP-2907: TLS Configuration Placement and Terminology

- Status: [x] Completed
- Upstream Status: Memorandum
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-2907/index.md>
- FeatureNames: None
- Summary: Unifies Gateway API TLS terminology and configuration placement, distinguishing between frontend/backend, terminate/passthrough, and client/server roles.
- Acceptance Criteria: The project's TLS documentation, field naming, IR/proto, and status reason are consistent with frontend/backend terminology; conflicts or historical naming have migration notes.
- Completion Evidence: `docs/gateway-api-support.md` and `docs/user/operations.md` document frontend/backend TLS separately: Gateway listener `certificateRefs` and `Gateway.spec.tls.frontend` are frontend TLS/client certificate validation; `BackendTLSPolicy` and `Gateway.spec.backendTLS.clientCertificateRef` are backend TLS origination/mTLS; `TLSRoute` only declares passthrough. Names like `FrontendValidation`, `BackendTlsConfig`, and `ClientCertificateRef` in IR/proto are consistent with this boundary; status reasons use `InvalidCertificateRef`, `InvalidCACertificateRef`, `NoValidCACertificate`, `InsecureFrontendValidationMode`, etc., to express failure semantics. Historically, TLSRoute terminate/mixed is undeclared to avoid confusion with HTTPS termination.

### GEP-3155: Complete Backend mutual TLS Configuration

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3155/index.md>
- FeatureNames: `BackendTLSPolicySANValidation`, `GatewayBackendClientCertificate`
- Summary: Complements backend mTLS configuration, including client certificate used by Gateway to connect to backends, SAN validation, and BackendTLSPolicy TLS options.
- Acceptance Criteria: Backend client cert reference, SAN validation, optional SPIFFE semantics, Secret/ReferenceGrant, IR/proto, data plane mTLS, and supportedFeatures are independently covered.
- Completion Evidence: `BackendTLSPolicySANValidation` and `GatewayBackendClientCertificate` are declared in supportedFeatures and `docs/gateway-api-support.md`. The control plane supports `Gateway.spec.backendTLS.clientCertificateRef` Secret references, cross-namespace ReferenceGrant, BackendTLSPolicy Hostname/URI SAN validation, and system/custom CA; the translator/proto delivers client certificates and backend TLS validation to the data plane, with `aeg-http` loading client certificates and performing SAN validation on HTTPS/GRPCS upstream. Tests include `controlplane/internal/status/backend_tls_test.go`, `reconciler_backend_tls_validation_test.go`, `controlplane/internal/translator/backend_tls.go` related tests, `dataplane/crates/aeg-ir/tests/backend_tls.rs`, backend TLS runtime tests, and `tests/e2e/validate-tls-asset-rotation.sh`; unsupported BackendTLSPolicy options are explicitly rejected in `controlplane/internal/backendtls/options.go`.

### GEP-3171: Percentage-based Request Mirroring

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3171/index.md>
- FeatureNames: `HTTPRouteRequestPercentageMirror`
- Summary: Extends HTTPRoute request mirroring to allow mirroring traffic by percentage, rather than only 0% or 100%.
- Acceptance Criteria: percentage/fraction field parsing, random or deterministic sampling strategy, mirror backend refs, data plane behavior, and conformance coverage; must not declare the feature if only 100% is supported.
- Completion Evidence: `HTTPRouteRequestMirror`, `HTTPRouteRequestMultipleMirrors`, and `HTTPRouteRequestPercentageMirror` are declared in supportedFeatures. The control plane parses `RequestMirror` percent/fraction, multiple mirrors, and mirror backendRef/ServiceImport; the extension filter resolver rejects simultaneous percent+fraction, invalid denominator/range, and unsupported backend kind; the data plane `dataplane/crates/aeg-http/src/mirror.rs` and IR request mirror selection perform percentage-based mirroring without affecting the primary request. Tests cover `controlplane/internal/extensionfilter/resolver_test.go`, `dataplane/crates/aeg-http/src/mirror/tests.rs`, `dataplane/crates/aeg-ir/src/tests_weighted/request_mirror/`, and runtime `request_mirror` tests; the archived full-suite summary records `HTTPRouteRequestPercentageMirror` as provisional success.

### GEP-3388: Retry Budgets

- Status: [x] Completed
- Upstream Status: Experimental
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3388/index.md>
- FeatureNames: None
- Summary: Defines a retry budget to limit the proportion of retries in the current request load, avoiding retry storms.
- Acceptance Criteria: Retry budget configuration location, ratio/min concurrency, interaction with HTTPRoute retries, data plane enforcement, metrics, and status are complete; when not implemented, clearly unsupported.
- Completion Evidence: The current implementation does not implement the Gateway API standard retry budget configuration API or declare the corresponding feature. There is a repo-internal `RetryBudgetController`/runtime option and metrics in the data plane for limiting current HTTP retry behavior and performance reporting, but this configuration does not come from Gateway API resources, has no control plane status, and is not declared as GEP-3388 support. `docs/gateway-api-support.md` only records `HTTPRoute.retry` as an implemented experimental subset and does not mark retry budget as a Gateway API feature; if upstream fields enter the dependency version in the future, control plane API, status, IR/proto, and conformance/e2e must be completed first.

### GEP-3567: Gateway TLS Updates for HTTP/2 Connection Coalescing

- Status: [x] Completed
- Upstream Status: Standard
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3567/index.md>
- FeatureNames: `GatewayHTTPSListenerDetectMisdirectedRequests`
- Summary: Addresses HTTPS listener hostname/SNI/Host inconsistencies caused by HTTP/2 connection coalescing, and defines the ability to detect misrouted requests.
- Acceptance Criteria: HTTPS listener can detect misdirected requests; when certificates cover multiple hostnames, routing still safely follows listener hostname; status, logs, tests, and featureName are consistent.
- Completion Evidence: `GatewayHTTPSListenerDetectMisdirectedRequests` is declared in supportedFeatures and `docs/gateway-api-support.md`. The data plane HTTPS handshake records the downstream SNI, and the HTTP proxy compares the best HTTPS listener matched by SNI versus Host before route selection, returning `421` when they are disjoint, and preserving normal routing/404 semantics for unknown SNI or unmatched Host; relevant code anchors include `dataplane/crates/aeg-ir/src/http_selection/candidates.rs`, `dataplane/crates/aeg-http/src/proxy.rs`, and `runtime/server.rs`. Tests cover `dataplane/crates/aeg-http/src/runtime/tests_http1/https_misdirected.rs`, shared TLS selection, and conformance HTTPS listener scenarios.

### GEP-3779: Identity Based Authz for east-west traffic

- Status: [x] Completed
- Upstream Status: Implementable
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3779/index.md>
- FeatureNames: None
- Summary: Defines identity-based AuthorizationPolicy for GAMMA east-west traffic, supporting access restriction by ServiceAccount, SPIFFE ID, and port.
- Acceptance Criteria: AuthorizationPolicy API, identity source, targetRef, allow/deny semantics, mesh data-plane enforcement, status, and conformance/e2e evidence are complete; when not implemented, clearly unsupported for mesh authz.
- Completion Evidence: The current implementation does not implement Gateway API/GAMMA AuthorizationPolicy or identity-based east-west authz, and does not declare related features. Implemented mesh capabilities are limited to Service parent routing, mesh frontend Service/EndpointSlice, route selection, and partial mesh conformance; `docs/gateway-api-support.md` clarifies that the mesh profile still lacks production-grade east-west long-stability evidence, and `docs/backlog/gateway-api-experimental.md` does not include mesh authz in the current maintainable subset. The repository has no ServiceAccount/SPIFFE identity source, allow/deny policy, authz IR/proto, or data plane enforcement paths.

### GEP-3792: Out-of-Cluster Gateways

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3792/index.md>
- FeatureNames: None
- Summary: Defines how out-of-cluster Gateways participate in in-cluster mesh, including secure communication, identity, credential maintenance, and mesh policy interaction.
- Acceptance Criteria: OCG identity, certificate/credential rotation, mesh authz, workload access paths, and documentation boundaries are clear; record as not supported when only in-cluster is supported.
- Completion Evidence: The current repository only supports an in-cluster controlplane/dataplane deployment model and does not implement out-of-cluster Gateway identity, credential maintenance, mesh authz, or workload access paths. `docs/architecture.md`, `deploy/kubernetes/base/`, and `tests/e2e/run-kind.sh` all use in-cluster Kind/Kubernetes as validation targets; xDS mTLS/certificate rotation only covers in-cluster controlplane and dataplane communication. The in-project conclusion for this GEP is “OCG not supported,” and it is not declared in supportedFeatures, proto, or data plane configuration.

### GEP-3793: Default Gateways

- Status: [x] Completed
- Upstream Status: Implementable
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3793/index.md>
- FeatureNames: None
- Summary: Allows Gateway to declare a default scope, with Routes implicitly binding to the default Gateway via `useDefaultGateways` without needing to explicitly write parentRefs.
- Acceptance Criteria: `Gateway.spec.defaultScope`, Route `spec.useDefaultGateways`, synthetic parent status, Gateway `DefaultGateway` condition, attachedRoutes, translator IR, partial rebuild, and documentation evidence are complete; do not declare non-existent featureNames in supportedFeatures.
- Completion Evidence: The current implementation covers the implicit Gateway parent subset for `Gateway.spec.defaultScope=All` and Route `spec.useDefaultGateways=All`, but upstream currently has no corresponding supportedFeature, so the repository does not declare it as a feature. The control plane `controlplane/internal/gatewayapi/default_gateways.go`, `translator_routes.go`, `partial_routes.go`, and status evaluator/reconciler synthesize parentRef, Route parent status, Gateway `DefaultGateway` condition, and scoped rebuild; tests cover `controlplane/internal/translator/attachments_test.go`, `controlplane/internal/status/object_reconciler_gateway_test.go`, and `object_reconciler_route_test.go`. `docs/gateway-api-support.md` and `docs/backlog/gateway-api-experimental.md` clarify this capability is a unit-covered experimental subset with no conformance/e2e/production evidence.

### GEP-3798: Client IP-Based Session Persistence

- Status: [x] Completed
- Upstream Status: Deferred
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3798/index.md>
- FeatureNames: None
- Summary: Proposes session persistence based on client IP or IP mask, related to GEP-1619 session persistence but with state maintained on the gateway/load balancer side.
- Acceptance Criteria: Do not declare support while upstream is deferred; if implementing experimental extensions, they must be clearly non-standard; add fields, data plane, and tests when resumed later.
- Completion Evidence: Upstream is Deferred; the repository does not declare client-IP Gateway API session persistence. The current standard/experimental session persistence subset only covers Cookie/Header transport; the repo-specific `BackendLBPolicy.loadBalancing.consistentHash` supports `SourceIP` hash key, but `docs/gateway-api-support.md` clarifies it is a backend load balancing extension, not the GEP-3798 IP mask/session persistence API, and has no corresponding supportedFeature. If upstream resumes later, fields, status, IR/proto, data plane, and tests need to be added separately.

### GEP-3949: Mesh Resource

- Status: [x] Completed
- Upstream Status: Implementable
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-3949/index.md>
- FeatureNames: None
- Summary: Defines a `Mesh` resource parallel to Gateway for mesh-wide configuration and mesh implementation feature discovery.
- Acceptance Criteria: Mesh resource watch/status/supportedFeatures, mesh-wide config, relationship with Service parent/GAMMA policy, and conformance coverage; when not implemented, documentation explains the source of mesh configuration.
- Completion Evidence: The current implementation does not implement an independent `Mesh` resource CRD/watch/status or mesh-wide config object. The repository's `Mesh` supportedFeature represents Gateway API conformance mesh profile/Service parent routing capability, not a Kubernetes `Mesh` resource implementation; `docs/gateway-api-support.md` describes Service parent / mesh frontend separately from the missing production east-west evidence. There is no Mesh resource supportedFeatures status, mesh-wide policy attachment, or data-plane config path; future implementation should be designed separately and avoid semantic confusion with existing feature names.

### GEP-4152: Extending TLS Validation in BackendTLSPolicy

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-4152/index.md>
- FeatureNames: None
- Summary: Extends BackendTLSPolicy to support modes such as skipping backend TLS validation, certificate fingerprint, or public key hash pinning.
- Acceptance Criteria: Security semantics of skip verify and pinning, field mutual exclusion, status warnings, data plane TLS validation, and documentation risk explanations are complete; defaults remain secure-by-default.
- Completion Evidence: The current implementation does not implement skip-verify, fingerprint pinning, or public key hash pinning, and does not declare GEP-4152 related support. `BackendTLSPolicy` currently remains secure-by-default: system/custom CA, hostname/SAN validation, and backend client certificate; `controlplane/internal/backendtls/options.go` explicitly rejects known unsupported implementation-specific TLS version options and errors on other unknown options. `docs/gateway-api-support.md` clarifies that `BackendTLSPolicy.spec.options` should not be considered supported; future extensions must separately add security semantics, status warnings, data plane validation, and risk documentation.

### GEP-4488: Backend Resource

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-4488/index.md>
- FeatureNames: `BackendResource`
- Summary: Proposes a new Gateway-native `Backend` resource for decorating Services, expressing external hostnames, and carrying backend-level connection metadata.
- Acceptance Criteria: Backend resource CRD/watch/status, EndpointSelector/ExternalHostname, Route backendRefs, TLS/protocol metadata, IR/proto, and security boundaries are complete; must not declare `BackendResource` when not implemented.
- Completion Evidence: The current implementation does not implement or declare `BackendResource`. Route backendRef support remains limited to `Service` and the current MCS `ServiceImport` subset; backend-level metadata is expressed through `ServicePort.appProtocol`, `BackendTLSPolicy`, and the current `BackendLBPolicy` subset; `docs/gateway-api-support.md` clarifies that other backendRef group/kind will be treated as `InvalidKind`. The repository has no Backend resource CRD/watch/status, EndpointSelector/ExternalHostname, security boundaries, or proto/IR delivery paths.

### GEP-4768: Standardized Telemetry API

- Status: [x] Completed
- Upstream Status: Provisional
- Original URL: <https://github.com/kubernetes-sigs/gateway-api/blob/main/geps/gep-4768/index.md>
- FeatureNames: None
- Summary: Proposes a provider-agnostic Telemetry API for configuring metrics, access logs, and traces for Gateway traffic.
- Acceptance Criteria: TelemetryPolicy or final API target/precedence, metrics/logs/traces/export config, data plane observability integration, and conflict status are complete; when not implemented, describe the boundary using existing metrics/admin/logging.
- Completion Evidence: The current implementation does not implement Gateway API TelemetryPolicy or standardized Telemetry API, and does not declare related features. The repository already has Prometheus metrics, admin API, traffic graph, access logs, and partial OpenTelemetry/logging runtime, but these configurations come from project-specific configuration and data plane options, not from Gateway API policy resources. `docs/gateway-api-support.md` only records the existing admin/metrics/logging as implemented observability capabilities, not promoted as GEP-4768 support; future implementation requires adding target/precedence, metrics/logs/traces export configuration, status, and data plane policy enforcement.
