package domain

import "time"

// RolePermissionPair includes retained mappings, even when deleted.
type RolePermissionPair struct {
	RoleID       uint64     `sql:"column:role_id"`
	PermissionID uint64     `sql:"column:permission_id"`
	DeletedAt    *time.Time `sql:"column:deleted_at"`
}

func (RolePermissionPair) TableName() string { return "role_permissions rp" }

// RolePermissionChanges is a feature-decided, disjoint, sorted mapping diff.
type RolePermissionChanges struct {
	Removed  []uint64
	Restored []uint64
	Inserted []uint64
}
