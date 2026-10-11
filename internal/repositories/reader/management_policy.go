package reader

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type managementPolicyReader struct{ handle domain.QueryReader }

func NewManagementPolicyReader(handle domain.QueryReader) domain.ManagementPolicyReader {
	return &managementPolicyReader{handle}
}

type managementRole struct {
	ApplicationID uint64 `sql:"column:application_id"`
}

func (managementRole) TableName() string { return "role" }

func (r *managementPolicyReader) RoleApplication(ctx context.Context, id uint64) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if r.handle == nil {
		return 0, common.ErrDependencyUnavailable
	}
	var row managementRole
	err := r.handle.UseModel(managementRole{}).WithContext(ctx).Select("application_id").Where("id = ?", id).Limit(1).Scan(&row)
	if err != nil {
		return 0, helpers.HandleDBError(err)
	}
	if row.ApplicationID == 0 {
		return 0, common.ErrDependencyUnavailable
	}
	return row.ApplicationID, nil
}

type managementAdminCount struct {
	ID          int64 `sql:"column:id"`
	Total       int64 `sql:"column:total"`
	TargetAdmin bool  `sql:"column:target_admin"`
}

func (managementAdminCount) TableName() string { return "admin_guard" }

// The global last-admin lock is owned by the feature before this read. The
// singleton must already exist; never manufacture a baseline or first admin.
func (r *managementPolicyReader) AdminCount(ctx context.Context, target uint64) (int64, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if r.handle == nil {
		return 0, false, common.ErrDependencyUnavailable
	}
	var row managementAdminCount
	err := r.handle.UseModel(managementAdminCount{}).WithContext(ctx).Select("id").Where("id = ?", 1).Limit(1).Scan(&row)
	if err != nil {
		return 0, false, helpers.HandleDBError(err)
	}
	if row.ID != 1 {
		return 0, false, common.ErrDependencyUnavailable
	}
	var ids []managementAdminID
	err = r.handle.UseModel(managementAdminID{}).WithContext(ctx).Select("DISTINCT a.id").Join("user_role ur", "ur.user_id = a.id").Join("role r", "r.id = ur.role_id").Where("a.deleted_at IS NULL").Where("a.status = ? AND ur.status = ? AND r.code = ?", types.UserActive, types.AssignmentActive, types.RoleIAMPlatformAdmin).Scan(&ids)
	if err != nil {
		return 0, false, helpers.HandleDBError(err)
	}
	for _, id := range ids {
		if domain.ValidateManagementTarget(id.ID) != nil {
			return 0, false, common.ErrDependencyUnavailable
		}
		if id.ID == target {
			row.TargetAdmin = true
		}
	}
	row.Total = int64(len(ids))
	return row.Total, row.TargetAdmin, nil
}

type managementAdminID struct {
	ID uint64 `sql:"column:id"`
}

func (managementAdminID) TableName() string { return "auth a" }
