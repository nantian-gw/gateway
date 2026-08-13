package admin

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/nantian-gw/gateway/internal/ir"
	"github.com/nantian-gw/gateway/internal/constants"
)

func parseOptionalBool(raw string) (*bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, errInvalidQuery("invalid boolean filter")
	}

	return &value, nil
}

func parseOptionalPositiveInt(raw, field string) (*int, error) {
	value, err := parseOptionalInt(raw, field)
	if err != nil || value == nil {
		return value, err
	}
	if *value <= 0 {
		return nil, errInvalidQuery(field + " must be greater than 0")
	}
	return value, nil
}

func parseOptionalNonNegativeInt(raw, field string) (*int, error) {
	value, err := parseOptionalInt(raw, field)
	if err != nil || value == nil {
		return value, err
	}
	if *value < 0 {
		return nil, errInvalidQuery(field + " must be greater than or equal to 0")
	}
	return value, nil
}

func parseOptionalInt(raw, field string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return nil, errInvalidQuery("invalid " + field)
	}
	return &value, nil
}

func parseIncludeAllBackends(raw string) (bool, error) {
	value, err := parseOptionalBool(raw)
	if err != nil {
		return false, err
	}
	if value == nil {
		return false, nil
	}
	return *value, nil
}

type sortOrder int

const (
	sortOrderAscending sortOrder = iota
	sortOrderDescending
)

type listenerSortField string

const (
	listenerSortByName     listenerSortField = constants.StrName
	listenerSortByProtocol listenerSortField = "protocol"
)

type backendSortField string

const (
	backendSortByNamespace backendSortField = "namespace"
	backendSortByName      backendSortField = constants.StrName
	backendSortByProtocol  backendSortField = "protocol"
)

type nodeSortField string

const (
	nodeSortByNodeID  nodeSortField = "nodeId"
	nodeSortByCluster nodeSortField = "cluster"
	nodeSortByVersion nodeSortField = "version"
)

type routeSortField string

const (
	routeSortByNamespace routeSortField = "namespace"
	routeSortByName      routeSortField = constants.StrName
)

type listPagination struct {
	offset   int
	limit    int
	hasLimit bool
}

type pageMetadata struct {
	TotalCount int
	Offset     int
	Limit      int
	HasLimit   bool
	HasNext    bool
}

func parseListPagination(query url.Values) (listPagination, error) {
	limit, err := parseOptionalPositiveInt(query.Get("limit"), "limit")
	if err != nil {
		return listPagination{}, err
	}
	offset, err := parseOptionalNonNegativeInt(query.Get("offset"), "offset")
	if err != nil {
		return listPagination{}, err
	}

	pagination := listPagination{}
	if limit != nil {
		pagination.limit = *limit
		pagination.hasLimit = true
	}
	if offset != nil {
		pagination.offset = *offset
	}

	return pagination, nil
}

func parseRoutePagination(query url.Values, kind string) (listPagination, error) {
	pagination, err := parseListPagination(query)
	if err != nil {
		return listPagination{}, err
	}
	if kind == "" && (query.Get("limit") != "" || query.Get("offset") != "") {
		return listPagination{}, errInvalidQuery("route pagination requires kind")
	}
	return pagination, nil
}

func parseSortOrder(raw string) (sortOrder, error) {
	switch normalizeSortField(raw) {
	case "", "asc":
		return sortOrderAscending, nil
	case "desc":
		return sortOrderDescending, nil
	default:
		return sortOrderAscending, errInvalidQuery("invalid order")
	}
}

func parseListenerSortField(raw string) (listenerSortField, error) {
	switch normalizeSortField(raw) {
	case "", constants.StrName:
		return listenerSortByName, nil
	case "protocol":
		return listenerSortByProtocol, nil
	default:
		return "", errInvalidQuery("invalid sort")
	}
}

func parseBackendSortField(raw string) (backendSortField, error) {
	switch normalizeSortField(raw) {
	case "", "namespace":
		return backendSortByNamespace, nil
	case constants.StrName:
		return backendSortByName, nil
	case "protocol":
		return backendSortByProtocol, nil
	default:
		return "", errInvalidQuery("invalid sort")
	}
}

func parseNodeSortField(raw string) (nodeSortField, error) {
	switch normalizeSortField(raw) {
	case "", "nodeid":
		return nodeSortByNodeID, nil
	case "cluster":
		return nodeSortByCluster, nil
	case "version":
		return nodeSortByVersion, nil
	default:
		return "", errInvalidQuery("invalid sort")
	}
}

func parseRouteSortField(raw string) (routeSortField, error) {
	switch normalizeSortField(raw) {
	case "", "namespace":
		return routeSortByNamespace, nil
	case constants.StrName:
		return routeSortByName, nil
	default:
		return "", errInvalidQuery("invalid sort")
	}
}

