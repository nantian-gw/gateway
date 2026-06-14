package admin

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ManagedResource struct {
	APIVersion        string            `json:"apiVersion"`
	Kind              string            `json:"kind"`
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	UID               string            `json:"uid,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	CreationTimestamp time.Time         `json:"creationTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Resource          map[string]any    `json:"resource"`
}

type ResourceKindDescriptor struct {
	Kind                string `json:"kind"`
	APIVersion          string `json:"apiVersion"`
	Category            string `json:"category"`
	Description         string `json:"description"`
	Namespaced          bool   `json:"namespaced"`
	Available           bool   `json:"available"`
	AvailabilityMessage string `json:"availabilityMessage,omitempty"`
}

type ResourceListFilter struct {
	Kind      string
	Namespace string
	Name      string
	Offset    int
	Limit     int
	HasLimit  bool
}

type ResourceManager struct {
	client    client.Client
	logger    *slog.Logger
	listCache *adminListCache
}

func NewResourceManager(k8sClient client.Client, logger *slog.Logger) *ResourceManager {
	if logger == nil {
		logger = slog.Default()
	}

	return &ResourceManager{
		client:    k8sClient,
		logger:    logger,
		listCache: newAdminListCache(defaultAdminListCacheTTL),
	}
}

func (m *ResourceManager) ListNamespaces(ctx context.Context) ([]string, error) {
	cacheKey := "namespaces"
	if items, ok := m.listCache.getStrings(cacheKey); ok {
		return items, nil
	}

	var nsList corev1.NamespaceList
	if err := m.client.List(ctx, &nsList); err != nil {
		return nil, err
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	sort.Strings(namespaces)

	m.listCache.putStrings(cacheKey, namespaces)
	return namespaces, nil
}

func (m *ResourceManager) DescribeKinds(ctx context.Context) []ResourceKindDescriptor {
	items := make([]ResourceKindDescriptor, 0, len(supportedResourceKinds))
	for _, spec := range supportedResourceKinds {
		descriptor := spec.descriptor
		descriptor.Available = true

		available, message := m.kindAvailable(ctx, spec)
		descriptor.Available = available
		descriptor.AvailabilityMessage = message
		items = append(items, descriptor)
	}

	return items
}

func (m *ResourceManager) List(
	ctx context.Context,
	filter ResourceListFilter,
	maxListItems int,
) ([]ManagedResource, pageMetadata, error) {
	if items, meta, handled, err := m.listExactMatch(ctx, filter, maxListItems); handled {
		return items, meta, err
	}

	kinds := supportedResourceKinds
	canonicalKind := ""
	if strings.TrimSpace(filter.Kind) != "" {
		spec, err := resourceKindSpecFor(filter.Kind)
		if err != nil {
			return nil, pageMetadata{}, err
		}
		kinds = []resourceKindSpec{spec}
		canonicalKind = spec.descriptor.Kind
	}

	cacheKey := resourceListCacheKey(filter, canonicalKind)
	if items, meta, ok := m.listCache.getManagedResources(cacheKey); ok {
		return items, meta, nil
	}

	out := make([]ManagedResource, 0)
	for _, spec := range kinds {
		items, err := m.listByKind(ctx, spec, filter)
		if err != nil {
			return nil, pageMetadata{}, err
		}
		out = append(out, items...)
	}

	paged, meta := paginateManagedResources(out, filter, maxListItems)
	m.listCache.putManagedResources(cacheKey, paged, meta)
	return paged, meta, nil
}

func paginateManagedResources(
	out []ManagedResource,
	filter ResourceListFilter,
	maxListItems int,
) ([]ManagedResource, pageMetadata) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})

	return paginateSliceWithMetadata(out, resourceListPagination(filter), maxListItems)
}

func (m *ResourceManager) Get(ctx context.Context, kind, namespace, name string) (ManagedResource, bool, error) {
	spec, err := resourceKindSpecFor(kind)
	if err != nil {
		return ManagedResource{}, false, err
	}

	return m.getBySpec(ctx, spec, namespace, name)
}

func (m *ResourceManager) Apply(ctx context.Context, raw []byte, expectedKind, expectedNamespace, expectedName string) (ManagedResource, error) {
	spec, obj, err := decodeManagedResource(raw, expectedKind)
	if err != nil {
		return ManagedResource{}, err
	}

	expectedNamespace = normalizedResourceNamespace(spec, expectedNamespace)

	if expectedNamespace != "" {
		if namespace := strings.TrimSpace(obj.GetNamespace()); namespace != "" && namespace != expectedNamespace {
			return ManagedResource{}, errInvalidRequest("resource namespace does not match request path")
		}
		obj.SetNamespace(expectedNamespace)
	} else if !spec.namespaced {
		if namespace := strings.TrimSpace(obj.GetNamespace()); namespace != "" {
			return ManagedResource{}, errInvalidRequest("cluster-scoped resources must not set metadata.namespace")
		}
		obj.SetNamespace("")
	}
	if expectedName != "" {
		if name := strings.TrimSpace(obj.GetName()); name != "" && name != expectedName {
			return ManagedResource{}, errInvalidRequest("resource name does not match request path")
		}
		obj.SetName(expectedName)
	}

	if spec.namespaced && strings.TrimSpace(obj.GetNamespace()) == "" {
		return ManagedResource{}, errInvalidRequest("resource metadata.namespace is required")
	}
	if strings.TrimSpace(obj.GetName()) == "" {
		return ManagedResource{}, errInvalidRequest("resource metadata.name is required")
	}

	key := client.ObjectKeyFromObject(obj)
	current := spec.newObject()
	err = m.client.Get(ctx, key, current)
	switch {
	case apierrors.IsNotFound(err):
		obj.SetResourceVersion("")
		if err := m.client.Create(ctx, obj); err != nil {
			return ManagedResource{}, err
		}
	case err != nil:
		return ManagedResource{}, err
	default:
		obj.SetResourceVersion(current.GetResourceVersion())
		if err := m.client.Update(ctx, obj); err != nil {
			return ManagedResource{}, err
		}
	}

	m.invalidateListCache()
	return managedResourceFromObject(spec, obj)
}

