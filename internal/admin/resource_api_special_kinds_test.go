package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestResourceEndpointsSupportBackendPolicyKinds(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var kinds []ResourceKindDescriptor
	recorder := performRequest(t, server, http.MethodGet, "/v1/resource-kinds", &kinds)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	foundLB := false
	foundTLS := false
	for _, kind := range kinds {
		if kind.Kind == "BackendLBPolicy" {
			foundLB = true
			if !kind.Available {
				t.Fatalf("expected BackendLBPolicy kind to be available, got %+v", kind)
			}
		}
		if kind.Kind == "BackendTLSPolicy" {
			foundTLS = true
			if !kind.Available {
				t.Fatalf("expected BackendTLSPolicy kind to be available, got %+v", kind)
			}
		}
	}
	if !foundLB || !foundTLS {
		t.Fatalf("expected backend policy kinds, got %+v", kinds)
	}

	backendLBPolicy := []byte(`
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: BackendLBPolicy
metadata:
  name: orders-sticky
  namespace: default
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: orders
  sessionPersistence:
    sessionName: orders-session
    type: Header
`)
	recorder = performRequestWithBody(t, server, http.MethodPost, "/v1/resources", backendLBPolicy)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backend lb create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var created ManagedResource
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created backend lb policy: %v", err)
	}
	if created.Kind != "BackendLBPolicy" || created.Name != "orders-sticky" {
		t.Fatalf("unexpected backend lb policy: %+v", created)
	}

	var resources []ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources?kind=BackendLBPolicy", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backend lb list 200, got %d", recorder.Code)
	}
	if len(resources) != 1 || resources[0].Kind != "BackendLBPolicy" {
		t.Fatalf("unexpected backend lb policies: %+v", resources)
	}

	backendTLSPolicy := []byte(`
apiVersion: gateway.networking.k8s.io/v1
kind: BackendTLSPolicy
metadata:
  name: orders-tls
  namespace: default
spec:
  targetRefs:
    - group: ""
      kind: Service
      name: orders
      sectionName: https
  validation:
    hostname: orders.internal.example
    wellKnownCACertificates: System
`)
	recorder = performRequestWithBody(t, server, http.MethodPost, "/v1/resources", backendTLSPolicy)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backend tls create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var detail ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources/backendtlspolicy/default/orders-tls", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected backend tls detail 200, got %d", recorder.Code)
	}
	if detail.Kind != "BackendTLSPolicy" || detail.Name != "orders-tls" {
		t.Fatalf("unexpected backend tls detail: %+v", detail)
	}
}

func TestResourceEndpointsSupportReferenceGrant(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var kinds []ResourceKindDescriptor
	recorder := performRequest(t, server, http.MethodGet, "/v1/resource-kinds", &kinds)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	found := false
	for _, kind := range kinds {
		if kind.Kind == "ReferenceGrant" {
			found = true
			if !kind.Available {
				t.Fatalf("expected ReferenceGrant kind to be available, got %+v", kind)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected ReferenceGrant kind, got %+v", kinds)
	}

	createBody := []byte(`
apiVersion: gateway.networking.k8s.io/v1beta1
kind: ReferenceGrant
metadata:
  name: allow-default-to-backend
  namespace: backend
spec:
  from:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
      namespace: default
  to:
    - group: ""
      kind: Service
`)
	recorder = performRequestWithBody(t, server, http.MethodPost, "/v1/resources", createBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected reference grant create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var created ManagedResource
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created reference grant: %v", err)
	}
	if created.Kind != "ReferenceGrant" || created.Namespace != "backend" || created.Name != "allow-default-to-backend" {
		t.Fatalf("unexpected reference grant: %+v", created)
	}

	var resources []ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources?kind=ReferenceGrant", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected reference grant list 200, got %d", recorder.Code)
	}
	if len(resources) != 1 || resources[0].Kind != "ReferenceGrant" {
		t.Fatalf("unexpected reference grants: %+v", resources)
	}

	var detail ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources/referencegrant/backend/allow-default-to-backend", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected reference grant detail 200, got %d", recorder.Code)
	}
	if detail.Kind != "ReferenceGrant" || detail.Namespace != "backend" || detail.Name != "allow-default-to-backend" {
		t.Fatalf("unexpected reference grant detail: %+v", detail)
	}
}

