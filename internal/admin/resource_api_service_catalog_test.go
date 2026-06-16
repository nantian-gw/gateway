package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestServiceCatalogEndpointReturnsServicesAndPorts(t *testing.T) {
	t.Parallel()

	server := newTestServerWithResourceManager(t, resourceManagerForTest(t))

	var services []ServiceCatalogEntry
	recorder := performRequest(t, server, http.MethodGet, "/v1/service-catalog", &services)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %+v", services)
	}
	if services[0].Namespace != "default" || services[0].Name != "orders" {
		t.Fatalf("unexpected service catalog entry: %+v", services[0])
	}
	if len(services[0].Ports) != 2 {
		t.Fatalf("expected 2 service ports, got %+v", services[0].Ports)
	}
	if services[0].Ports[0].Port != 8080 || services[0].Ports[0].Name != "http" {
		t.Fatalf("unexpected first service port: %+v", services[0].Ports[0])
	}
	if services[0].Ports[1].Port != 8443 || services[0].Ports[1].TargetPort != "https" {
		t.Fatalf("unexpected second service port: %+v", services[0].Ports[1])
	}
}

func TestServiceCatalogEndpointSupportsFilteringSortingAndPagination(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	createServiceForTest(t, manager, &corev1.Service{
		TypeMeta:   metav1TypeMeta("v1", "Service"),
		ObjectMeta: metav1ObjectMeta("ops", "metrics"),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "prom",
				Port:       9090,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromString("prom"),
			}},
		},
	})
	createServiceForTest(t, manager, &corev1.Service{
		TypeMeta:   metav1TypeMeta("v1", "Service"),
		ObjectMeta: metav1ObjectMeta("default", "echo"),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "udp",
				Port:       53,
				Protocol:   corev1.ProtocolUDP,
				TargetPort: intstr.FromInt(53),
			}},
		},
	})

	server := newTestServerWithResourceManager(t, manager)

	var services []ServiceCatalogEntry
	recorder := performRequest(t, server, http.MethodGet, "/v1/service-catalog?namespace=default&protocol=udp", &services)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(services) != 1 || services[0].Namespace != "default" || services[0].Name != "echo" {
		t.Fatalf("unexpected filtered services: %+v", services)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/service-catalog?sort=name&order=desc&limit=2", &services)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := serviceCatalogKeys(services); strings.Join(got, ",") != "default/orders,ops/metrics" {
		t.Fatalf("unexpected sorted paginated services: %+v", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/service-catalog?offset=1&limit=1", &services)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if got := serviceCatalogKeys(services); strings.Join(got, ",") != "default/orders" {
		t.Fatalf("unexpected paginated services: %+v", got)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/service-catalog?protocol=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid service catalog protocol, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/service-catalog?sort=broken", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid service catalog sort, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/service-catalog?order=sideways", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid service catalog order, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/service-catalog?limit=0", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid service catalog limit, got %d", recorder.Code)
	}

	recorder = performRequest(t, server, http.MethodGet, "/v1/service-catalog?offset=-1", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid service catalog offset, got %d", recorder.Code)
	}
}