func (m *ResourceManager) Delete(ctx context.Context, kind, namespace, name string) (bool, error) {
	spec, err := resourceKindSpecFor(kind)
	if err != nil {
		return false, err
	}

	obj := spec.newObject()
	obj.SetNamespace(normalizedResourceNamespace(spec, namespace))
	obj.SetName(name)
	err = m.client.Delete(ctx, obj)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	m.invalidateListCache()
	return true, nil
}

func (m *ResourceManager) invalidateListCache() {
	if m == nil {
		return
	}
	m.listCache.clear()
}

func (m *ResourceManager) listByKind(ctx context.Context, spec resourceKindSpec, filter ResourceListFilter) ([]ManagedResource, error) {
	list := spec.newList()
	options := make([]client.ListOption, 0, 1)
	if namespace := strings.TrimSpace(filter.Namespace); namespace != "" && spec.namespaced {
		options = append(options, client.InNamespace(namespace))
	} else if namespace != "" && !spec.namespaced {
		return []ManagedResource{}, nil
	}

	if err := m.client.List(ctx, list, options...); err != nil {
		if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
			return []ManagedResource{}, nil
		}
		return nil, err
	}

	items, err := metaExtractList(list)
	if err != nil {
		return nil, err
	}

	out := make([]ManagedResource, 0, len(items))
	for _, item := range items {
		if name := strings.TrimSpace(filter.Name); name != "" && item.GetName() != name {
			continue
		}

		resource, err := managedResourceFromObject(spec, item)
		if err != nil {
			return nil, err
		}
		out = append(out, resource)
	}

	return out, nil
}

func (m *ResourceManager) listExactMatch(
	ctx context.Context,
	filter ResourceListFilter,
	maxListItems int,
) ([]ManagedResource, pageMetadata, bool, error) {
	kind := strings.TrimSpace(filter.Kind)
	name := strings.TrimSpace(filter.Name)
	if kind == "" || name == "" {
		return nil, pageMetadata{}, false, nil
	}

	spec, err := resourceKindSpecFor(kind)
	if err != nil {
		return nil, pageMetadata{}, true, err
	}

	namespace := strings.TrimSpace(filter.Namespace)
	switch {
	case spec.namespaced && namespace == "":
		return nil, pageMetadata{}, false, nil
	case !spec.namespaced && namespace != "" && namespace != clusterScopeNamespaceMarker:
		items, meta := paginateManagedResources([]ManagedResource{}, filter, maxListItems)
		return items, meta, true, nil
	}

	item, ok, err := m.getBySpec(ctx, spec, namespace, name)
	if err != nil {
		return nil, pageMetadata{}, true, err
	}
	if !ok {
		items, meta := paginateManagedResources([]ManagedResource{}, filter, maxListItems)
		return items, meta, true, nil
	}

	items, meta := paginateManagedResources([]ManagedResource{item}, filter, maxListItems)
	return items, meta, true, nil
}

func resourceListPagination(filter ResourceListFilter) listPagination {
	return listPagination{
		offset:   filter.Offset,
		limit:    filter.Limit,
		hasLimit: filter.HasLimit,
	}
}

func (m *ResourceManager) getBySpec(
	ctx context.Context,
	spec resourceKindSpec,
	namespace, name string,
) (ManagedResource, bool, error) {
	obj := spec.newObject()
	err := m.client.Get(ctx, client.ObjectKey{Namespace: normalizedResourceNamespace(spec, namespace), Name: name}, obj)
	if apierrors.IsNotFound(err) {
		return ManagedResource{}, false, nil
	}
	if err != nil {
		return ManagedResource{}, false, err
	}

	resource, err := managedResourceFromObject(spec, obj)
	if err != nil {
		return ManagedResource{}, false, err
	}

	return resource, true, nil
}

func (m *ResourceManager) kindAvailable(ctx context.Context, spec resourceKindSpec) (bool, string) {
	list := spec.newList()
	if err := m.client.List(ctx, list, client.Limit(1)); err != nil {
		if meta.IsNoMatchError(err) || runtime.IsNotRegisteredError(err) {
			return false, "CRD is not installed on the cluster"
		}
		return true, ""
	}

	return true, ""
}
