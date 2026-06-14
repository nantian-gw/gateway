# Controlplane Tracing Closure Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a repository-local controlplane tracing closure for `gateway` so operators can enable, verify, and troubleshoot tracing from this repository alone.

**Architecture:** Keep the existing config-driven OpenTelemetry setup, add a small tracing summary helper for startup visibility, enrich the highest-value controlplane spans with stable diagnostic attributes, and add an explicit tracing-enabled Kustomize overlay that reuses the current deployment layout. Document the new entry point in the repository README and deploy guide without changing default behavior for existing install paths.

**Tech Stack:** Go 1.26, OpenTelemetry Go SDK, controller-runtime, Kustomize via `kubectl kustomize`, Markdown repository docs

---

## File Map

- Modify: `cmd/manager/app.go`
  - Wire a structured tracing status log into controlplane startup after tracing is configured.
- Modify: `cmd/manager/app_test.go`
  - Add tests that prove tracing startup logging uses sanitized summary fields.
- Modify: `internal/observability/tracing.go`
  - Add a tracing summary helper that normalizes endpoint, sampler ratio, and header count without exposing header values.
- Modify: `internal/observability/tracing_test.go`
  - Add focused tests for tracing summary normalization.
- Modify: `internal/controller/leader_runner.go`
  - Add run-level and scope-level span attributes for requested, succeeded, and failed reconcile scopes.
- Modify: `internal/controller/leader_runner_test.go`
  - Assert new run span attributes using existing `tracetest` infrastructure.
- Modify: `internal/controller/syncer.go`
  - Add snapshot build shape attributes to the publish span.
- Modify: `internal/controller/syncer_scope.go`
  - Add a tiny route-key counting helper used by snapshot tracing.
- Modify: `internal/controller/syncer_test.go`
  - Assert new snapshot span attributes.
- Modify: `internal/infrastructure/reconciler.go`
  - Add a small result attribute that makes success vs gateway-service failure visible in the infrastructure reconcile span.
- Modify: `internal/infrastructure/reconciler_core_test.go`
  - Assert the new infrastructure span attribute alongside the existing span name.
- Create: `deploy/kubernetes/overlays/observability-enabled/kustomization.yaml`
  - Add an explicit tracing-enabled Kustomize entry point.
- Create: `deploy/kubernetes/overlays/observability-enabled/controlplane-config.yaml`
  - Provide tracing-enabled controlplane configuration for the new overlay.
- Modify: `README.md`
  - Add a short tracing entry point under operations/observability.
- Modify: `deploy/README.md`
  - Document the new overlay, its intent, and the operator verification flow.
- Modify: `deploy/kubernetes/overlays/production/README.md`
  - Update wording so profile references stay accurate after the new overlay is added.

### Task 1: Startup Tracing Visibility

**Files:**
- Modify: `internal/observability/tracing.go`
- Modify: `internal/observability/tracing_test.go`
- Modify: `cmd/manager/app.go`
- Modify: `cmd/manager/app_test.go`

- [ ] **Step 1: Write the failing tests**

Add this test to `internal/observability/tracing_test.go`:

```go
func TestSummarizeTracingNormalizesFields(t *testing.T) {
	t.Parallel()

	summary := SummarizeTracing(TracingConfig{
		Enabled:      true,
		Endpoint:     " otel-collector:4317 ",
		Insecure:     true,
		SamplerRatio: 5,
		Headers: map[string]string{
			" authorization ": " Bearer token ",
			"":                 "ignored",
		},
	})

	if !summary.Enabled {
		t.Fatal("expected tracing summary to stay enabled")
	}
	if summary.Endpoint != "otel-collector:4317" {
		t.Fatalf("summary endpoint = %q, want otel-collector:4317", summary.Endpoint)
	}
	if !summary.Insecure {
		t.Fatal("expected insecure transport to be preserved")
	}
	if summary.SamplerRatio != 1 {
		t.Fatalf("summary sampler ratio = %v, want 1", summary.SamplerRatio)
	}
	if summary.HeaderCount != 1 {
		t.Fatalf("summary header count = %d, want 1", summary.HeaderCount)
	}
}
```

