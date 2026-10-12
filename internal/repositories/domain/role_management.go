package domain

import (
	"context"
	"strings"
	"time"

	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// RoleSummary is the public management view. It cannot carry credentials or
// assignment identities. NumericCode preserves the legacy numeric role code.
type RoleSummary struct {
	ID                uint64                `sql:"column:id" json:"id"`
	Code              string                `sql:"column:code" json:"code"`
	Name              string                `sql:"column:name" json:"name"`
	Description       *string               `sql:"column:description" json:"description,omitempty"`
	ApplicationID     uint64                `sql:"column:application_id" json:"application_id"`
	ApplicationCode   types.ApplicationCode `sql:"column:application_code" json:"application_code"`
	Status            types.RoleStatus      `sql:"column:status" json:"status"`
	IsSystem          bool                  `sql:"column:is_system" json:"is_system"`
	Version           int64                 `sql:"column:version" json:"version"`
	NumericCode       int32                 `sql:"column:role_code" json:"role_code"`
	CreatedAt         *time.Time            `sql:"column:created_at" json:"created_at,omitempty"`
	UpdatedAt         *time.Time            `sql:"column:updated_at" json:"updated_at,omitempty"`
	AffectedUserCount uint64                `sql:"column:affected_user_count" json:"affected_user_count"`
}

func (RoleSummary) TableName() string { return "role r" }

type RoleDetail struct {
	Role        RoleSummary
	Permissions []PermissionItem
}

type PermissionItem struct {
	ID              uint64                `sql:"column:id" json:"id"`
	ApplicationID   uint64                `sql:"column:application_id" json:"application_id"`
	ApplicationCode types.ApplicationCode `sql:"column:application_code" json:"application_code"`
	Code            string                `sql:"column:code" json:"code"`
	Description     string                `sql:"column:description" json:"description"`
}

func (PermissionItem) TableName() string { return "permissions p" }

type ApplicationItem struct {
	ID       uint64                  `sql:"column:id" json:"id"`
	Code     types.ApplicationCode   `sql:"column:code" json:"code"`
	Name     string                  `sql:"column:name" json:"name"`
	Status   types.ApplicationStatus `sql:"column:status" json:"status"`
	IsSystem bool                    `sql:"column:is_system" json:"is_system"`
}

func (ApplicationItem) TableName() string { return "applications a" }

// RoleManagementReader is separate from the registration/default Member reader.
type RoleManagementReader interface {
	Role(context.Context, uint64) (RoleSummary, error)
	Application(context.Context, string) (ApplicationItem, error)
	RolePermissions(context.Context, uint64) ([]PermissionItem, error)
	PermissionSet(context.Context, uint64, []string) ([]PermissionItem, error)
}

type RoleManagementWriter interface {
	CreateRole(context.Context, RoleSummary) (RoleSummary, error)
	UpdateRole(context.Context, RoleSummary, int64) (bool, error)
	ReplacePermissions(context.Context, uint64, []uint64) error
}

func (r RoleSummary) Protect() error {
	if r.IsSystem || r.ApplicationCode == types.ApplicationIAM || strings.HasPrefix(r.Code, string(types.ApplicationIAM)+".") {
		return roleerrors.ErrSystemRoleProtected
	}
	return nil
}

func (a ApplicationItem) AllowCustomRole() error {
	if a.IsSystem || a.Code == types.ApplicationIAM {
		return applicationerrors.ErrSystemApplicationProtected
	}
	if a.Status != types.ApplicationActive {
		return applicationerrors.ErrApplicationInactive
	}
	return nil
}

func (r RoleSummary) Validate() error {
	if ValidateManagementTarget(r.ID) != nil || ValidateManagementTarget(r.ApplicationID) != nil || r.Version < 1 || r.NumericCode < 1 || !RoleStatusValid(r.Status) || !roleCodeValid(r.Code) || !applicationCodeValid(string(r.ApplicationCode)) || !strings.HasPrefix(r.Code, string(r.ApplicationCode)+".") || !userText(r.Name, 1, 50) {
		return common.ErrBadRequest
	}
	if r.Description != nil && !userText(*r.Description, 0, 100) {
		return common.ErrBadRequest
	}
	for _, at := range []*time.Time{r.CreatedAt, r.UpdatedAt} {
		if at != nil && at.IsZero() {
			return common.ErrBadRequest
		}
	}
	return nil
}

func RoleStatusValid(status types.RoleStatus) bool {
	return status == types.RoleActive || status == types.RoleDisabled
}
