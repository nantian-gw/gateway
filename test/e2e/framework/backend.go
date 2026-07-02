//go:build e2e

package framework

import (
	"context"
	"fmt"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func EchoImage() string {
	if img := os.Getenv("ECHO_IMAGE"); img != "" {
		return img
	}
	return "nantian-echo:latest"
}

func ClientSet() (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client config: %w", err)
	}
	return kubernetes.NewForConfig(config)
}

func DeployEchoBackend(t T, ns, name string) {
	t.Helper()

	clientset, err := ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()
	replicas := int32(1)
	labels := map[string]string{"app": name}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "echo",
							Image:           EchoImage(),
							ImagePullPolicy: corev1.PullNever,
							Ports: []corev1.ContainerPort{
								{ContainerPort: 8080, Name: "http"},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(ns).Create(ctx, deploy, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create deployment %s/%s: %v", ns, name, err)
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt32(8080),
				},
			},
		},
	}

	_, err = clientset.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create service %s/%s: %v", ns, name, err)
	}

	t.Logf("deployed echo backend %s/%s", ns, name)
}

func WaitForBackendReady(t T, ns, name string) {
	t.Helper()

	clientset, err := ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()
	deadline := time.Now().Add(120 * time.Second)

	for time.Now().Before(deadline) {
		deploy, err := clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		if deploy.Status.ReadyReplicas >= 1 {
			t.Logf("backend %s/%s is ready (%d/%d replicas)", ns, name, deploy.Status.ReadyReplicas, deploy.Status.Replicas)
			return
		}

		if deploy.Status.Replicas > 0 && deploy.Status.ReadyReplicas == 0 {
			t.Logf("backend %s/%s: %d replicas, %d ready, %d available, %d unavailable",
				ns, name, deploy.Status.Replicas, deploy.Status.ReadyReplicas,
				deploy.Status.AvailableReplicas, deploy.Status.UnavailableReplicas)
		}

		time.Sleep(2 * time.Second)
	}

	// Log pod status on timeout for debugging
	pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", name),
	})
	if err == nil {
		for _, pod := range pods.Items {
			t.Logf("pod %s/%s: phase=%s", ns, pod.Name, pod.Status.Phase)
			for _, cs := range pod.Status.ContainerStatuses {
				t.Logf("  container %s: ready=%v, restartCount=%d, state=%v",
					cs.Name, cs.Ready, cs.RestartCount, cs.State)
			}
		}
	} else {
		t.Logf("failed to list pods for %s/%s: %v", ns, name, err)
	}

	t.Fatalf("backend %s/%s did not become ready within timeout", ns, name)
}

func ScaleBackendToZero(t T, ns, name string) {
	t.Helper()

	clientset, err := ClientSet()
	if err != nil {
		t.Fatalf("create clientset: %v", err)
	}

	ctx := context.Background()
	replicas := int32(0)

	deploy, err := clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get deployment %s/%s: %v", ns, name, err)
	}

	deploy.Spec.Replicas = &replicas
	_, err = clientset.AppsV1().Deployments(ns).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("scale deployment %s/%s to zero: %v", ns, name, err)
	}

	t.Logf("scaled backend %s/%s to 0 replicas", ns, name)
}