Add this test to `cmd/manager/app_test.go`:

```go
func TestLogControlplaneTracingStatusRedactsHeaderValues(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	logControlplaneTracingStatus(logger, observability.TracingConfig{
		Enabled:      true,
		Endpoint:     "otel-collector:4317",
		Insecure:     true,
		SamplerRatio: 0.25,
		Headers: map[string]string{
			"authorization": "Bearer secret-token",
		},
	})

	output := buf.String()
	if !strings.Contains(output, "configured controlplane tracing") {
		t.Fatalf("expected tracing log message, got %q", output)
	}
	if !strings.Contains(output, "header_count=1") {
		t.Fatalf("expected tracing header count in log output, got %q", output)
	}
	if strings.Contains(output, "secret-token") {
		t.Fatalf("expected tracing log to redact header values, got %q", output)
	}
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:

```bash
go test ./internal/observability ./cmd/manager -run 'TestSummarizeTracingNormalizesFields|TestLogControlplaneTracingStatusRedactsHeaderValues' -count=1
```

Expected: FAIL with undefined `SummarizeTracing` and undefined `logControlplaneTracingStatus`.

- [ ] **Step 3: Write the minimal implementation**

Add this helper to `internal/observability/tracing.go`:

```go
type TracingSummary struct {
	Enabled      bool
	Endpoint     string
	Insecure     bool
	SamplerRatio float64
	HeaderCount  int
}

func SummarizeTracing(cfg TracingConfig) TracingSummary {
	headers := traceHeaders(cfg.Headers)
	return TracingSummary{
		Enabled:      cfg.Enabled,
		Endpoint:     strings.TrimSpace(cfg.Endpoint),
		Insecure:     cfg.Insecure,
		SamplerRatio: clampSamplerRatio(cfg.SamplerRatio),
		HeaderCount:  len(headers),
	}
}
```

Update `cmd/manager/app.go` to centralize and log the tracing summary:

```go
func logControlplaneTracingStatus(logger *slog.Logger, cfg observability.TracingConfig) {
	if logger == nil {
		return
	}

	summary := observability.SummarizeTracing(cfg)
	logger.Info(
		"configured controlplane tracing",
		"enabled", summary.Enabled,
		"endpoint", summary.Endpoint,
		"insecure", summary.Insecure,
		"sampler_ratio", summary.SamplerRatio,
		"header_count", summary.HeaderCount,
	)
}
```

Then replace the current startup block in `run` with:

```go
	tracingCfg := controlplaneTracingConfig(cfg)
	tracingShutdown, err := observability.ConfigureTracing(ctx, tracingCfg)
	if err != nil {
		return fmt.Errorf("configure tracing: %w", err)
	}
	logControlplaneTracingStatus(logger, tracingCfg)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer shutdownCancel()
		_ = tracingShutdown(shutdownCtx)
	}()
```

Update `cmd/manager/app_test.go` imports to include:

```go
import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
)
```

- [ ] **Step 4: Run the focused tests to verify they pass**

Run:

```bash
go test ./internal/observability ./cmd/manager -run 'TestSummarizeTracingNormalizesFields|TestLogControlplaneTracingStatusRedactsHeaderValues|TestControlplaneTracingConfigUsesNormalizedConfigValues' -count=1
```

Expected: PASS with 3 passing tests and no leaked header values in output.

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/observability/tracing.go internal/observability/tracing_test.go cmd/manager/app.go cmd/manager/app_test.go
git commit -m "feat: add controlplane tracing startup summary"
```

### Task 2: Enrich Controlplane Trace Spans

**Files:**
- Modify: `internal/controller/leader_runner.go`
- Modify: `internal/controller/leader_runner_test.go`
- Modify: `internal/controller/syncer.go`
- Modify: `internal/controller/syncer_scope.go`
- Modify: `internal/controller/syncer_test.go`
- Modify: `internal/infrastructure/reconciler.go`
- Modify: `internal/infrastructure/reconciler_core_test.go`

