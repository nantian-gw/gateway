package infrastructure

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/nodeinfo"
)

const stableFrontendCohortWarning = "no dataplane node has acknowledged the current snapshot; exposing the last stable frontend cohort"

func TestLoadFrontendEligibleDataplanePodsRequiresCurrentSnapshotForAllFrontends(t *testing.T) {
	reconciler, logBuffer := newFrontendEligibilityTestReconciler(t)

	sharedPods, gatewayPods, meshPods, err := reconciler.loadFrontendEligibleDataplanePods(context.Background())
	if err != nil {
		t.Fatalf("loadFrontendEligibleDataplanePods returned error: %v", err)
	}

	if len(sharedPods) != 0 {
		t.Fatalf("unexpected shared eligible pods without current snapshot ACK: %#v", sharedPods)
	}
	if len(gatewayPods) != 0 {
		t.Fatalf("unexpected gateway eligible pods: %#v", gatewayPods)
	}
	if len(meshPods) != 0 {
		t.Fatalf("unexpected mesh eligible pods: %#v", meshPods)
	}
	if got := strings.Count(logBuffer.String(), stableFrontendCohortWarning); got != 0 {
		t.Fatalf("stable cohort warning count = %d, want 0; logs=%q", got, logBuffer.String())
	}
}

func TestLoadFrontendEligibleDataplanePodsRejectsPeerSnapshotAckForAllFrontends(t *testing.T) {
	scheme := newScheme(t)
	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-current",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-gw-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.51",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	store := ir.NewSnapshotStore(discardLogger())
	if !store.Publish(&ir.Snapshot{}) {
		t.Fatal("expected snapshot publish to succeed")
	}
	if store.Current().ID == "peer-snapshot-version" {
		t.Fatal("test requires the local controlplane snapshot version to differ from the peer version")
	}

	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, discardLogger(), nodeinfo.Options{})
	now := time.Now().UTC()
	nodes.Connect(context.Background(), "nantian-dataplane-current", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-current", "peer-snapshot-version", now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-current", "kind", "peer-snapshot-version", "peer-snapshot-version", nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-current", "peer-snapshot-version", true, "ready", now)

	options := DefaultOptions()
	options.SnapshotStore = store
	options.NodeStatus = nodes
	reconciler := NewWithOptions(k8sClient, k8sClient, "gateway.networking.k8s.io/nantian-gw", options, discardLogger())

	sharedPods, gatewayPods, meshPods, err := reconciler.loadFrontendEligibleDataplanePods(context.Background())
	if err != nil {
		t.Fatalf("loadFrontendEligibleDataplanePods returned error: %v", err)
	}

	if len(sharedPods) != 0 {
		t.Fatalf("unexpected shared eligible pods: %#v", sharedPods)
	}
	if len(gatewayPods) != 0 {
		t.Fatalf("unexpected gateway eligible pods without current snapshot ACK: %#v", gatewayPods)
	}
	if len(meshPods) != 0 {
		t.Fatalf("unexpected mesh eligible pods without current snapshot ACK: %#v", meshPods)
	}
}

func TestExpectedInfrastructureDoesNotUseStableFallback(t *testing.T) {
	reconciler, logBuffer := newFrontendEligibilityTestReconciler(t)

	expectedServices, expectedSlices, err := reconciler.expectedInfrastructure(
		context.Background(),
		nil,
		nil,
		nil,
		map[string]corev1.Service{},
	)
	if err != nil {
		t.Fatalf("expectedInfrastructure returned error: %v", err)
	}

	if len(expectedServices) != 0 {
		t.Fatalf("expected no services, got %#v", expectedServices)
	}
	if len(expectedSlices) != 0 {
		t.Fatalf("expected no endpoint slices, got %#v", expectedSlices)
	}
	if got := strings.Count(logBuffer.String(), stableFrontendCohortWarning); got != 0 {
		t.Fatalf("stable cohort warning count = %d, want 0; logs=%q", got, logBuffer.String())
	}
}

func newFrontendEligibilityTestReconciler(t *testing.T) (*Reconciler, *bytes.Buffer) {
	t.Helper()

	scheme := newScheme(t)
	k8sClient := newInfrastructureClientBuilder(scheme).
		WithObjects(
			&corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nantian-dataplane-stable",
					Namespace: defaultDataplaneNamespace,
					Labels:    map[string]string{"app": "nantian-gw-dataplane"},
				},
				Status: corev1.PodStatus{
					PodIP: "10.0.0.50",
					Conditions: []corev1.PodCondition{{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					}},
				},
			},
		).
		Build()

	store := ir.NewSnapshotStore(discardLogger())
	if !store.Publish(&ir.Snapshot{}) {
		t.Fatal("expected snapshot publish to succeed")
	}

	nodes := nodeinfo.NewRegistry(ir.NewNodeStatusStore(), nil, discardLogger(), nodeinfo.Options{})
	now := time.Now().UTC()
	currentVersion := store.Current().ID
	nodes.Connect(context.Background(), "nantian-dataplane-stable", "kind", nil, now)
	nodes.ObservePublished(context.Background(), "nantian-dataplane-stable", currentVersion, now)
	nodes.ObserveAck(context.Background(), "nantian-dataplane-stable", "kind", "stable-version", "stable-version", nil, now)
	nodes.ObserveReport(context.Background(), "nantian-dataplane-stable", "stable-version", true, "stable", now)

	logBuffer := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logBuffer, nil))
	options := DefaultOptions()
	options.SnapshotStore = store
	options.NodeStatus = nodes
	return NewWithOptions(k8sClient, k8sClient, "gateway.networking.k8s.io/nantian-gw", options, logger), logBuffer
}
