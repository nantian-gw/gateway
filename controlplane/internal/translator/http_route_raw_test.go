package translator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var httpRouteResource = schema.GroupResource{
	Group:    gatewayv1.GroupName,
	Resource: "httproutes",
}

func TestBuildSkipsRawHTTPRouteFetchWithoutCustomRawFilters(t *testing.T) {
	scheme := buildSupportScheme(t)

	standardRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "standard", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{{
				Filters: []gatewayv1.HTTPRouteFilter{{
					Type: gatewayv1.HTTPRouteFilterRequestRedirect,
					RequestRedirect: &gatewayv1.HTTPRequestRedirectFilter{
						StatusCode: ptr(302),
					},
				}},
			}},
		},
	}

	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(standardRoute).
		Build()

	validatingClient := &rawHTTPRouteValidatingClient{
		Client:        baseClient,
		forbidRawList: true,
		forbidRawGet:  true,
	}

	_, err := New(
		"gateway.networking.k8s.io/aether-gateway",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), validatingClient)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
}

func TestBuildLoadsRawHTTPRoutesOnDemandForCORSFilters(t *testing.T) {
	scheme := buildSupportScheme(t)

	corsRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "cors", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{{
				Filters: []gatewayv1.HTTPRouteFilter{{
					Type: gatewayv1.HTTPRouteFilterType("CORS"),
				}},
			}},
		},
	}
	standardRoute := &gatewayv1.HTTPRoute{
		ObjectMeta: metav1.ObjectMeta{Name: "standard", Namespace: "default"},
		Spec: gatewayv1.HTTPRouteSpec{
			Rules: []gatewayv1.HTTPRouteRule{{
				Filters: []gatewayv1.HTTPRouteFilter{{
					Type: gatewayv1.HTTPRouteFilterRequestHeaderModifier,
				}},
			}},
		},
	}

	baseClient := newTranslatorClientBuilder(scheme).
		WithObjects(corsRoute, standardRoute).
		Build()

	validatingClient := &rawHTTPRouteValidatingClient{
		Client:        baseClient,
		forbidRawList: true,
		rawRoutes: map[client.ObjectKey]*unstructured.Unstructured{
			client.ObjectKeyFromObject(corsRoute): rawHTTPRouteWithCORSConfig(t, corsRoute),
		},
	}

	snapshot, err := New(
		"gateway.networking.k8s.io/aether-gateway",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	).Build(context.Background(), validatingClient)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if !reflect.DeepEqual(validatingClient.rawGets, []client.ObjectKey{{Namespace: "default", Name: "cors"}}) {
		t.Fatalf("unexpected raw HTTPRoute gets: %#v", validatingClient.rawGets)
	}
	if len(snapshot.HTTPRoutes) != 2 {
		t.Fatalf("expected 2 HTTPRoutes, got %d", len(snapshot.HTTPRoutes))
	}

	var corsFilter map[string]any
	for _, route := range snapshot.HTTPRoutes {
		if route.Name != "cors" || len(route.Rules) == 0 || len(route.Rules[0].Filters) == 0 {
			continue
		}
		if route.Rules[0].Filters[0].Type != "CORS" {
			t.Fatalf("unexpected filter type: %#v", route.Rules[0].Filters[0])
		}
		corsFilter = route.Rules[0].Filters[0].Config
	}

	if !reflect.DeepEqual(corsFilter["allowMethods"], []any{"GET", "POST"}) {
		t.Fatalf("unexpected CORS allowMethods config: %#v", corsFilter)
	}
	if got := corsFilter["maxAge"]; got != 600 {
		t.Fatalf("unexpected CORS maxAge config: %#v", got)
	}
}

type rawHTTPRouteValidatingClient struct {
	client.Client
	rawRoutes     map[client.ObjectKey]*unstructured.Unstructured
	rawGets       []client.ObjectKey
	forbidRawList bool
	forbidRawGet  bool
}

func (c *rawHTTPRouteValidatingClient) List(
	ctx context.Context,
	list client.ObjectList,
	opts ...client.ListOption,
) error {
	if raw, ok := list.(*unstructured.UnstructuredList); ok && c.forbidRawList &&
		raw.GetKind() == "HTTPRouteList" &&
		raw.GetAPIVersion() == gatewayv1.GroupName+"/v1" {
		return fmt.Errorf("unexpected raw HTTPRoute List")
	}
	return c.Client.List(ctx, list, opts...)
}

func (c *rawHTTPRouteValidatingClient) Get(
	ctx context.Context,
	key client.ObjectKey,
	obj client.Object,
	opts ...client.GetOption,
) error {
	raw, ok := obj.(*unstructured.Unstructured)
	if !ok || raw.GetKind() != "HTTPRoute" || raw.GetAPIVersion() != gatewayv1.GroupName+"/v1" {
		return c.Client.Get(ctx, key, obj, opts...)
	}
	if c.forbidRawGet {
		return fmt.Errorf("unexpected raw HTTPRoute Get for %s/%s", key.Namespace, key.Name)
	}

	c.rawGets = append(c.rawGets, key)
	item := c.rawRoutes[key]
	if item == nil {
		return apierrors.NewNotFound(httpRouteResource, key.Name)
	}

	raw.Object = item.DeepCopy().Object
	raw.SetGroupVersionKind(item.GroupVersionKind())
	return nil
}

func rawHTTPRouteWithCORSConfig(t *testing.T, route *gatewayv1.HTTPRoute) *unstructured.Unstructured {
	t.Helper()

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":      route.Name,
				"namespace": route.Namespace,
			},
			"spec": map[string]any{
				"rules": []any{
					map[string]any{
						"filters": []any{
							map[string]any{
								"type": "CORS",
								"cors": map[string]any{
									"allowMethods": []any{"GET", "POST"},
									"maxAge":       int64(600),
								},
							},
						},
					},
				},
			},
		},
	}
}