- [ ] **Step 1: Write the failing tests**

Add this test to `internal/controller/leader_runner_test.go`:

```go
func TestReconcilerRunnerRunSpanRecordsScopeResults(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	runner := NewReconcilerRunner(
		time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		testReconcilerRunnerMetrics(),
		NewScopedReconciler("infra", staticReconciler{}, ReconcilerRunnerScopeInfra),
	)

	runner.runOnce(context.Background(), ReconcilerRunnerScopeInfra)

	runSpan, ok := spanByName(exporter.GetSpans(), "controlplane.reconciler_runner.run")
	if !ok {
		t.Fatal("expected run span")
	}
	if got := spanStringSliceAttr(runSpan, "reconciler.scopes.requested"); !slices.Equal(got, []string{"infra"}) {
		t.Fatalf("requested scopes = %v, want [infra]", got)
	}
	if got := spanStringSliceAttr(runSpan, "reconciler.scopes.succeeded"); !slices.Equal(got, []string{"infra"}) {
		t.Fatalf("succeeded scopes = %v, want [infra]", got)
	}
	if got := spanBoolAttr(runSpan, "reconciler.failed"); got {
		t.Fatal("expected successful run span")
	}
}
```

Add this test to `internal/controller/syncer_test.go`:

```go
func TestSyncerPublishSnapshotSpanRecordsBuildShape(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1beta1.Install)
	mustAddToScheme(t, scheme, corev1.AddToScheme)
	mustAddToScheme(t, scheme, discoveryv1.AddToScheme)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := ir.NewSnapshotStore(logger)
	syncer := NewSyncer(
		newControllerClientBuilder(scheme).Build(),
		translator.New("gateway.networking.k8s.io/nantian-gw", logger),
		store,
		testMetrics(),
		0,
		logger,
	)

	_, _ = syncer.publishSnapshotWithScope(
		context.Background(),
		snapshotBuildScopeRoutes,
		[]string{"apps"},
		[]string{"backends"},
		[]client.ObjectKey{{Namespace: "default", Name: "edge"}},
		[]client.ObjectKey{{Namespace: "default", Name: "echo"}},
		nil,
		snapshotRouteObjectKeys{
			http: []client.ObjectKey{{Namespace: "default", Name: "echo"}},
		},
	)

	span, ok := spanByName(exporter.GetSpans(), "controlplane.syncer.publish_snapshot")
	if !ok {
		t.Fatal("expected publish snapshot span")
	}
	if got := spanStringAttr(span, "snapshot.scope"); got != snapshotBuildScopeRoutes.String() {
		t.Fatalf("snapshot scope = %q, want %q", got, snapshotBuildScopeRoutes.String())
	}
	if got := spanIntAttr(span, "snapshot.gateway_key_count"); got != 1 {
		t.Fatalf("gateway key count = %d, want 1", got)
	}
	if got := spanIntAttr(span, "snapshot.route_key_count"); got != 1 {
		t.Fatalf("route key count = %d, want 1", got)
	}
	if got := spanBoolAttr(span, "snapshot.published"); !got {
		t.Fatal("expected snapshot span to record publish=true")
	}
}
```

Add this test to `internal/infrastructure/reconciler_core_test.go`:

```go
func TestInfrastructureReconcileSpanRecordsGatewayServiceResult(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	original := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	defer func() { otel.SetTracerProvider(original) }()

	reconciler := New(newInfrastructureClientBuilder(newScheme(t)).Build(), "gateway.networking.k8s.io/nantian-gw", discardLogger())
	_ = reconciler.Reconcile(context.Background())

	span, ok := spanByName(exporter.GetSpans(), "controlplane.infrastructure.reconcile")
	if !ok {
		t.Fatal("expected infrastructure reconcile span")
	}
	if got := spanBoolAttr(span, "infrastructure.gateway_services_failed"); got {
		t.Fatal("expected empty infrastructure reconcile to complete without gateway service failure")
	}
}
```

