package admin

import (
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultAdminListCacheTTL = time.Second

type adminListCache struct {
	mu              sync.Mutex
	ttl             time.Duration
	now             func() time.Time
	resourceLists   map[string]cachedManagedResources
	serviceCatalogs map[string]cachedServiceCatalogEntries
	strings         map[string]cachedStrings
}

type cachedManagedResources struct {
	expiresAt time.Time
	items     []ManagedResource
}

type cachedServiceCatalogEntries struct {
	expiresAt time.Time
	items     []ServiceCatalogEntry
}

type cachedStrings struct {
	expiresAt time.Time
	items     []string
}

func newAdminListCache(ttl time.Duration) *adminListCache {
	return &adminListCache{
		ttl:             ttl,
		now:             time.Now,
		resourceLists:   make(map[string]cachedManagedResources),
		serviceCatalogs: make(map[string]cachedServiceCatalogEntries),
		strings:         make(map[string]cachedStrings),
	}
}

func (c *adminListCache) getManagedResources(key string) ([]ManagedResource, bool) {
	if c == nil || c.ttl <= 0 {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.resourceLists[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.resourceLists, key)
		return nil, false
	}
	return cloneManagedResourceList(entry.items), true
}

func (c *adminListCache) putManagedResources(key string, items []ManagedResource) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.resourceLists[key] = cachedManagedResources{
		expiresAt: c.now().Add(c.ttl),
		items:     cloneManagedResourceList(items),
	}
}

func (c *adminListCache) getServiceCatalogEntries(key string) ([]ServiceCatalogEntry, bool) {
	if c == nil || c.ttl <= 0 {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.serviceCatalogs[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.serviceCatalogs, key)
		return nil, false
	}
	return cloneServiceCatalogEntries(entry.items), true
}

func (c *adminListCache) putServiceCatalogEntries(key string, items []ServiceCatalogEntry) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.serviceCatalogs[key] = cachedServiceCatalogEntries{
		expiresAt: c.now().Add(c.ttl),
		items:     cloneServiceCatalogEntries(items),
	}
}

func (c *adminListCache) getStrings(key string) ([]string, bool) {
	if c == nil || c.ttl <= 0 {
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.strings[key]
	if !ok {
		return nil, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.strings, key)
		return nil, false
	}
	return append([]string(nil), entry.items...), true
}

func (c *adminListCache) putStrings(key string, items []string) {
	if c == nil || c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.strings[key] = cachedStrings{
		expiresAt: c.now().Add(c.ttl),
		items:     append([]string(nil), items...),
	}
}

func (c *adminListCache) clear() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.resourceLists = make(map[string]cachedManagedResources)
	c.serviceCatalogs = make(map[string]cachedServiceCatalogEntries)
	c.strings = make(map[string]cachedStrings)
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

func cloneManagedResourceList(items []ManagedResource) []ManagedResource {
	return append([]ManagedResource(nil), items...)
}

func cloneServiceCatalogEntries(items []ServiceCatalogEntry) []ServiceCatalogEntry {
	return append([]ServiceCatalogEntry(nil), items...)
}
