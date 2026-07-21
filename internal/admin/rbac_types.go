package admin

import "github.com/nantian-gw/gateway/internal/config"

// Reexported RBAC types so callers in the admin package can use them
// without importing config directly.

type Permission = config.Permission
type PermissionSet = config.PermissionSet
type Role = config.Role
type AdminRBACConfig = config.AdminRBACConfig

const (
	PermissionRead           = config.PermissionRead
	PermissionWriteResources = config.PermissionWriteResources
	PermissionWriteChatbot   = config.PermissionWriteChatbot
	PermissionWriteMetrics   = config.PermissionWriteMetrics
	PermissionAdmin          = config.PermissionAdmin
)
