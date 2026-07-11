package chatbot

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestRenderContext_TwoSections(t *testing.T) {
	index := ClusterIndex{Entries: []IndexEntry{
		{Ref: ResourceRef{Kind: kindGateway, Namespace: "default", Name: "public"}, Summary: "class=nantian-gw listeners=1", StatusSummary: "Programmed=True"},
	}}
	selected := []ResourceRef{{Kind: kindGateway, Namespace: "default", Name: "public"}}
	out := renderContext(index, selected, 4000, false)
	if !strings.Contains(out, "## Topology Index") {
		t.Error("missing index section")
	}
	if !strings.Contains(out, "## Relevant Resources") {
		t.Error("missing detail section")
	}
	if !strings.Contains(out, "public") {
		t.Error("missing resource")
	}
}

func TestRenderContext_BudgetTruncation(t *testing.T) {
	var entries []IndexEntry
	var selected []ResourceRef
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("route-%d", i)
		entries = append(entries, IndexEntry{Ref: ResourceRef{Kind: kindHTTPRoute, Namespace: "default", Name: name}, Summary: "rules=1"})
		selected = append(selected, ResourceRef{Kind: kindHTTPRoute, Namespace: "default", Name: name})
	}
	index := ClusterIndex{Entries: entries}
	out := renderContext(index, selected, 500, false)
	if len(out) > 1500 {
		t.Errorf("output %d exceeds budget-ish bound", len(out))
	}
	if !strings.Contains(out, "truncated") {
		t.Error("expected truncation notice")
	}
}

func TestRenderContext_DetailExpanded(t *testing.T) {
	ref := ResourceRef{Kind: kindService, Namespace: "default", Name: "svc"}
	index := ClusterIndex{
		Entries: []IndexEntry{{Ref: ref, Summary: "type=ClusterIP, ports=[80]"}},
		objects: map[ResourceRef]client.Object{ref: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.5",
				Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(8080)}},
			},
		}},
	}
	out := renderContext(index, []ResourceRef{ref}, 4000, false)
	if !strings.Contains(out, "clusterIP=10.0.0.5") || !strings.Contains(out, "8080") {
		t.Errorf("detail section should contain expanded fields:\n%s", out)
	}
}

func TestRenderContext_PerBlockCap(t *testing.T) {
	ports := make([]corev1.ServicePort, 0, 500)
	for i := 0; i < 500; i++ {
		ports = append(ports, corev1.ServicePort{Name: "p", Port: int32(1000 + i), TargetPort: intstr.FromInt32(int32(2000 + i))})
	}
	ref := ResourceRef{Kind: kindService, Namespace: "default", Name: "big"}
	index := ClusterIndex{
		Entries: []IndexEntry{{Ref: ref, Summary: "big"}},
		objects: map[ResourceRef]client.Object{ref: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "big", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Ports: ports},
		}},
	}
	out := renderContext(index, []ResourceRef{ref}, 100000, false)
	if !strings.Contains(out, "…(truncated)") {
		t.Errorf("oversized block should be capped, got len=%d", len(out))
	}
}
