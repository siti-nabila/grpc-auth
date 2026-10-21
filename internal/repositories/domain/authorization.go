package domain

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type PermissionReader interface {
	Permissions(context.Context, uint64) ([]PermissionContribution, error)
}

type PermissionContribution struct {
	PermissionCode          string                  `sql:"column:permission_code"`
	PermissionApplicationID uint64                  `sql:"column:permission_application_id"`
	RoleID                  uint64                  `sql:"column:role_id"`
	RoleCode                string                  `sql:"column:role_code"`
	RoleApplicationID       uint64                  `sql:"column:role_application_id"`
	RoleStatus              types.RoleStatus        `sql:"column:role_status"`
	AssignmentStatus        types.AssignmentStatus  `sql:"column:assignment_status"`
	ApplicationCode         types.ApplicationCode   `sql:"column:application_code"`
	ApplicationStatus       types.ApplicationStatus `sql:"column:application_status"`
	AccessStatus            sql.NullString          `sql:"column:access_status"`
}

func (PermissionContribution) TableName() string { return "user_role ur" }

type PermissionRole struct {
	ID   uint64
	Code string
}
type EffectivePermission struct {
	Code  string
	Roles []PermissionRole
}
