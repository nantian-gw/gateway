package admin

import (
	"net/url"
	"sort"
	"strings"

	"github.com/nantian-gw/gateway/controlplane/internal/infrastructure"
)

type infrastructureSortField string

const (
	infrastructureSortByState     infrastructureSortField = "state"
	infrastructureSortByRole      infrastructureSortField = "role"
	infrastructureSortByKind      infrastructureSortField = "kind"
	infrastructureSortByNamespace infrastructureSortField = "namespace"
	infrastructureSortByName      infrastructureSortField = "name"
)

type infrastructureQueryFilter struct {
	State      string
	Role       string
	Kind       string
	Namespace  string
	Name       string
	Sort       infrastructureSortField
	Order      sortOrder
	Pagination listPagination
}

func parseInfrastructureQueryFilter(query url.Values) (infrastructureQueryFilter, error) {
	state, err := parseInfrastructureStateFilter(query.Get("state"))
	if err != nil {
		return infrastructureQueryFilter{}, err
	}
	role, err := parseInfrastructureRoleFilter(query.Get("role"))
	if err != nil {
		return infrastructureQueryFilter{}, err
	}
	kind, err := parseInfrastructureKindFilter(query.Get("kind"))
	if err != nil {
		return infrastructureQueryFilter{}, err
	}
	sortField, err := parseInfrastructureSortField(query.Get("sort"))
	if err != nil {
		return infrastructureQueryFilter{}, err
	}
	order, err := parseSortOrder(query.Get("order"))
	if err != nil {
		return infrastructureQueryFilter{}, err
	}
	pagination, err := parseListPagination(query)
	if err != nil {
		return infrastructureQueryFilter{}, err
	}

	return infrastructureQueryFilter{
		State:      state,
		Role:       role,
		Kind:       kind,
		Namespace:  strings.TrimSpace(query.Get("namespace")),
		Name:       strings.TrimSpace(query.Get("name")),
		Sort:       sortField,
		Order:      order,
		Pagination: pagination,
	}, nil
}

func filterInfrastructureReport(
	report infrastructure.InfrastructureReport,
	filter infrastructureQueryFilter,
) infrastructure.InfrastructureReport {
	filtered := make([]infrastructure.InfrastructureResource, 0, len(report.Resources))
	for _, item := range report.Resources {
		if filter.State != "" && item.State != filter.State {
			continue
		}
		if filter.Role != "" && item.Role != filter.Role {
			continue
		}
		if filter.Kind != "" && item.Kind != filter.Kind {
			continue
		}
		if filter.Namespace != "" && item.Namespace != filter.Namespace {
			continue
		}
		if filter.Name != "" && item.Name != filter.Name {
			continue
		}
		filtered = append(filtered, item)
	}

	sortInfrastructureResources(filtered, filter.Sort, filter.Order)

	report.Resources = paginateSlice(filtered, filter.Pagination)
	return report
}

func parseInfrastructureStateFilter(raw string) (string, error) {
	switch normalizeSortField(raw) {
	case "":
		return "", nil
	case "ready":
		return infrastructure.InfrastructureStateReady, nil
	case "drifted":
		return infrastructure.InfrastructureStateDrifted, nil
	case "missing":
		return infrastructure.InfrastructureStateMissing, nil
	case "orphan":
		return infrastructure.InfrastructureStateOrphan, nil
	default:
		return "", errInvalidQuery("invalid infrastructure state")
	}
}

