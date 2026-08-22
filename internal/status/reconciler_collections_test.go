package status

import (
	"context"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestStatusBatchWorkerLimitUsesReconcilerOptions(t *testing.T) {
	if got := (&Reconciler{}).statusBatchWorkerLimit(); got != defaultMaxConcurrentReconciles {
		t.Fatalf("zero-value worker limit = %d, want %d", got, defaultMaxConcurrentReconciles)
	}

	reconciler := &Reconciler{options: Options{MaxConcurrentReconciles: 2}}
	if got := reconciler.statusBatchWorkerLimit(); got != 2 {
		t.Fatalf("configured worker limit = %d, want 2", got)
	}
}

func TestReconcileGatewaysUsesConfiguredStatusBatchWorkerLimit(t *testing.T) {
	scheme := newScheme(t)
	gateways := []gatewayv1.Gateway{
		statusBatchGateway("gw-1"),
		statusBatchGateway("gw-2"),
		statusBatchGateway("gw-3"),
	}
	objects := make([]client.Object, 0, len(gateways))
	for i := range gateways {
		objects = append(objects, &gateways[i])
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.Gateway{}).
		WithObjects(objects...).
		Build()
	entered := make(chan struct{}, len(gateways))
	release := make(chan struct{})
	gate := &statusBatchGateWriter{
		delegate: baseClient.Status(),
		entered:  entered,
		release:  release,
	}
	k8sClient := statusBatchGateClient{Client: baseClient, writer: gate}
	reconciler := NewWithAddressesAndReaderOptions(
		k8sClient,
		k8sClient,
		"gateway.networking.k8s.io/nantian-gw",
		[]string{"127.0.0.1"},
		discardLogger(),
		Options{EnableExperimentalGateway: true, MaxConcurrentReconciles: 2},
	)

	evals := make(map[client.ObjectKey]gatewayEvaluation, len(gateways))
	for i := range gateways {
		evals[client.ObjectKeyFromObject(&gateways[i])] = statusBatchGatewayEvaluation()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- reconciler.reconcileGateways(ctx, gateways, evals)
	}()

	waitForStatusBatchEntries(t, entered, 2)
	assertNoStatusBatchEntry(t, entered)
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("reconcileGateways returned error: %v", err)
	}
	if got := gate.maxActive(); got != 2 {
		t.Fatalf("max concurrent status updates = %d, want 2", got)
	}
}

func TestReconcileListenerSetStatusesUsesConfiguredStatusBatchWorkerLimit(t *testing.T) {
	scheme := newScheme(t)
	listenerSets := []gatewayv1.ListenerSet{
		statusBatchListenerSet("ls-1"),
		statusBatchListenerSet("ls-2"),
		statusBatchListenerSet("ls-3"),
	}
	objects := make([]client.Object, 0, len(listenerSets))
	for i := range listenerSets {
		objects = append(objects, &listenerSets[i])
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.ListenerSet{}).
		WithObjects(objects...).
		Build()
	entered := make(chan struct{}, len(listenerSets))
	release := make(chan struct{})
	gate := &statusBatchGateWriter{
		delegate: baseClient.Status(),
		entered:  entered,
		release:  release,
	}
	k8sClient := statusBatchGateClient{Client: baseClient, writer: gate}
	reconciler := NewWithAddressesAndReaderOptions(
		k8sClient,
		k8sClient,
		"gateway.networking.k8s.io/nantian-gw",
		[]string{"127.0.0.1"},
		discardLogger(),
		Options{EnableExperimentalGateway: true, MaxConcurrentReconciles: 2},
	)

	evals := make(map[string]listenerSetEvaluation, len(listenerSets))
	for i := range listenerSets {
		evals[listenerSets[i].Namespace+"/"+listenerSets[i].Name] = statusBatchListenerSetEvaluation()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- reconciler.reconcileListenerSetStatuses(ctx, listenerSets, evals)
	}()

	waitForStatusBatchEntries(t, entered, 2)
	assertNoStatusBatchEntry(t, entered)
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("reconcileListenerSetStatuses returned error: %v", err)
	}
	if got := gate.maxActive(); got != 2 {
		t.Fatalf("max concurrent status updates = %d, want 2", got)
	}
}

func statusBatchGateway(name string) gatewayv1.Gateway {
	gateway := gatewayv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gatewayv1.GatewaySpec{
			GatewayClassName: "nantian-gw",
		},
	}
	setCondition(&gateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionAccepted),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonAccepted),
		Message:            "Gateway is accepted",
		ObservedGeneration: 1,
	})
	setCondition(&gateway.Status.Conditions, conditionSpec{
		Type:               string(gatewayv1.GatewayConditionProgrammed),
		Status:             metav1.ConditionTrue,
		Reason:             string(gatewayv1.GatewayReasonProgrammed),
		Message:            "Gateway is programmed",
		ObservedGeneration: 1,
	})
	return gateway
}

