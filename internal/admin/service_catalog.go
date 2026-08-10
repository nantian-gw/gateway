package admin

import (
	"context"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ServiceCatalogEntry struct {
	Namespace string               `json:"namespace"`
	Name      string               `json:"name"`
	Ports     []ServiceCatalogPort `json:"ports,omitempty"`
}

type ServiceCatalogPort struct {
	Name       string `json:"name,omitempty"`
	Port       int32  `json:"port"`
	Protocol   string `json:"protocol,omitempty"`
	TargetPort string `json:"targetPort,omitempty"`
}

type ServiceCatalogFilter struct {
	Namespace string
	Name      string
	Protocol  string
	Port      int
	HasPort   bool
	Sort      serviceCatalogSortField
	Order     sortOrder
	Offset    int
	Limit     int
	HasLimit  bool
}

type serviceCatalogSortField string

const (
	serviceCatalogSortByNamespace serviceCatalogSortField = "namespace"
	serviceCatalogSortByName      serviceCatalogSortField = "name"
)

func parseServiceCatalogFilter(query url.Values) (ServiceCatalogFilter, error) {
	filter := ServiceCatalogFilter{
		Namespace: strings.TrimSpace(query.Get("namespace")),
		Name:      strings.TrimSpace(query.Get("name")),
	}

	protocol, err := parseServiceCatalogProtocolFilter(query.Get("protocol"))
	if err != nil {
		return ServiceCatalogFilter{}, err
	}
	filter.Protocol = protocol

	port, err := parseOptionalPositiveInt(query.Get("port"), "port")
	if err != nil {
		return ServiceCatalogFilter{}, err
	}
	if port != nil {
		if *port > 65535 {
			return ServiceCatalogFilter{}, errInvalidQuery("port must be less than or equal to 65535")
		}
		filter.Port = *port
		filter.HasPort = true
	}

	sortField, err := parseServiceCatalogSortField(query.Get("sort"))
	if err != nil {
		return ServiceCatalogFilter{}, err
	}
	filter.Sort = sortField

	order, err := parseSortOrder(query.Get("order"))
	if err != nil {
		return ServiceCatalogFilter{}, err
	}
	filter.Order = order

	limit, err := parseOptionalPositiveInt(query.Get("limit"), "limit")
	if err != nil {
		return ServiceCatalogFilter{}, err
	}
	if limit != nil {
		filter.Limit = *limit
		filter.HasLimit = true
	}

	offset, err := parseOptionalNonNegativeInt(query.Get("offset"), "offset")
	if err != nil {
		return ServiceCatalogFilter{}, err
	}
	if offset != nil {
		filter.Offset = *offset
	}

	return filter, nil
}

func (m *ResourceManager) ListServiceCatalog(
	ctx context.Context,
	filter ServiceCatalogFilter,
	maxListItems int,
) ([]ServiceCatalogEntry, pageMetadata, error) {
	if exact := strings.TrimSpace(filter.Namespace); exact != "" && strings.TrimSpace(filter.Name) != "" {
		items, err := m.getExactServiceCatalogEntry(ctx, filter)
		if err != nil {
			return nil, pageMetadata{}, err
		}
		paged, meta := paginateServiceCatalogEntries(items, filter, maxListItems)
		return paged, meta, nil
	}

	cacheKey := serviceCatalogCacheKey(filter)
	if items, ok := m.listCache.getServiceCatalogEntries(cacheKey); ok {
		paged, meta := paginateServiceCatalogEntries(items, filter, maxListItems)
		return paged, meta, nil
	}

	var services corev1.ServiceList
	listOptions := make([]client.ListOption, 0, 1)
	if filter.Namespace != "" {
		listOptions = append(listOptions, client.InNamespace(filter.Namespace))
	}
	if err := m.client.List(ctx, &services, listOptions...); err != nil {
		return nil, pageMetadata{}, err
	}

	items := make([]ServiceCatalogEntry, 0, len(services.Items))
	for _, service := range services.Items {
		entry, ok := buildServiceCatalogEntry(service, filter)
		if !ok {
			continue
		}
		items = append(items, entry)
	}

	sortServiceCatalogEntries(items, filter.Sort, filter.Order)
	m.listCache.putServiceCatalogEntries(cacheKey, items)
	paged, meta := paginateServiceCatalogEntries(items, filter, maxListItems)
	return paged, meta, nil
}

func (m *ResourceManager) getExactServiceCatalogEntry(ctx context.Context, filter ServiceCatalogFilter) ([]ServiceCatalogEntry, error) {
	var service corev1.Service
	if err := m.client.Get(ctx, client.ObjectKey{Namespace: filter.Namespace, Name: filter.Name}, &service); err != nil {
		if apierrors.IsNotFound(err) {
			return []ServiceCatalogEntry{}, nil
		}
		return nil, err
	}

	entry, ok := buildServiceCatalogEntry(service, filter)
	if !ok {
		return []ServiceCatalogEntry{}, nil
	}
	return []ServiceCatalogEntry{entry}, nil
}

func serviceCatalogPagination(filter ServiceCatalogFilter) listPagination {
	return listPagination{
		offset:   filter.Offset,
		limit:    filter.Limit,
		hasLimit: filter.HasLimit,
	}
}

func paginateServiceCatalogEntries(
	items []ServiceCatalogEntry,
	filter ServiceCatalogFilter,
	maxListItems int,
) ([]ServiceCatalogEntry, pageMetadata) {
	paged, meta := paginateSliceWithMetadata(items, serviceCatalogPagination(filter), maxListItems)
	return slices.Clone(paged), meta
}

func buildServiceCatalogEntry(service corev1.Service, filter ServiceCatalogFilter) (ServiceCatalogEntry, bool) {
	if filter.Namespace != "" && service.Namespace != filter.Namespace {
		return ServiceCatalogEntry{}, false
	}
	if filter.Name != "" && service.Name != filter.Name {
		return ServiceCatalogEntry{}, false
	}

	entry := ServiceCatalogEntry{
		Namespace: service.Namespace,
		Name:      service.Name,
		Ports:     make([]ServiceCatalogPort, 0, len(service.Spec.Ports)),
	}
	for _, port := range service.Spec.Ports {
		if filter.Protocol != "" && string(port.Protocol) != filter.Protocol {
			continue
		}
		if filter.HasPort && int(port.Port) != filter.Port {
			continue
		}
		entry.Ports = append(entry.Ports, ServiceCatalogPort{
			Name:       port.Name,
			Port:       port.Port,
			Protocol:   string(port.Protocol),
			TargetPort: targetPortString(port.TargetPort),
		})
	}

	if len(entry.Ports) == 0 {
		return ServiceCatalogEntry{}, false
	}

	sort.Slice(entry.Ports, func(i, j int) bool {
		if entry.Ports[i].Port != entry.Ports[j].Port {
			return entry.Ports[i].Port < entry.Ports[j].Port
		}
		return entry.Ports[i].Name < entry.Ports[j].Name
	})
	return entry, true
}

func sortServiceCatalogEntries(items []ServiceCatalogEntry, field serviceCatalogSortField, order sortOrder) {
	sort.Slice(items, func(i, j int) bool {
		left := items[i]
		right := items[j]

		switch field {
		case serviceCatalogSortByName:
			return orderedLess(
				order,
				strings.Compare(left.Name, right.Name),
				strings.Compare(left.Namespace, right.Namespace),
			)
		default:
			return orderedLess(
				order,
				strings.Compare(left.Namespace, right.Namespace),
				strings.Compare(left.Name, right.Name),
			)
		}
	})
}

func parseServiceCatalogProtocolFilter(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	switch strings.ToUpper(raw) {
	case string(corev1.ProtocolTCP):
		return string(corev1.ProtocolTCP), nil
	case string(corev1.ProtocolUDP):
		return string(corev1.ProtocolUDP), nil
	case string(corev1.ProtocolSCTP):
		return string(corev1.ProtocolSCTP), nil
	default:
		return "", errInvalidQuery("invalid service protocol")
	}
}

func parseServiceCatalogSortField(raw string) (serviceCatalogSortField, error) {
	switch normalizeSortField(raw) {
	case "", "namespace":
		return serviceCatalogSortByNamespace, nil
	case "name":
		return serviceCatalogSortByName, nil
	default:
		return "", errInvalidQuery("invalid sort")
	}
}

func targetPortString(value intstr.IntOrString) string {
	switch value.Type {
	case intstr.Int:
		if value.IntValue() == 0 {
			return ""
		}
		return strconv.Itoa(value.IntValue())
	case intstr.String:
		return value.StrVal
	default:
		return ""
	}
}
