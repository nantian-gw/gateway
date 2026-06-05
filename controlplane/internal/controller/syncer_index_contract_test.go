package controller

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
	gatewayv1alpha3 "sigs.k8s.io/gateway-api/apis/v1alpha3"

	"github.com/aether-gateway/aether-gateway/controlplane/internal/ir"
	"github.com/aether-gateway/aether-gateway/controlplane/internal/translator"
)

func TestBackendTLSPolicyConfigMapIndexFallbackLogsMissingIndexOnceWithoutIndexValue(t *testing.T) {
	scheme := runtime.NewScheme()
	mustAddToScheme(t, scheme, gatewayv1.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha2.Install)
	mustAddToScheme(t, scheme, gatewayv1alpha3.Install)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	syncer := NewSyncer(
		fake.NewClientBuilder().WithScheme(scheme).Build(),
		translator.New("gateway.networking.k8s.io/aether-gateway", logger),
		ir.NewSnapshotStore(logger),
		testMetrics(),
		time.Minute,
		logger,
	)

	policies, usedIndex, err := syncer.backendTLSPoliciesForConfigMapIndex(context.Background(), "default/ca")
	if err != nil {
		t.Fatalf("backendTLSPoliciesForConfigMapIndex returned error: %v", err)
	}
	if usedIndex {
		t.Fatal("backendTLSPoliciesForConfigMapIndex reported index usage despite missing index registration")
	}
	if len(policies) != 0 {
		t.Fatalf("expected no indexed policies when falling back from missing index, got %d", len(policies))
	}
	if _, _, err := syncer.backendTLSPoliciesForConfigMapIndex(context.Background(), "default/other-ca"); err != nil {
		t.Fatalf("second backendTLSPoliciesForConfigMapIndex returned error: %v", err)
	}

	logOutput := logs.String()
	if count := strings.Count(logOutput, "missing field index; falling back to configured list path"); count != 1 {
		t.Fatalf("expected exactly one missing index fallback warn, got %d in %q", count, logOutput)
	}
	if strings.Contains(logOutput, "index_value=") {
		t.Fatalf("expected missing index fallback warn to omit high-cardinality index_value, got %q", logOutput)
	}
	if !strings.Contains(logOutput, "missing field index; falling back to configured list path") {
		t.Fatalf("expected missing index fallback log, got %q", logOutput)
	}
	if !strings.Contains(logOutput, `field_index=`+backendTLSPolicyConfigMapRefIndex) {
		t.Fatalf("expected fallback log to include field index %q, got %q", backendTLSPolicyConfigMapRefIndex, logOutput)
	}
	if !strings.Contains(logOutput, "fallback_scope=namespace") {
		t.Fatalf("expected fallback log to include namespace fallback scope, got %q", logOutput)
	}
}

func TestControllerReferenceIndexContractsDeclareSemantics(t *testing.T) {
	contracts := controllerReferenceIndexContracts(true)
	wantNames := []IndexName{
		gatewaySecretReferenceIndex,
		gatewayConfigMapReferenceIndex,
		gatewayReferenceGrantNamespaceIndex,
		gatewayNamespaceSelectorIndex,
		httpRouteConfigMapReferenceIndex,
		httpRouteParentGatewayIndex,
		httpRouteReferenceGrantNamespaceIndex,
		grpcRouteConfigMapReferenceIndex,
		grpcRouteParentGatewayIndex,
		grpcRouteReferenceGrantNamespaceIndex,
		tcpRouteParentGatewayIndex,
		tcpRouteReferenceGrantNamespaceIndex,
		udpRouteParentGatewayIndex,
		udpRouteReferenceGrantNamespaceIndex,
		tlsRouteParentGatewayIndex,
		tlsRouteReferenceGrantNamespaceIndex,
		listenerSetParentGatewayIndex,
		backendTLSPolicyConfigMapRefIndex,
	}

	if len(contracts) != len(wantNames) {
		t.Fatalf("expected %d controller reference index contracts, got %d", len(wantNames), len(contracts))
	}

	seen := make(map[IndexName]fieldIndexContract, len(contracts))
	for _, contract := range contracts {
		if contract.Name == "" {
			t.Fatal("index contract has empty name")
		}
		if contract.Object == nil {
			t.Fatalf("index contract %q has nil object", contract.Name)
		}
		if contract.Extract == nil {
			t.Fatalf("index contract %q has nil extract function", contract.Name)
		}
		if contract.Owner == "" {
			t.Fatalf("index contract %q has empty owner", contract.Name)
		}
		if contract.WatchSource == "" {
			t.Fatalf("index contract %q has empty watch source", contract.Name)
		}
		if contract.RequestMapping == "" {
			t.Fatalf("index contract %q has empty request mapping", contract.Name)
		}
		if contract.Fallback == "" {
			t.Fatalf("index contract %q has empty fallback semantics", contract.Name)
		}
		if _, ok := seen[contract.Name]; ok {
			t.Fatalf("duplicate index contract %q", contract.Name)
		}
		seen[contract.Name] = contract
	}

	for _, name := range wantNames {
		if _, ok := seen[name]; !ok {
			t.Fatalf("missing index contract %q", name)
		}
	}
}
