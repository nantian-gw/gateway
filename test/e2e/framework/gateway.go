//go:build e2e

package framework

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	gatewayGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gateways",
	}
	httpRouteGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "httproutes",
	}
	referenceGrantGVR = schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1beta1",
		Resource: "referencegrants",
	}
)

type HTTPRouteOption func(route *unstructured.Unstructured)

func DynamicClient() (dynamic.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client config: %w", err)
	}
	return dynamic.NewForConfig(config)
}

func CreateGateway(t T, name, gwClass string) {
	t.Helper()

	dc, err := DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	gw := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ControlPlaneNS,
			},
			"spec": map[string]interface{}{
				"gatewayClassName": gwClass,
				"listeners": []interface{}{
					map[string]interface{}{
						"name":     "http",
						"port":     int64(80),
						"protocol": "HTTP",
					},
				},
			},
		},
	}

	ctx := context.Background()
	_, err = dc.Resource(gatewayGVR).Namespace(ControlPlaneNS).Create(ctx, gw, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create gateway %s: %v", name, err)
	}
	t.Logf("created Gateway %s in %s", name, ControlPlaneNS)

	waitForGatewayReady(t, dc, name)
}

func waitForGatewayReady(t T, dc dynamic.Interface, name string) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(120 * time.Second)

	for time.Now().Before(deadline) {
		gw, err := dc.Resource(gatewayGVR).Namespace(ControlPlaneNS).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		conditions, found, err := unstructured.NestedSlice(gw.Object, "status", "conditions")
		if !found || err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		accepted := false
		programmed := false
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			condType, _, _ := unstructured.NestedString(cond, "type")
			condStatus, _, _ := unstructured.NestedString(cond, "status")
			if condType == "Accepted" && condStatus == "True" {
				accepted = true
			}
			if condType == "Programmed" && condStatus == "True" {
				programmed = true
			}
		}

		if accepted && programmed {
			t.Logf("Gateway %s is accepted and programmed", name)
			return
		}
		time.Sleep(2 * time.Second)
	}

	t.Fatalf("Gateway %s did not become ready within timeout", name)
}

func WithRule(path, pathType, svcName string, svcPort int32) HTTPRouteOption {
	return func(route *unstructured.Unstructured) {
		rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
		rule := map[string]interface{}{
			"matches": []interface{}{
				map[string]interface{}{
					"path": map[string]interface{}{
						"type":  pathType,
						"value": path,
					},
				},
			},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name": svcName,
					"port": int64(svcPort),
				},
			},
		}
		rules = append(rules, rule)
		unstructured.SetNestedSlice(route.Object, rules, "spec", "rules")
	}
}

func WithRuleMatchHeaders(path, pathType, svcName string, svcPort int32, headers map[string]string) HTTPRouteOption {
	return func(route *unstructured.Unstructured) {
		match := map[string]interface{}{
			"path": map[string]interface{}{
				"type":  pathType,
				"value": path,
			},
		}
		headerMatches := make([]interface{}, 0, len(headers))
		for k, v := range headers {
			headerMatches = append(headerMatches, map[string]interface{}{
				"name":  k,
				"value": v,
			})
		}
		if len(headerMatches) > 0 {
			match["headers"] = headerMatches
		}

		rule := map[string]interface{}{
			"matches": []interface{}{match},
			"backendRefs": []interface{}{
				map[string]interface{}{
					"name": svcName,
					"port": int64(svcPort),
				},
			},
		}

		rules, _, _ := unstructured.NestedSlice(route.Object, "spec", "rules")
		rules = append(rules, rule)
		unstructured.SetNestedSlice(route.Object, rules, "spec", "rules")
	}
}

func CreateHTTPRoute(t T, ns, name string, parentGatewayName string, opts ...HTTPRouteOption) {
	t.Helper()

	dc, err := DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	route := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"name":      parentGatewayName,
						"namespace": ControlPlaneNS,
					},
				},
				"rules": []interface{}{},
			},
		},
	}

	for _, o := range opts {
		o(route)
	}

	ctx := context.Background()
	_, err = dc.Resource(httpRouteGVR).Namespace(ns).Create(ctx, route, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create HTTPRoute %s/%s: %v", ns, name, err)
	}
	t.Logf("created HTTPRoute %s/%s", ns, name)
}

func CreateReferenceGrant(t T, ns, name, fromNS string) {
	t.Helper()

	dc, err := DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}

	grant := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1beta1",
			"kind":       "ReferenceGrant",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]interface{}{
				"from": []interface{}{
					map[string]interface{}{
						"group":     "gateway.networking.k8s.io",
						"kind":      "HTTPRoute",
						"namespace": fromNS,
					},
				},
				"to": []interface{}{
					map[string]interface{}{
						"group": "",
						"kind":  "Service",
					},
				},
			},
		},
	}

	ctx := context.Background()
	_, err = dc.Resource(referenceGrantGVR).Namespace(ns).Create(ctx, grant, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create ReferenceGrant %s/%s: %v", ns, name, err)
	}
	t.Logf("created ReferenceGrant %s/%s", ns, name)
}

func CleanupResource(t T, gvr schema.GroupVersionResource, ns, name string) {
	t.Helper()

	dc, err := DynamicClient()
	if err != nil {
		t.Logf("cleanup: create dynamic client: %v", err)
		return
	}

	ctx := context.Background()
	err = dc.Resource(gvr).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		t.Logf("cleanup: delete %s/%s (%s): %v", ns, name, gvr.Resource, err)
		return
	}
	t.Logf("cleaned up %s/%s (%s)", ns, name, gvr.Resource)

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		_, err := dc.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Logf("resource %s/%s may still exist after cleanup timeout", ns, name)
}