func sortBy[T any](slice []T, order sortOrder, cmp func(*T, *T) []int) {
	sort.Slice(slice, func(i, j int) bool {
		return orderedLess(order, cmp(&slice[i], &slice[j])...)
	})
}

func sortListeners(listeners []ir.Listener, field listenerSortField, order sortOrder) {
	sortBy(listeners, order, func(left, right *ir.Listener) []int {
		switch field {
		case listenerSortByProtocol:
			return []int{
				strings.Compare(listenerProtocolValue(*left), listenerProtocolValue(*right)),
				strings.Compare(left.Name, right.Name),
			}
		default:
			return []int{
				strings.Compare(left.Name, right.Name),
				strings.Compare(listenerProtocolValue(*left), listenerProtocolValue(*right)),
			}
		}
	})
}

func sortBackends(backends []ir.BackendCluster, field backendSortField, order sortOrder) {
	sortBy(backends, order, func(left, right *ir.BackendCluster) []int {
		switch field {
		case backendSortByName:
			return []int{
				strings.Compare(left.Name, right.Name),
				strings.Compare(left.Namespace, right.Namespace),
				strings.Compare(backendProtocolValue(*left), backendProtocolValue(*right)),
			}
		case backendSortByProtocol:
			return []int{
				strings.Compare(backendProtocolValue(*left), backendProtocolValue(*right)),
				strings.Compare(left.Namespace, right.Namespace),
				strings.Compare(left.Name, right.Name),
			}
		default:
			return []int{
				strings.Compare(left.Namespace, right.Namespace),
				strings.Compare(left.Name, right.Name),
				strings.Compare(backendProtocolValue(*left), backendProtocolValue(*right)),
			}
		}
	})
}

func sortNodes(nodes []ir.NodeStatus, field nodeSortField, order sortOrder) {
	sortBy(nodes, order, func(left, right *ir.NodeStatus) []int {
		switch field {
		case nodeSortByCluster:
			return []int{
				strings.Compare(left.Cluster, right.Cluster),
				strings.Compare(left.NodeID, right.NodeID),
				strings.Compare(nodeVersionValue(*left), nodeVersionValue(*right)),
			}
		case nodeSortByVersion:
			return []int{
				strings.Compare(nodeVersionValue(*left), nodeVersionValue(*right)),
				strings.Compare(left.NodeID, right.NodeID),
				strings.Compare(left.Cluster, right.Cluster),
			}
		default:
			return []int{
				strings.Compare(left.NodeID, right.NodeID),
				strings.Compare(left.Cluster, right.Cluster),
				strings.Compare(nodeVersionValue(*left), nodeVersionValue(*right)),
			}
		}
	})
}

func sortHTTPRoutes(routes []ir.HTTPRoute, field routeSortField, order sortOrder) {
	sortBy(routes, order, func(left, right *ir.HTTPRoute) []int {
		return compareRouteIdentity(field, left.Namespace, left.Name, right.Namespace, right.Name)
	})
}

func sortGRPCRoutes(routes []ir.GRPCRoute, field routeSortField, order sortOrder) {
	sortBy(routes, order, func(left, right *ir.GRPCRoute) []int {
		return compareRouteIdentity(field, left.Namespace, left.Name, right.Namespace, right.Name)
	})
}

func sortStreamRoutes(routes []ir.StreamRoute, field routeSortField, order sortOrder) {
	sortBy(routes, order, func(left, right *ir.StreamRoute) []int {
		return compareRouteIdentity(field, left.Namespace, left.Name, right.Namespace, right.Name)
	})
}

func compareRouteIdentity(field routeSortField, leftNamespace, leftName, rightNamespace, rightName string) []int {
	switch field {
	case routeSortByName:
		return []int{
			strings.Compare(leftName, rightName),
			strings.Compare(leftNamespace, rightNamespace),
		}
	default:
		return []int{
			strings.Compare(leftNamespace, rightNamespace),
			strings.Compare(leftName, rightName),
		}
	}
}

func orderedLess(order sortOrder, comparisons ...int) bool {
	for _, comparison := range comparisons {
		if comparison == 0 {
			continue
		}
		if order == sortOrderDescending {
			return comparison > 0
		}
		return comparison < 0
	}
	return false
}

func paginateSlice[T any](items []T, pagination listPagination) []T {
	if pagination.offset >= len(items) {
		return []T{}
	}

	start := pagination.offset
	end := len(items)
	if pagination.hasLimit && start+pagination.limit < end {
		end = start + pagination.limit
	}
	return items[start:end]
}

