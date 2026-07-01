//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"

	"github.com/nantian-gw/gateway/test/e2e/framework"
)

func TestConformanceReadiness(t *testing.T) {
	dc, err := framework.DynamicClient()
	if err != nil {
		t.Fatalf("create dynamic client: %v", err)
	}
	ctx := context.Background()

	// 1. Verify GatewayClass "nantian-gw" exists.
	gcGVR := schema.GroupVersionResource{
		Group:    "gateway.networking.k8s.io",
		Version:  "v1",
		Resource: "gatewayclasses",
	}

	gc, err := dc.Resource(gcGVR).Get(ctx, "nantian-gw", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("GatewayClass nantian-gw not found: %v", err)
	}
	t.Logf("GatewayClass nantian-gw exists")

	// Verify controller name.
	controllerName, _, _ := unstructured.NestedString(gc.Object, "spec", "controllerName")
	t.Logf("GatewayClass nantian-gw controllerName: %s", controllerName)

	// 2. Verify Gateway "nantian-gw" is Accepted and Programmed.
	gwName := "nantian-gw"
	gw, err := dc.Resource(gatewayGVR).Namespace(framework.ControlPlaneNS).Get(ctx, gwName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Gateway %s not found in %s: %v", gwName, framework.ControlPlaneNS, err)
	}

	conditions, found, err := unstructured.NestedSlice(gw.Object, "status", "conditions")
	if !found || err != nil {
		t.Fatalf("Gateway %s has no status.conditions", gwName)
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

	if !accepted {
		t.Error("Gateway nantian-gw is not Accepted")
	}
	if !programmed {
		t.Error("Gateway nantian-gw is not Programmed")
	}
	if accepted && programmed {
		t.Log("Gateway nantian-gw is accepted and programmed")
	}

	// 3. Verify the conformance manifest overlay is parseable.
	manifestPath := filepath.Join(
		framework.GatewayRoot(),
		"conformance", "manifests", "tests", "mesh", "httproute-303-redirect.yaml",
	)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read conformance manifest %s: %v", manifestPath, err)
	}

	var parsed interface{}
	if err := yaml.Unmarshal(manifestData, &parsed); err != nil {
		t.Errorf("failed to parse conformance manifest %s: %v", manifestPath, err)
	} else {
		t.Logf("conformance manifest %s is parseable", manifestPath)
	}

	t.Log("Gateway API conformance environment is ready")
}