Extend the existing tracing test helpers in the three test files with:

```go
func spanByName(spans tracetest.SpanStubs, name string) (tracetest.SpanStub, bool) {
	for _, span := range spans {
		if span.Name == name {
			return span, true
		}
	}
	return tracetest.SpanStub{}, false
}
```

For `internal/controller/leader_runner_test.go` and `internal/controller/syncer_test.go`, also add:

```go
func spanStringAttr(span tracetest.SpanStub, key string) string {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	return ""
}

func spanStringSliceAttr(span tracetest.SpanStub, key string) []string {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsStringSlice()
		}
	}
	return nil
}

func spanBoolAttr(span tracetest.SpanStub, key string) bool {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsBool()
		}
	}
	return false
}

func spanIntAttr(span tracetest.SpanStub, key string) int64 {
	for _, attr := range span.Attributes {
		if string(attr.Key) == key {
			return attr.Value.AsInt64()
		}
	}
	return 0
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:

```bash
go test ./internal/controller ./internal/infrastructure -run 'TestReconcilerRunnerRunSpanRecordsScopeResults|TestSyncerPublishSnapshotSpanRecordsBuildShape|TestInfrastructureReconcileSpanRecordsGatewayServiceResult' -count=1
```

Expected: FAIL because the new span attributes are not emitted yet.

- [ ] **Step 3: Write the minimal implementation**

Update the run span in `internal/controller/leader_runner.go`:

```go
	requested := requestedScopes.sortedOrFull()
	span.SetAttributes(
		attribute.StringSlice("reconciler.scopes.requested", runnerScopeStrings(requested)),
		attribute.Int("reconciler.scope_count", len(requested)),
	)
```

Then, before each `return` in `runOnce`, record the run result:

```go
	span.SetAttributes(
		attribute.StringSlice("reconciler.scopes.succeeded", runnerScopeStrings(successfulScopes.sorted())),
		attribute.StringSlice("reconciler.scopes.failed", runnerScopeStrings(failedScopes.sorted())),
		attribute.Bool("reconciler.failed", !failedScopes.empty()),
	)
```

Update the publish span in `internal/controller/syncer.go`:

```go
	span.SetAttributes(
		attribute.String("snapshot.scope", scope.String()),
		attribute.Int("snapshot.attachment_namespace_count", len(attachmentNamespaces)),
		attribute.Int("snapshot.backend_namespace_count", len(backendNamespaces)),
		attribute.Int("snapshot.gateway_key_count", len(gatewayKeys)),
		attribute.Int("snapshot.service_key_count", len(serviceKeys)),
		attribute.Int("snapshot.service_import_key_count", len(serviceImportKeys)),
		attribute.Int("snapshot.route_key_count", routeKeys.count()),
	)
```

Add this helper near `snapshotRouteObjectKeys` in `internal/controller/syncer_scope.go`:

```go
func (k snapshotRouteObjectKeys) count() int {
	return len(k.http) + len(k.grpc) + len(k.tcp) + len(k.udp) + len(k.tls)
}
```

Update the infrastructure reconcile span in `internal/infrastructure/reconciler.go` before the final return:

```go
	span.SetAttributes(attribute.Bool("infrastructure.gateway_services_failed", gwErr != nil))