func TestResourceEndpointsSupportGatewayClass(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var kinds []ResourceKindDescriptor
	recorder := performRequest(t, server, http.MethodGet, "/v1/resource-kinds", &kinds)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	found := false
	for _, kind := range kinds {
		if kind.Kind == "GatewayClass" {
			found = true
			if kind.Namespaced {
				t.Fatalf("expected GatewayClass to be cluster-scoped, got %+v", kind)
			}
			if !kind.Available {
				t.Fatalf("expected GatewayClass kind to be available, got %+v", kind)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected GatewayClass kind, got %+v", kinds)
	}

	createBody := []byte(`
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: edge
spec:
  controllerName: gateway.nantian.dev/controller
`)
	recorder = performRequestWithBody(t, server, http.MethodPost, "/v1/resources", createBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected gatewayclass create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var created ManagedResource
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created gatewayclass: %v", err)
	}
	if created.Kind != "GatewayClass" || created.Namespace != "" || created.Name != "edge" {
		t.Fatalf("unexpected gatewayclass: %+v", created)
	}

	var resources []ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources?kind=GatewayClass&name=edge", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected gatewayclass list 200, got %d", recorder.Code)
	}
	if len(resources) != 1 || resources[0].Kind != "GatewayClass" || resources[0].Namespace != "" {
		t.Fatalf("unexpected gatewayclasses: %+v", resources)
	}

	var detail ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources/gatewayclass/_cluster/edge", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected gatewayclass detail 200, got %d", recorder.Code)
	}
	if detail.Kind != "GatewayClass" || detail.Namespace != "" || detail.Name != "edge" {
		t.Fatalf("unexpected gatewayclass detail: %+v", detail)
	}

	updateBody := []byte(`{
  "apiVersion": "gateway.networking.k8s.io/v1",
  "kind": "GatewayClass",
  "metadata": {
    "name": "edge",
    "labels": {
      "tier": "shared"
    }
  },
  "spec": {
    "controllerName": "gateway.nantian.dev/controller"
  }
}`)
	recorder = performRequestWithBody(t, server, http.MethodPut, "/v1/resources/gatewayclass/_cluster/edge", updateBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected gatewayclass update 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/resources/gatewayclass/_cluster/edge", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected gatewayclass detail after update 200, got %d", recorder.Code)
	}
	if detail.Labels["tier"] != "shared" {
		t.Fatalf("expected updated gatewayclass labels, got %+v", detail.Labels)
	}

	recorder = performRequest(t, server, http.MethodDelete, "/v1/resources/gatewayclass/_cluster/edge", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected gatewayclass delete 200, got %d", recorder.Code)
	}
}

func TestResourceEndpointsSupportServiceImport(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var kinds []ResourceKindDescriptor
	recorder := performRequest(t, server, http.MethodGet, "/v1/resource-kinds", &kinds)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}

	found := false
	for _, kind := range kinds {
		if kind.Kind == "ServiceImport" {
			found = true
			if !kind.Namespaced || kind.Category != "backend" {
				t.Fatalf("unexpected service import descriptor: %+v", kind)
			}
			if !kind.Available {
				t.Fatalf("expected ServiceImport kind to be available, got %+v", kind)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected ServiceImport kind, got %+v", kinds)
	}

	createBody := []byte(`
apiVersion: multicluster.x-k8s.io/v1alpha1
kind: ServiceImport
metadata:
  name: payments
  namespace: default
spec:
  type: ClusterSetIP
  ips:
    - 10.200.0.15
  ports:
    - name: https
      protocol: TCP
      port: 8443
`)
	recorder = performRequestWithBody(t, server, http.MethodPost, "/v1/resources", createBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected serviceimport create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var created ManagedResource
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created serviceimport: %v", err)
	}
	if created.Kind != "ServiceImport" || created.Namespace != "default" || created.Name != "payments" {
		t.Fatalf("unexpected serviceimport: %+v", created)
	}

	var resources []ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources?kind=ServiceImport&name=payments", &resources)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected serviceimport list 200, got %d", recorder.Code)
	}
	if len(resources) != 1 || resources[0].Kind != "ServiceImport" {
		t.Fatalf("unexpected serviceimports: %+v", resources)
	}

	var detail ManagedResource
	recorder = performRequest(t, server, http.MethodGet, "/v1/resources/serviceimport/default/payments", &detail)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected serviceimport detail 200, got %d", recorder.Code)
	}
	if detail.Kind != "ServiceImport" || detail.Namespace != "default" || detail.Name != "payments" {
		t.Fatalf("unexpected serviceimport detail: %+v", detail)
	}
}