func clampPagination(pagination listPagination, maxListItems int) listPagination {
	if !pagination.hasLimit || maxListItems <= 0 || pagination.limit <= maxListItems {
		return pagination
	}
	pagination.limit = maxListItems
	return pagination
}

func paginateSliceWithMetadata[T any](items []T, pagination listPagination, maxListItems int) ([]T, pageMetadata) {
	pagination = clampPagination(pagination, maxListItems)

	meta := pageMetadata{
		TotalCount: len(items),
		Offset:     pagination.offset,
		Limit:      pagination.limit,
		HasLimit:   pagination.hasLimit,
	}

	paged := paginateSlice(items, pagination)
	meta.HasNext = meta.Offset+len(paged) < meta.TotalCount

	return paged, meta
}

func writePaginationHeaders(header http.Header, meta pageMetadata) {
	header.Set("X-Nantian-Total-Count", strconv.Itoa(meta.TotalCount))

	if meta.HasLimit {
		header.Set("X-Nantian-Page-Limit", strconv.Itoa(meta.Limit))
	}
	if meta.HasLimit || meta.Offset > 0 {
		header.Set("X-Nantian-Page-Offset", strconv.Itoa(meta.Offset))
		header.Set("X-Nantian-Has-Next-Page", strconv.FormatBool(meta.HasNext))
	}
}

func listenerProtocolValue(listener ir.Listener) string {
	if value := canonicalProtocol(listener.Protocol); value != "" {
		return value
	}
	return strings.TrimSpace(listener.Protocol)
}

func backendProtocolValue(backend ir.BackendCluster) string {
	return strings.TrimSpace(canonicalBackendProtocol(backend.Protocol))
}

func nodeVersionValue(node ir.NodeStatus) string {
	if version := strings.TrimSpace(node.LastAckVersion); version != "" {
		return version
	}
	if version := strings.TrimSpace(node.LastSentVersion); version != "" {
		return version
	}
	return strings.TrimSpace(node.LastNackVersion)
}

func parseProtocolFilter(raw string) (string, error) {
	return parseCanonicalToken(raw, canonicalProtocol, "protocol")
}

func parseBackendProtocolFilter(raw string) (string, error) {
	return parseCanonicalToken(raw, canonicalBackendProtocol, "backend protocol")
}

func parseRouteKindFilter(raw string) (string, error) {
	return parseCanonicalToken(raw, canonicalRouteKind, "route kind")
}

func parseRequiredRouteKind(raw string) (string, error) {
	kind, err := parseRouteKindFilter(raw)
	if err != nil {
		return "", err
	}
	if kind == "" {
		return "", errInvalidQuery("route kind is required")
	}
	return kind, nil
}

func parseCanonicalToken(raw string, normalizer func(string) string, field string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}

	value := normalizer(raw)
	if value == "" {
		return "", errInvalidQuery("invalid " + field)
	}

	return value, nil
}

func canonicalProtocol(protocol string) string {
	switch normalizeToken(protocol) {
	case kindHTTPRoute:
		return kindHTTPRoute
	case "HTTPS":
		return "HTTPS"
	case kindGRPCRoute:
		return kindGRPCRoute
	case "HTTP3":
		return "HTTP3"
	case kindTCPRoute:
		return kindTCPRoute
	case kindUDPRoute:
		return kindUDPRoute
	case "TLSPASSTHROUGH", kindTLSRoute:
		return kindTLSRoute
	default:
		return ""
	}
}

func canonicalBackendProtocol(protocol string) string {
	switch normalizeToken(protocol) {
	case kindTCPRoute:
		return kindTCPRoute
	case kindUDPRoute:
		return kindUDPRoute
	case kindHTTPRoute:
		return kindHTTPRoute
	case "HTTPS":
		return "HTTPS"
	case kindGRPCRoute:
		return kindGRPCRoute
	case "GRPCS":
		return "GRPCS"
	case "H2C":
		return "H2C"
	default:
		return strings.ToUpper(strings.TrimSpace(protocol))
	}
}

func canonicalRouteKind(kind string) string {
	switch normalizeToken(kind) {
	case kindHTTPRoute:
		return kindHTTPRoute
	case kindGRPCRoute:
		return kindGRPCRoute
	case kindTCPRoute:
		return kindTCPRoute
	case kindUDPRoute:
		return kindUDPRoute
	case kindTLSRoute:
		return kindTLSRoute
	default:
		return ""
	}
}

func normalizeToken(value string) string {
	value = strings.TrimSpace(strings.ToUpper(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.TrimPrefix(value, "LISTENER_PROTOCOL_")
	value = strings.TrimPrefix(value, "ROUTE_KIND_")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.TrimSuffix(value, "ROUTE")
	return value
}

func normalizeSortField(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	return value
}

func stringSliceContains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}

	return false
}
