package translator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

const (
	testGatewayClassControllerNameIndex = "nantian.dev/infrastructure.gatewayclass.controller-name"
	testGatewayGatewayClassNameIndex    = "nantian.dev/infrastructure.gateway.gatewayclass-name"
)

func TestLoadFilteredGatewaysUsesFieldIndexes(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, testGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok {
				return nil
			}
			if gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, testGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok {
				return nil
			}
			if gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "other"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "public",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ignored",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "other",
				},
			},
		).
		Build()

	xlator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	gateways, err := xlator.loadFilteredGateways(
		context.Background(),
		scopedGatewayQueryValidatingClient{
			Client:         baseClient,
			controllerName: string(controllerName),
			classNames:     map[string]struct{}{"nantian-gw": {}},
		},
	)
	if err != nil {
		t.Fatalf("loadFilteredGateways returned error: %v", err)
	}
	if len(gateways) != 1 {
		t.Fatalf("filtered gateway count = %d, want 1", len(gateways))
	}
	if gateways[0].Namespace != "default" || gateways[0].Name != "public" {
		t.Fatalf("unexpected filtered gateways: %#v", gateways)
	}
}

func TestLoadFilteredGatewaysSkipsGatewayListWhenNoManagedGatewayClassesExist(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, testGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok {
				return nil
			}
			if gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "other"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foreign",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "other",
				},
			},
		).
		Build()

	xlator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	gateways, err := xlator.loadFilteredGateways(
		context.Background(),
		noManagedGatewayClassesClient{
			Client:         baseClient,
			controllerName: string(controllerName),
		},
	)
	if err != nil {
		t.Fatalf("loadFilteredGateways returned error: %v", err)
	}
	if len(gateways) != 0 {
		t.Fatalf("filtered gateway count = %d, want 0", len(gateways))
	}
}

func TestRebuildAttachmentsForNamespacesLoadsReferencedGatewaysDirectly(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gatewayv1.GatewayClass{}, testGatewayClassControllerNameIndex, func(object client.Object) []string {
			gatewayClass, ok := object.(*gatewayv1.GatewayClass)
			if !ok {
				return nil
			}
			if gatewayClass.Spec.ControllerName == "" {
				return nil
			}
			return []string{string(gatewayClass.Spec.ControllerName)}
		}).
		WithIndex(&gatewayv1.Gateway{}, testGatewayGatewayClassNameIndex, func(object client.Object) []string {
			gateway, ok := object.(*gatewayv1.Gateway)
			if !ok {
				return nil
			}
			if gateway.Spec.GatewayClassName == "" {
				return nil
			}
			return []string{string(gateway.Spec.GatewayClassName)}
		}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "other"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ignored",
					Namespace: "other",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
				},
			},
		).
		Build()

	current := &ir.Snapshot{
		Listeners: []ir.Listener{{
			Name:           "default/gw/http",
			AttachedRoutes: []string{"default/route"},
		}},
		HTTPRoutes: []ir.HTTPRoute{{
			Name:      "route",
			Namespace: "default",
			ParentRefs: []ir.ParentRef{{
				Name: "gw",
			}},
		}},
	}

	validatingClient := &attachmentGatewayQueryValidatingClient{
		Client:         baseClient,
		controllerName: string(controllerName),
	}
	xlator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	listeners, err := xlator.RebuildAttachmentsForNamespaces(
		context.Background(),
		validatingClient,
		current,
		[]string{"default"},
	)
	if err != nil {
		t.Fatalf("RebuildAttachmentsForNamespaces returned error: %v", err)
	}
	if len(listeners) != 1 {
		t.Fatalf("listener count = %d, want 1", len(listeners))
	}
	if got := listeners[0].AttachedRoutes; len(got) != 1 || got[0] != "default/route" {
		t.Fatalf("unexpected attached routes: %#v", got)
	}
	if len(validatingClient.gatewayGets) != 1 {
		t.Fatalf("gateway Get count = %d, want 1", len(validatingClient.gatewayGets))
	}
	if validatingClient.gatewayGets[0] != (client.ObjectKey{Namespace: "default", Name: "gw"}) {
		t.Fatalf("unexpected gateway Get keys: %#v", validatingClient.gatewayGets)
	}
}