func parseInfrastructureRoleFilter(raw string) (string, error) {
	switch normalizeSortField(raw) {
	case "":
		return "", nil
	case "sharedservice":
		return infrastructure.InfrastructureRoleSharedSvc, nil
	case "gatewayservice":
		return infrastructure.InfrastructureRoleGatewaySvc, nil
	case "meshfrontendservice":
		return infrastructure.InfrastructureRoleMeshFrontendSvc, nil
	case "meshshadowservice":
		return infrastructure.InfrastructureRoleMeshShadowSvc, nil
	case "sharedendpointslice":
		return infrastructure.InfrastructureRoleSharedSlice, nil
	case "gatewayendpointslice":
		return infrastructure.InfrastructureRoleGatewaySlice, nil
	case "meshendpointslice":
		return infrastructure.InfrastructureRoleMeshSlice, nil
	default:
		return "", errInvalidQuery("invalid infrastructure role")
	}
}

func parseInfrastructureKindFilter(raw string) (string, error) {
	switch normalizeSortField(raw) {
	case "":
		return "", nil
	case "service":
		return infrastructure.InfrastructureKindService, nil
	case "endpointslice":
		return infrastructure.InfrastructureKindSlice, nil
	default:
		return "", errInvalidQuery("invalid infrastructure kind")
	}
}

func parseInfrastructureSortField(raw string) (infrastructureSortField, error) {
	switch normalizeSortField(raw) {
	case "", "state":
		return infrastructureSortByState, nil
	case "role":
		return infrastructureSortByRole, nil
	case "kind":
		return infrastructureSortByKind, nil
	case "namespace":
		return infrastructureSortByNamespace, nil
	case "name":
		return infrastructureSortByName, nil
	default:
		return "", errInvalidQuery("invalid sort")
	}
}

func sortInfrastructureResources(
	items []infrastructure.InfrastructureResource,
	field infrastructureSortField,
	order sortOrder,
) {
	sort.Slice(items, func(i, j int) bool {
		return orderedLess(order, compareInfrastructureResources(field, items[i], items[j])...)
	})
}

func compareInfrastructureResources(
	field infrastructureSortField,
	left infrastructure.InfrastructureResource,
	right infrastructure.InfrastructureResource,
) []int {
	switch field {
	case infrastructureSortByRole:
		return []int{
			strings.Compare(left.Role, right.Role),
			compareInt(infrastructureStateRank(left.State), infrastructureStateRank(right.State)),
			strings.Compare(left.Kind, right.Kind),
			strings.Compare(left.Namespace, right.Namespace),
			strings.Compare(left.Name, right.Name),
		}
	case infrastructureSortByKind:
		return []int{
			strings.Compare(left.Kind, right.Kind),
			compareInt(infrastructureStateRank(left.State), infrastructureStateRank(right.State)),
			strings.Compare(left.Role, right.Role),
			strings.Compare(left.Namespace, right.Namespace),
			strings.Compare(left.Name, right.Name),
		}
	case infrastructureSortByNamespace:
		return []int{
			strings.Compare(left.Namespace, right.Namespace),
			strings.Compare(left.Name, right.Name),
			compareInt(infrastructureStateRank(left.State), infrastructureStateRank(right.State)),
			strings.Compare(left.Role, right.Role),
			strings.Compare(left.Kind, right.Kind),
		}
	case infrastructureSortByName:
		return []int{
			strings.Compare(left.Name, right.Name),
			strings.Compare(left.Namespace, right.Namespace),
			compareInt(infrastructureStateRank(left.State), infrastructureStateRank(right.State)),
			strings.Compare(left.Role, right.Role),
			strings.Compare(left.Kind, right.Kind),
		}
	default:
		return []int{
			compareInt(infrastructureStateRank(left.State), infrastructureStateRank(right.State)),
			strings.Compare(left.Role, right.Role),
			strings.Compare(left.Kind, right.Kind),
			strings.Compare(left.Namespace, right.Namespace),
			strings.Compare(left.Name, right.Name),
		}
	}
}

func infrastructureStateRank(state string) int {
	switch state {
	case infrastructure.InfrastructureStateMissing:
		return 0
	case infrastructure.InfrastructureStateDrifted:
		return 1
	case infrastructure.InfrastructureStateOrphan:
		return 2
	default:
		return 3
	}
}

func compareInt(left, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}