func statusBatchGatewayEvaluation() gatewayEvaluation {
	return gatewayEvaluation{
		sourceGeneration: 1,
		addresses: []gatewayv1.GatewayStatusAddress{{
			Value: "127.0.0.1",
		}},
		acceptedCondition: conditionSpec{
			Type:    string(gatewayv1.GatewayConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayv1.GatewayReasonAccepted),
			Message: "Gateway is accepted",
		},
		programmedCondition: conditionSpec{
			Type:    string(gatewayv1.GatewayConditionProgrammed),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayv1.GatewayReasonProgrammed),
			Message: "Gateway is programmed",
		},
	}
}

func statusBatchListenerSet(name string) gatewayv1.ListenerSet {
	return gatewayv1.ListenerSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  "default",
			Generation: 1,
		},
		Spec: gatewayv1.ListenerSetSpec{
			ParentRef: gatewayv1.ParentGatewayReference{Name: "gw"},
			Listeners: []gatewayv1.ListenerEntry{{
				Name:     "http",
				Protocol: gatewayv1.HTTPProtocolType,
				Port:     80,
			}},
		},
	}
}

func statusBatchListenerSetEvaluation() listenerSetEvaluation {
	return listenerSetEvaluation{
		accepted: conditionSpec{
			Type:    string(gatewayv1.ListenerSetConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayv1.ListenerSetReasonAccepted),
			Message: "ListenerSet is accepted",
		},
		programmed: conditionSpec{
			Type:    string(gatewayv1.ListenerSetConditionProgrammed),
			Status:  metav1.ConditionTrue,
			Reason:  string(gatewayv1.ListenerSetReasonProgrammed),
			Message: "ListenerSet listeners are programmed",
		},
	}
}

func waitForStatusBatchEntries(t *testing.T, entered <-chan struct{}, want int) {
	t.Helper()

	timeout := time.After(2 * time.Second)
	for i := 0; i < want; i++ {
		select {
		case <-entered:
		case <-timeout:
			t.Fatalf("timed out waiting for status update %d of %d", i+1, want)
		}
	}
}

func assertNoStatusBatchEntry(t *testing.T, entered <-chan struct{}) {
	t.Helper()

	select {
	case <-entered:
		t.Fatal("status batch started more workers than configured")
	case <-time.After(100 * time.Millisecond):
	}
}

type statusBatchGateClient struct {
	client.Client
	writer client.SubResourceWriter
}

func (c statusBatchGateClient) Status() client.SubResourceWriter {
	return c.writer
}

type statusBatchGateWriter struct {
	delegate client.SubResourceWriter
	entered  chan<- struct{}
	release  <-chan struct{}

	mu        sync.Mutex
	active    int
	maxSeen   int
	delegateM sync.Mutex
}

func (w *statusBatchGateWriter) Create(
	ctx context.Context,
	obj client.Object,
	subResource client.Object,
	opts ...client.SubResourceCreateOption,
) error {
	return w.delegate.Create(ctx, obj, subResource, opts...)
}

func (w *statusBatchGateWriter) Update(
	ctx context.Context,
	obj client.Object,
	opts ...client.SubResourceUpdateOption,
) error {
	if err := w.enter(ctx); err != nil {
		return err
	}
	w.delegateM.Lock()
	defer w.delegateM.Unlock()
	return w.delegate.Update(ctx, obj, opts...)
}

func (w *statusBatchGateWriter) Patch(
	ctx context.Context,
	obj client.Object,
	patch client.Patch,
	opts ...client.SubResourcePatchOption,
) error {
	if err := w.enter(ctx); err != nil {
		return err
	}
	w.delegateM.Lock()
	defer w.delegateM.Unlock()
	return w.delegate.Patch(ctx, obj, patch, opts...)
}

func (w *statusBatchGateWriter) Apply(
	ctx context.Context,
	obj runtime.ApplyConfiguration,
	opts ...client.SubResourceApplyOption,
) error {
	return w.delegate.Apply(ctx, obj, opts...)
}

func (w *statusBatchGateWriter) enter(ctx context.Context) error {
	w.mu.Lock()
	w.active++
	if w.active > w.maxSeen {
		w.maxSeen = w.active
	}
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		w.active--
		w.mu.Unlock()
	}()

	select {
	case w.entered <- struct{}{}:
	default:
	}

	select {
	case <-w.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *statusBatchGateWriter) maxActive() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxSeen
}