```

- [ ] **Step 4: Run the focused tests to verify they pass**

Run:

```bash
go test ./internal/controller ./internal/infrastructure -run 'TestReconcilerRunnerRunSpanRecordsScopeResults|TestSyncerPublishSnapshotSpanRecordsBuildShape|TestInfrastructureReconcileSpanRecordsGatewayServiceResult|TestReconcilerRunnerCreatesScopeSpans|TestSyncerPublishSnapshotCreatesSpan|TestInfrastructureReconcileCreatesSpan' -count=1
```

Expected: PASS with all tracing-focused controller and infrastructure tests green.

- [ ] **Step 5: Commit**

Run:

```bash
git add internal/controller/leader_runner.go internal/controller/leader_runner_test.go internal/controller/syncer.go internal/controller/syncer_scope.go internal/controller/syncer_test.go internal/infrastructure/reconciler.go internal/infrastructure/reconciler_core_test.go
git commit -m "feat: enrich controlplane tracing spans"
```

### Task 3: Add a Tracing-Enabled Kustomize Overlay

**Files:**
- Create: `deploy/kubernetes/overlays/observability-enabled/kustomization.yaml`
- Create: `deploy/kubernetes/overlays/observability-enabled/controlplane-config.yaml`

- [ ] **Step 1: Run the render command to verify the overlay does not exist yet**

Run:

```bash
kubectl kustomize deploy/kubernetes/overlays/observability-enabled
```

Expected: FAIL with a path-not-found error because the overlay directory does not exist yet.

- [ ] **Step 2: Create the tracing-enabled overlay files**

Create `deploy/kubernetes/overlays/observability-enabled/kustomization.yaml`:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
  - ../production

generatorOptions:
  disableNameSuffixHash: true

configMapGenerator:
  - name: nantian-gw-controlplane-config
    namespace: nantian-gw
    behavior: replace
    files:
      - config.yaml=controlplane-config.yaml
```

Create `deploy/kubernetes/overlays/observability-enabled/controlplane-config.yaml`:

```yaml
grpcAddr: ":18080"
adminAddr: ":18081"
metricsAddr: ":18082"
healthProbeAddr: ":18083"
controllerName: "gateway.networking.k8s.io/nantian-gw"
statusAddresses:
  - "replace-me.example.invalid"
syncPeriod: 30s
syncSettleDelay: 200ms
reconcilerRunner:
  settleDelay: "300ms"
  retryBackoff: "1s"
nodeStatus:
  leasePrefix: "aeg-node"
  persistTimeout: "2s"
  persistDebounce: "250ms"
adminReadiness:
  mode: "snapshot"
adminRuntime:
  readHeaderTimeout: "5s"
  readTimeout: "30s"
  writeTimeout: "30s"
  idleTimeout: "2m"
nodeDrift:
  warningThreshold: "15s"
log:
  level: "info"
  format: "json"
  addSource: false
leaderElection:
  enabled: true
  id: "nantian-controlplane-leader"
  leaseDuration: "15s"
  renewDeadline: "10s"
  retryPeriod: "2s"
adminAuth:
  bearerToken: ""
  bearerTokenFile: "/etc/nantian-gw/admin-auth/token"
dashboard:
  enabled: true
  capabilities:
    aiOverview: true
    aiServices: true
    aiTokenPolicies: true
    aiCost: true
    aiTraces: true
    aiUsage: true
    wasmPlugins: true
    chatbot: true
adminTLS:
  enabled: false
  certPath: ""
  keyPath: ""
grpcTLS:
  enabled: true
  certPath: "/etc/nantian-gw/grpc-tls/tls.crt"
  keyPath: "/etc/nantian-gw/grpc-tls/tls.key"
  clientCAPath: "/etc/nantian-gw/grpc-tls/ca.crt"
  requireClientCert: true
tracing:
  enabled: true
  endpoint: "otel-collector.observability.svc.cluster.local:4317"
  insecure: true
  samplerRatio: 0.1
```

- [ ] **Step 3: Render the overlay and verify the tracing config appears**

Run:

```bash
kubectl kustomize deploy/kubernetes/overlays/observability-enabled >/tmp/nantian-gw-observability-enabled.yaml
rg -n "tracing:" /tmp/nantian-gw-observability-enabled.yaml
rg -n "otel-collector.observability.svc.cluster.local:4317" /tmp/nantian-gw-observability-enabled.yaml
```

Expected:
- `kubectl kustomize` exits 0
- the rendered manifest contains a controlplane ConfigMap with the tracing block
- the rendered manifest contains the OTLP endpoint example value

- [ ] **Step 4: Commit**

Run:

```bash
git add deploy/kubernetes/overlays/observability-enabled/kustomization.yaml deploy/kubernetes/overlays/observability-enabled/controlplane-config.yaml
git commit -m "feat: add tracing-enabled deployment overlay"
```

