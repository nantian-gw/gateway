package translator

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/nantian-gw/gateway/controlplane/internal/ir"
)

func (t *Translator) BuildWorkloadsForSnapshot(
	ctx context.Context,
	cl client.Client,
	current *ir.Snapshot,
) ([]ir.Workload, error) {
	if current == nil {
		return nil, nil
	}

	pods, err := loadPodsForNamespaces(ctx, cl, meshWorkloadNamespacesFromSnapshot(current))
	if err != nil {
		return nil, err
	}
	return translateWorkloads(pods), nil
}

func translateWorkloads(pods []corev1.Pod) []ir.Workload {
	out := make([]ir.Workload, 0, len(pods))
	for _, pod := range pods {
		if pod.Status.PodIP == "" {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}

		out = append(out, ir.Workload{
			Namespace: pod.Namespace,
			Name:      pod.Name,
			IP:        pod.Status.PodIP,
		})
	}
	return out
}
