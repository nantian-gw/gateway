package config

import "fmt"

// Permission represents an RBAC permission string.
type Permission string

const (
	PermissionRead           Permission = "read:*"
	PermissionWriteResources Permission = "write:resources"
	PermissionWriteChatbot   Permission = "write:chatbot"
	PermissionWriteMetrics   Permission = "write:metrics"
	PermissionAdmin          Permission = "admin:*"
)

// AllPermissions returns a slice of all defined permissions.
func AllPermissions() []Permission {
	return []Permission{
		PermissionRead,
		PermissionWriteResources,
		PermissionWriteChatbot,
		PermissionWriteMetrics,
		PermissionAdmin,
	}
}

// IsValid returns true if p is a recognised permission.
func (p Permission) IsValid() bool {
	switch p {
	case PermissionRead,
		PermissionWriteResources,
		PermissionWriteChatbot,
		PermissionWriteMetrics,
		PermissionAdmin:
		return true
	default:
		return false
	}
}

// PermissionSet is a set of permissions with efficient membership checks.
type PermissionSet map[Permission]bool

// NewPermissionSet creates a PermissionSet from a list of permissions.
func NewPermissionSet(perms ...Permission) PermissionSet {
	s := make(PermissionSet, len(perms))
	for _, p := range perms {
		s[p] = true
	}
	return s
}

// HasAdmin reports whether the set contains the admin:* permission.
func (s PermissionSet) HasAdmin() bool {
	return s[PermissionAdmin]
}

// Has reports whether the given permission is present.
// admin:* grants every permission.
func (s PermissionSet) Has(p Permission) bool {
	if s.HasAdmin() {
		return true
	}
	return s[p]
}

// Expand returns a slice of all permissions held by this set,
// resolving admin:* into all known permissions.
func (s PermissionSet) Expand() []Permission {
	if s.HasAdmin() {
		return AllPermissions()
	}
	out := make([]Permission, 0, len(s))
	for p := range s {
		out = append(out, p)
	}
	return out
}

// Role defines a named set of permissions and subject-matching rules.
type Role struct {
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
	MatchUsers  []string     `json:"matchUsers,omitempty"`
	MatchGroups []string     `json:"matchGroups,omitempty"`
}

// AdminRBACConfig holds the RBAC role definitions for the admin API.
type AdminRBACConfig struct {
	Roles []Role `json:"roles"`
}

// IsEnabled reports whether at least one role is defined.
func (c *AdminRBACConfig) IsEnabled() bool {
	return c != nil && len(c.Roles) > 0
}

// Validate checks the configuration for consistency and returns the first
// error encountered, or nil.
func (c *AdminRBACConfig) Validate() error {
	seen := make(map[string]bool, len(c.Roles))
	for i, role := range c.Roles {
		if role.Name == "" {
			return fmt.Errorf("role[%d]: name is required", i)
		}
		if seen[role.Name] {
			return fmt.Errorf("role[%d]: duplicate role name %q", i, role.Name)
		}
		seen[role.Name] = true

		if len(role.Permissions) == 0 {
			return fmt.Errorf("role %q: must have at least one permission", role.Name)
		}
		for j, p := range role.Permissions {
			if !p.IsValid() {
				return fmt.Errorf("role %q, permission[%d]: %q is not a valid permission", role.Name, j, p)
			}
		}

		for j, u := range role.MatchUsers {
			if u == "" {
				return fmt.Errorf("role %q, matchUsers[%d]: empty string not allowed", role.Name, j)
			}
		}
		for j, g := range role.MatchGroups {
			if g == "" {
				return fmt.Errorf("role %q, matchGroups[%d]: empty string not allowed", role.Name, j)
			}
		}
	}
	return nil
}

// Authorize checks whether the given identity owns the required permission.
// It returns the matched role name and true on success, or ("", false) when
// authorization fails (including when RBAC is disabled).
func (c *AdminRBACConfig) Authorize(username string, groups []string, required Permission) (string, bool) {
	if c == nil {
		return "", false
	}
	for _, role := range c.Roles {
		if !c.subjectMatches(role, username, groups) {
			continue
		}
		ps := NewPermissionSet(role.Permissions...)
		if ps.Has(required) {
			return role.Name, true
		}
	}
	return "", false
}

// subjectMatches returns true when username or any group matches the role's
// subject rules. Stripped-down version that operates on strings to avoid
// circular imports with the admin.Identity type.
func (c *AdminRBACConfig) subjectMatches(role Role, username string, groups []string) bool {
	for _, u := range role.MatchUsers {
		if u == username {
			return true
		}
	}
	for _, g := range role.MatchGroups {
		for _, ug := range groups {
			if g == ug {
				return true
			}
		}
	}
	return false
}