### Task 4: Document the Tracing Entry Point and Run Full Acceptance

**Files:**
- Modify: `README.md`
- Modify: `deploy/README.md`
- Modify: `deploy/kubernetes/overlays/production/README.md`

- [ ] **Step 1: Confirm the docs do not yet describe the new overlay**

Run:

```bash
rg -n "observability-enabled|controlplane tracing|otel-collector" README.md deploy/README.md deploy/kubernetes/overlays/production/README.md
```

Expected: existing matches do not yet describe the new overlay as the concrete tracing enablement path.

- [ ] **Step 2: Update the docs**

Add this block to `README.md` under **Operations And Observability**:

```md
- Controlplane tracing can be enabled with `deploy/kubernetes/overlays/observability-enabled/` when you want OTLP trace export from the controlplane without changing the default production overlay.
```

Update `deploy/README.md` in the directory structure and entry-point sections with text equivalent to:

```md
- `deploy/kubernetes/overlays/observability-enabled/`
  Tracing-enabled controlplane entry point. It reuses the production overlay and replaces only the controlplane config map content with a tracing-enabled configuration example.
```

And add a short verification subsection:

````md
### Controlplane Tracing Verification

1. Render the overlay:

   ```bash
   kubectl kustomize deploy/kubernetes/overlays/observability-enabled
   ```

2. Update the OTLP endpoint in `deploy/kubernetes/overlays/observability-enabled/controlplane-config.yaml`.
3. Apply the overlay and inspect controlplane logs for `configured controlplane tracing`.
4. If traces do not appear, first verify that:
   - the tracing-enabled overlay was used instead of `production`
   - the OTLP endpoint is reachable from the controlplane Pod
   - header values are intentionally redacted from startup logs
````

Update `deploy/kubernetes/overlays/production/README.md` so the profile sentence reads:

```md
The matrix of install profiles, Secrets, NetworkPolicy, Services, ports, HPA, PDB, and resource requests/limits can be found in [Install Profile Matrix](../../../../docs/user/install-profiles.md). This directory is the current Kustomize source for the `single-cluster-prod` and `multi-replica-prod` profiles. The `observability-enabled` profile now lives in `../observability-enabled/` and reuses this overlay as its base.
```

- [ ] **Step 3: Run repository acceptance commands**

Run:

```bash
go test ./cmd/manager ./internal/observability ./internal/controller ./internal/infrastructure
make test
kubectl kustomize deploy/kubernetes/overlays/observability-enabled >/tmp/nantian-gw-observability-enabled.yaml
git diff --check origin/main...HEAD
git -C /root/nantian-gw/dataplane status --short --branch
git -C /root/nantian-gw/dashboard status --short --branch
git -C /root/nantian-gw/website status --short --branch
git -C /root/nantian-gw/proto status --short --branch
git -C /root/nantian-gw/helm-charts status --short --branch
```

Expected:
- focused Go tests PASS
- `make test` PASS
- overlay render command exits 0
- `git diff --check origin/main...HEAD` prints no whitespace or merge-marker issues
- sibling repositories show no modified tracked files caused by this task

- [ ] **Step 4: Commit**

Run:

```bash
git add README.md deploy/README.md deploy/kubernetes/overlays/production/README.md
git commit -m "docs: document controlplane tracing enablement"
```

## Self-Review

- Spec coverage:
  - startup visibility is implemented in Task 1
  - higher-value controlplane spans are implemented in Task 2
  - explicit tracing-enabled Kustomize entry point is implemented in Task 3
  - repository-local enable/verify/troubleshoot docs are implemented in Task 4
  - sibling repositories stay unchanged and are verified in Task 4
- Placeholder scan:
  - no placeholder keywords or cross-task shorthand remain
- Type consistency:
  - `SummarizeTracing`, `logControlplaneTracingStatus`, and `snapshotRouteObjectKeys.count()` are introduced before later tasks depend on them