func TestBuildGatewayListenersForSnapshotLoadsReferencedGatewayClassesDirectly(t *testing.T) {
	scheme := buildSupportScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/nantian-gw")

	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "nantian-gw"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "other"},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: gatewayv1.GatewayController("example.com/other"),
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gw",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "nantian-gw",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foreign",
					Namespace: "default",
				},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "other",
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     8080,
					}},
				},
			},
		).
		Build()

	current := &ir.Snapshot{
		Listeners: []ir.Listener{
			{Name: "default/gw/http"},
			{Name: "default/foreign/http"},
		},
	}

	validatingClient := &gatewayListenerClassQueryValidatingClient{Client: baseClient}
	xlator := New(
		string(controllerName),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	next, err := xlator.BuildGatewayListenersForSnapshot(
		context.Background(),
		validatingClient,
		current,
		[]client.ObjectKey{
			{Namespace: "default", Name: "gw"},
			{Namespace: "default", Name: "foreign"},
		},
	)
	if err != nil {
		t.Fatalf("BuildGatewayListenersForSnapshot returned error: %v", err)
	}

	if len(next.Listeners) != 1 {
		t.Fatalf("listener count = %d, want 1 managed listener", len(next.Listeners))
	}
	if next.Listeners[0].Name != "default/gw/http" {
		t.Fatalf("unexpected listeners: %#v", next.Listeners)
	}
	if len(validatingClient.gatewayClassGets) != 2 {
		t.Fatalf("GatewayClass Get count = %d, want 2", len(validatingClient.gatewayClassGets))
	}
}

type scopedGatewayQueryValidatingClient struct {
	client.Client
	controllerName string
	classNames     map[string]struct{}
}

func (c scopedGatewayQueryValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	var listOptions client.ListOptions
	for _, opt := range opts {
		opt.ApplyToList(&listOptions)
	}

	switch list.(type) {
	case *gatewayv1.GatewayClassList:
		if listOptions.FieldSelector == nil || listOptions.FieldSelector.Empty() {
			return fmt.Errorf("GatewayClass list must include controllerName field selector")
		}
		if !listOptions.FieldSelector.Matches(fields.Set{
			testGatewayClassControllerNameIndex: c.controllerName,
		}) {
			return fmt.Errorf("GatewayClass field selector %q does not match controllerName index", listOptions.FieldSelector.String())
		}
	case *gatewayv1.GatewayList:
		if listOptions.FieldSelector == nil || listOptions.FieldSelector.Empty() {
			return fmt.Errorf("Gateway list must include gatewayClassName field selector")
		}
		matched := false
		for className := range c.classNames {
			if listOptions.FieldSelector.Matches(fields.Set{
				testGatewayGatewayClassNameIndex: className,
			}) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("Gateway field selector %q does not match expected class index", listOptions.FieldSelector.String())
		}
	}

	return c.Client.List(ctx, list, opts...)
}

type noManagedGatewayClassesClient struct {
	client.Client
	controllerName string
}

func (c noManagedGatewayClassesClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	var listOptions client.ListOptions
	for _, opt := range opts {
		opt.ApplyToList(&listOptions)
	}

	switch list.(type) {
	case *gatewayv1.GatewayClassList:
		if listOptions.FieldSelector == nil || listOptions.FieldSelector.Empty() {
			return fmt.Errorf("GatewayClass list must include controllerName field selector")
		}
		if !listOptions.FieldSelector.Matches(fields.Set{
			testGatewayClassControllerNameIndex: c.controllerName,
		}) {
			return fmt.Errorf("GatewayClass field selector %q does not match controllerName index", listOptions.FieldSelector.String())
		}
	case *gatewayv1.GatewayList:
		return fmt.Errorf("loadFilteredGateways should not list Gateways when no managed GatewayClasses exist")
	}

	return c.Client.List(ctx, list, opts...)
}

type attachmentGatewayQueryValidatingClient struct {
	client.Client
	controllerName string
	gatewayGets    []client.ObjectKey
}

func (c *attachmentGatewayQueryValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	var listOptions client.ListOptions
	for _, opt := range opts {
		opt.ApplyToList(&listOptions)
	}

	switch list.(type) {
	case *gatewayv1.GatewayClassList:
		if listOptions.FieldSelector == nil || listOptions.FieldSelector.Empty() {
			return fmt.Errorf("GatewayClass list must include controllerName field selector")
		}
		if !listOptions.FieldSelector.Matches(fields.Set{
			testGatewayClassControllerNameIndex: c.controllerName,
		}) {
			return fmt.Errorf("GatewayClass field selector %q does not match controllerName index", listOptions.FieldSelector.String())
		}
	case *gatewayv1.GatewayList:
		return fmt.Errorf("RebuildAttachmentsForNamespaces should not list Gateways")
	}

	return c.Client.List(ctx, list, opts...)
}

func (c *attachmentGatewayQueryValidatingClient) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	if _, ok := obj.(*gatewayv1.Gateway); ok {
		c.gatewayGets = append(c.gatewayGets, key)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

type gatewayListenerClassQueryValidatingClient struct {
	client.Client
	gatewayClassGets []client.ObjectKey
}

func (c *gatewayListenerClassQueryValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	switch list.(type) {
	case *gatewayv1.GatewayClassList:
		return fmt.Errorf("BuildGatewayListenersForSnapshot should not list GatewayClasses")
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *gatewayListenerClassQueryValidatingClient) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	if _, ok := obj.(*gatewayv1.GatewayClass); ok {
		c.gatewayClassGets = append(c.gatewayClassGets, key)
	}
	return c.Client.Get(ctx, key, obj, opts...)
}
