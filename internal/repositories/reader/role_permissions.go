package reader

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

func (r *roleManagementReader) RolePermissionPairs(ctx context.Context, id uint64) ([]domain.RolePermissionPair, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if domain.ValidateManagementTarget(id) != nil {
		return nil, common.ErrBadRequest
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	rows := []domain.RolePermissionPair{}
	err := r.handle.UseModel(domain.RolePermissionPair{}).WithContext(ctx).
		Select("rp.role_id", "rp.permission_id", "rp.deleted_at").Where("rp.role_id = ?", id).
		OrderBy("rp.permission_id ASC").Scan(&rows)
	if err != nil {
		return nil, helpers.HandleDBError(err)
	}
	var previous uint64
	for _, row := range rows {
		if row.RoleID != id || domain.ValidateManagementTarget(row.PermissionID) != nil || row.PermissionID <= previous || row.DeletedAt != nil && row.DeletedAt.IsZero() {
			return nil, common.ErrDependencyUnavailable
		}
		previous = row.PermissionID
	}
	return rows, nil
}
