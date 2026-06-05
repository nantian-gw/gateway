package status

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func TestReconcileRejectsCrossNamespaceBackendTLSWithoutReferenceGrant(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					TLS: &gatewayv1.GatewayTLSConfig{
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name:      "client-cert",
								Namespace: namespacePtr("shared"),
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.ListenerReasonRefNotPermitted),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonRefNotPermitted),
		1,
	)
}

func TestReconcileAcceptsCrossNamespaceBackendTLSWithReferenceGrant(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					TLS: &gatewayv1.GatewayTLSConfig{
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name:      "client-cert",
								Namespace: namespacePtr("shared"),
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "client-cert", Namespace: "shared"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
			&gatewayv1beta1.ReferenceGrant{
				ObjectMeta: metav1.ObjectMeta{Name: "allow-default-gateway", Namespace: "shared"},
				Spec: gatewayv1beta1.ReferenceGrantSpec{
					From: []gatewayv1beta1.ReferenceGrantFrom{{
						Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
						Kind:      gatewayv1beta1.Kind("Gateway"),
						Namespace: gatewayv1beta1.Namespace("default"),
					}},
					To: []gatewayv1beta1.ReferenceGrantTo{{
						Group: gatewayv1beta1.Group(""),
						Kind:  gatewayv1beta1.Kind("Secret"),
						Name:  ptr[gatewayv1beta1.ObjectName]("client-cert"),
					}},
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	if len(gateway.Status.Listeners) != 1 {
		t.Fatalf("expected 1 listener status, got %d", len(gateway.Status.Listeners))
	}

	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonResolvedRefs),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionResolvedRefs),
		metav1.ConditionTrue,
		string(gatewayv1.GatewayReasonResolvedRefs),
		1,
	)
}

func TestReconcileRefreshesBackendTLSCertificateRefsWhenReferenceGrantChanges(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shared"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					TLS: &gatewayv1.GatewayTLSConfig{
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name:      "client-cert",
								Namespace: namespacePtr("shared"),
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "client-cert", Namespace: "shared"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": []byte(readStatusTLSAsset(t, "client.key")),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("initial Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.ListenerReasonRefNotPermitted),
		1,
	)

	grant := &gatewayv1beta1.ReferenceGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "allow-default-gateway", Namespace: "shared"},
		Spec: gatewayv1beta1.ReferenceGrantSpec{
			From: []gatewayv1beta1.ReferenceGrantFrom{{
				Group:     gatewayv1beta1.Group(gatewayv1.GroupName),
				Kind:      gatewayv1beta1.Kind("Gateway"),
				Namespace: gatewayv1beta1.Namespace("default"),
			}},
			To: []gatewayv1beta1.ReferenceGrantTo{{
				Group: gatewayv1beta1.Group(""),
				Kind:  gatewayv1beta1.Kind("Secret"),
				Name:  ptr[gatewayv1beta1.ObjectName]("client-cert"),
			}},
		},
	}
	if err := k8sClient.Create(context.Background(), grant); err != nil {
		t.Fatalf("Create ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionTrue,
		string(gatewayv1.ListenerReasonResolvedRefs),
		1,
	)

	if err := k8sClient.Delete(context.Background(), grant); err != nil {
		t.Fatalf("Delete ReferenceGrant returned error: %v", err)
	}

	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile returned error: %v", err)
	}

	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.ListenerReasonRefNotPermitted),
		1,
	)
}

func TestReconcileRejectsBackendTLSMismatchedSecret(t *testing.T) {
	scheme := newScheme(t)
	controllerName := gatewayv1.GatewayController("gateway.networking.k8s.io/aether-gateway")

	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&gatewayv1.GatewayClass{}, &gatewayv1.Gateway{}).
		WithObjects(
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
			&gatewayv1.GatewayClass{
				ObjectMeta: metav1.ObjectMeta{Name: "aether-gateway", Generation: 1},
				Spec: gatewayv1.GatewayClassSpec{
					ControllerName: controllerName,
				},
			},
			&gatewayv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{Name: "gw", Namespace: "default", Generation: 1},
				Spec: gatewayv1.GatewaySpec{
					GatewayClassName: "aether-gateway",
					TLS: &gatewayv1.GatewayTLSConfig{
						Backend: &gatewayv1.GatewayBackendTLS{
							ClientCertificateRef: &gatewayv1.SecretObjectReference{
								Name: "client-cert",
							},
						},
					},
					Listeners: []gatewayv1.Listener{{
						Name:     "http",
						Protocol: gatewayv1.HTTPProtocolType,
						Port:     80,
					}},
				},
			},
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "client-cert", Namespace: "default"},
				Type:       corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte(readStatusTLSAsset(t, "client.crt")),
					"tls.key": readBackendTLSAsset(t, "server-san.key"),
				},
			},
		).
		Build()

	reconciler := New(k8sClient, string(controllerName), "127.0.0.1", discardLogger())
	if err := reconciler.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	var gateway gatewayv1.Gateway
	if err := k8sClient.Get(context.Background(), client.ObjectKey{Namespace: "default", Name: "gw"}, &gateway); err != nil {
		t.Fatalf("Get Gateway returned error: %v", err)
	}
	assertCondition(
		t,
		gateway.Status.Listeners[0].Conditions,
		string(gatewayv1.ListenerConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.ListenerReasonInvalidCertificateRef),
		1,
	)
	assertCondition(
		t,
		gateway.Status.Conditions,
		string(gatewayv1.GatewayConditionResolvedRefs),
		metav1.ConditionFalse,
		string(gatewayv1.GatewayReasonInvalidClientCertificateRef),
		1,
	)
}

func readBackendTLSAsset(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("..", "..", "..", "tests", "testdata", "backendtls", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return raw
}
