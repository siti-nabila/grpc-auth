package writer

import (
	"context"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type rolePermissionInsert struct {
	RoleID       uint64     `sql:"column:role_id"`
	PermissionID uint64     `sql:"column:permission_id"`
	DeletedAt    *time.Time `sql:"column:deleted_at"`
}

func (rolePermissionInsert) TableName() string { return "role_permissions" }

type rolePermissionPatch struct {
	DeletedAt *time.Time `sql:"column:deleted_at"`
}

func (rolePermissionPatch) TableName() string { return "role_permissions" }

func (w *roleManagementWriter) ReplacePermissions(ctx context.Context, id uint64, changes domain.RolePermissionChanges, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w.tx == nil {
		return common.ErrDependencyUnavailable
	}
	if domain.ValidateManagementTarget(id) != nil || at.IsZero() {
		return common.ErrBadRequest
	}
	seen := map[uint64]bool{}
	for _, ids := range [][]uint64{changes.Removed, changes.Restored, changes.Inserted} {
		var previous uint64
		for _, permissionID := range ids {
			if domain.ValidateManagementTarget(permissionID) != nil || permissionID <= previous || seen[permissionID] {
				return common.ErrBadRequest
			}
			previous = permissionID
			seen[permissionID] = true
		}
	}
	utc := at.UTC()
	for _, permissionID := range changes.Removed {
		if err := w.changePermission(ctx, id, permissionID, &utc); err != nil {
			return err
		}
	}
	for _, permissionID := range changes.Restored {
		if err := w.changePermission(ctx, id, permissionID, nil); err != nil {
			return err
		}
	}
	if len(changes.Inserted) == 0 {
		return nil
	}
	insert, ok := w.tx.(domain.InsertWriter)
	if !ok {
		return common.ErrDependencyUnavailable
	}
	for _, permissionID := range changes.Inserted {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := insert.CreateWith(&rolePermissionInsert{RoleID: id, PermissionID: permissionID}).ExecResult()
		changed, err := managementAffected(result, err)
		if err != nil {
			return err
		}
		if !changed {
			return common.ErrDependencyUnavailable
		}
	}
	return nil
}

func (w *roleManagementWriter) changePermission(ctx context.Context, roleID, permissionID uint64, at *time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	guard := "deleted_at IS NOT NULL"
	if at != nil {
		guard = "deleted_at IS NULL"
	}
	patch := rolePermissionPatch{DeletedAt: at}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).Where(guard).Updates()
	changed, err := managementAffected(result, err)
	if err != nil {
		return err
	}
	if !changed {
		return common.ErrDependencyUnavailable
	}
	return nil
}
