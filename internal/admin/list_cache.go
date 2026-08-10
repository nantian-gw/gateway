package admin

import (
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultListCacheTTL = time.Second

type cacheEntry[T any] struct {
	expiresAt time.Time
	items     []T
}

type listCache struct {
	mu              sync.Mutex
	ttl             time.Duration
	now             func() time.Time
	resourceLists   map[string]cacheEntry[ManagedResource]
	serviceCatalogs map[string]cacheEntry[ServiceCatalogEntry]
	strings         map[string]cacheEntry[string]
}

func newListCache(ttl time.Duration) *listCache {
	return &listCache{
		ttl:             ttl,
		now:             time.Now,
		resourceLists:   make(map[string]cacheEntry[ManagedResource]),
		serviceCatalogs: make(map[string]cacheEntry[ServiceCatalogEntry]),
		strings:         make(map[string]cacheEntry[string]),
	}
}

func getCached[T any](c *listCache, store map[string]cacheEntry[T], key string) ([]T, bool) {
	if c == nil || c.ttl <= 0 {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := store[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(store, key)
		return nil, false
	}
	return entry.items, true
}

func putCached[T any](c *listCache, store map[string]cacheEntry[T], key string, items []T) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	store[key] = cacheEntry[T]{
		expiresAt: c.now().Add(c.ttl),
		items:     slices.Clone(items),
	}
}

func (c *listCache) getManagedResources(key string) ([]ManagedResource, bool) {
	return getCached(c, c.resourceLists, key)
}

func (c *listCache) putManagedResources(key string, items []ManagedResource) {
	putCached(c, c.resourceLists, key, items)
}

func (c *listCache) getServiceCatalogEntries(key string) ([]ServiceCatalogEntry, bool) {
	return getCached(c, c.serviceCatalogs, key)
}

func (c *listCache) putServiceCatalogEntries(key string, items []ServiceCatalogEntry) {
	putCached(c, c.serviceCatalogs, key, items)
}

func (c *listCache) getStrings(key string) ([]string, bool) {
	items, ok := getCached(c, c.strings, key)
	if !ok {
		return nil, false
	}
	return slices.Clone(items), true
}

func (c *listCache) putStrings(key string, items []string) {
	putCached(c, c.strings, key, items)
}

func (c *listCache) clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.resourceLists = make(map[string]cacheEntry[ManagedResource])
	c.serviceCatalogs = make(map[string]cacheEntry[ServiceCatalogEntry])
	c.strings = make(map[string]cacheEntry[string])
}

func resourceListCacheKey(filter ResourceListFilter, canonicalKind string) string {
	return strings.Join([]string{
		canonicalKind,
		strings.TrimSpace(filter.Namespace),
		strings.TrimSpace(filter.Name),
	}, "\x00")
}

func serviceCatalogCacheKey(filter ServiceCatalogFilter) string {
	return strings.Join([]string{
		strings.TrimSpace(filter.Namespace),
		strings.TrimSpace(filter.Name),
		strings.TrimSpace(filter.Protocol),
		strconv.Itoa(cacheKeyPort(filter.Port, filter.HasPort)),
		strconv.FormatBool(filter.HasPort),
		string(filter.Sort),
		strconv.Itoa(int(filter.Order)),
	}, "\x00")
}

func cacheKeyPort(port int, hasPort bool) int {
	if !hasPort {
		return 0
	}
	return port
}