func TestServiceCatalogClampLimitAndEmitPaginationHeaders(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	createServiceForTest(t, manager, &corev1.Service{
		TypeMeta:   metav1TypeMeta("v1", "Service"),
		ObjectMeta: metav1ObjectMeta("ops", "metrics"),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "prom",
				Port:       9090,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromString("prom"),
			}},
		},
	})
	createServiceForTest(t, manager, &corev1.Service{
		TypeMeta:   metav1TypeMeta("v1", "Service"),
		ObjectMeta: metav1ObjectMeta("default", "echo"),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "udp",
				Port:       53,
				Protocol:   corev1.ProtocolUDP,
				TargetPort: intstr.FromInt(53),
			}},
		},
	})

	server := newTestServerWithResourceManagerAndOptions(t, manager, Options{MaxListItems: 1})

	var services []ServiceCatalogEntry
	recorder := performRequest(t, server, http.MethodGet, "/v1/service-catalog?sort=name&limit=9&offset=0", &services)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(services) != 1 {
		t.Fatalf("expected clamped service catalog page size of 1, got %+v", services)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Limit"); got != "1" {
		t.Fatalf("unexpected page limit header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Page-Offset"); got != "0" {
		t.Fatalf("unexpected page offset header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Total-Count"); got != "3" {
		t.Fatalf("unexpected total count header: %q", got)
	}
	if got := recorder.Header().Get("X-Nantian-Has-Next-Page"); got != "true" {
		t.Fatalf("unexpected has-next-page header: %q", got)
	}
}

func TestServiceCatalogListUsesDirectGetForExactMatch(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting
	server := newTestServerWithResourceManager(t, manager)

	var items []ServiceCatalogEntry
	recorder := performRequest(t, server, http.MethodGet, "/v1/service-catalog?namespace=default&name=orders", &items)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(items) != 1 || items[0].Namespace != "default" || items[0].Name != "orders" {
		t.Fatalf("unexpected exact service catalog result: %+v", items)
	}
	if got := counting.GetCalls(); got != 1 {
		t.Fatalf("get call count = %d, want 1", got)
	}
	if got := counting.ListCalls(); got != 0 {
		t.Fatalf("list call count = %d, want 0", got)
	}
}

func TestServiceCatalogListCachesRepeatedNamespaceList(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	filter := ServiceCatalogFilter{
		Namespace: "default",
		Sort:      serviceCatalogSortByName,
		Order:     sortOrderDescending,
		Limit:     25,
		HasLimit:  true,
	}

	for i := 0; i < 2; i++ {
		items, _, err := manager.ListServiceCatalog(context.Background(), filter, 0)
		if err != nil {
			t.Fatalf("list service catalog on iteration %d: %v", i, err)
		}
		if len(items) != 1 || items[0].Namespace != "default" || items[0].Name != "orders" {
			t.Fatalf("unexpected cached service catalog list on iteration %d: %+v", i, items)
		}
	}
	if got := counting.ListCalls(); got != 1 {
		t.Fatalf("list call count = %d, want 1", got)
	}
}

func TestServiceCatalogListCacheReusesPagination(t *testing.T) {
	t.Parallel()

	manager := resourceManagerForTest(t)
	manager.listCache.ttl = time.Minute
	createServiceForTest(t, manager, &corev1.Service{
		TypeMeta:   metav1TypeMeta("v1", "Service"),
		ObjectMeta: metav1ObjectMeta("default", "payments"),
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       8081,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(8081),
			}},
		},
	})

	counting := &countingResourceClient{Client: manager.client}
	manager.client = counting

	first := ServiceCatalogFilter{
		Namespace: "default",
		Sort:      serviceCatalogSortByName,
		Order:     sortOrderAscending,
		Limit:     1,
		HasLimit:  true,
	}
	second := first
	second.Offset = 1

	items, meta, err := manager.ListServiceCatalog(context.Background(), first, 0)
	if err != nil {
		t.Fatalf("list first service catalog page: %v", err)
	}
	if len(items) != 1 || items[0].Name != "orders" {
		t.Fatalf("unexpected first service catalog page: %+v", items)
	}
	if meta.TotalCount != 2 || !meta.HasNext {
		t.Fatalf("unexpected first service catalog metadata: %+v", meta)
	}

	items, meta, err = manager.ListServiceCatalog(context.Background(), second, 0)
	if err != nil {
		t.Fatalf("list second service catalog page: %v", err)
	}
	if len(items) != 1 || items[0].Name != "payments" {
		t.Fatalf("unexpected second service catalog page: %+v", items)
	}
	if meta.TotalCount != 2 || meta.HasNext {
		t.Fatalf("unexpected second service catalog metadata: %+v", meta)
	}

	if got := counting.ListCalls(); got != 1 {
		t.Fatalf("service catalog list call count across pagination = %d, want 1", got)
	}
}
