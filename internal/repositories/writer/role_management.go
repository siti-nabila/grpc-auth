package writer

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/lib/pq"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
)

type roleManagementWriter struct{ tx domain.Transaction }

func NewRoleManagementWriter(tx domain.Transaction) domain.RoleManagementWriter {
	return &roleManagementWriter{tx}
}

// Numeric role_code is intentionally omitted: PostgreSQL allocates it from the
// verified sequence. The insert returns only the primary ID, then the same
// transaction reads the complete public result including DB timestamps.
type newRole struct {
	ID            uint64           `sql:"column:id;primaryKey"`
	Name          string           `sql:"column:role_name"`
	Description   *string          `sql:"column:role_description"`
	ApplicationID uint64           `sql:"column:application_id"`
	Code          string           `sql:"column:code"`
	Status        types.RoleStatus `sql:"column:status"`
	IsSystem      bool             `sql:"column:is_system"`
	Version       int64            `sql:"column:version"`
}

func (newRole) TableName() string { return "role" }

func (w *roleManagementWriter) CreateRole(ctx context.Context, role domain.RoleSummary) (domain.RoleSummary, error) {
	if err := ctx.Err(); err != nil {
		return domain.RoleSummary{}, err
	}
	if w.tx == nil {
		return domain.RoleSummary{}, common.ErrDependencyUnavailable
	}
	in := (domain.CreateRoleInput{ApplicationCode: string(role.ApplicationCode), Code: role.Code, Name: role.Name, Description: role.Description}).Normalized()
	if in.ValidateDefinition() != nil || domain.ValidateManagementTarget(role.ApplicationID) != nil || role.ID != 0 || role.NumericCode != 0 || role.Version != 1 || role.Status != types.RoleActive || role.IsSystem {
		return domain.RoleSummary{}, common.ErrBadRequest
	}
	row := newRole{Name: in.Name, Description: in.Description, ApplicationID: role.ApplicationID, Code: in.Code, Status: role.Status, Version: 1}
	if err := w.tx.Create(&row); err != nil {
		return domain.RoleSummary{}, rolePersistenceError(err)
	}
	if err := ctx.Err(); err != nil {
		return domain.RoleSummary{}, err
	}
	return reader.NewRoleManagementReader(w.tx).Role(ctx, row.ID)
}

type rolePatch struct {
	Name        string           `sql:"column:role_name"`
	Description *string          `sql:"column:role_description"`
	Status      types.RoleStatus `sql:"column:status"`
	Version     int64            `sql:"column:version"`
	UpdatedAt   time.Time        `sql:"column:updated_at"`
}

func (rolePatch) TableName() string { return "role" }

func (w *roleManagementWriter) UpdateRole(ctx context.Context, role domain.RoleSummary, expected int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if w.tx == nil {
		return false, common.ErrDependencyUnavailable
	}
	if role.Validate() != nil || expected < 1 || expected == math.MaxInt64 || role.Version != expected+1 || role.UpdatedAt == nil {
		return false, common.ErrBadRequest
	}
	patch := rolePatch{role.Name, role.Description, role.Status, role.Version, role.UpdatedAt.UTC()}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).Where("id = ? AND version = ?", role.ID, expected).Updates()
	return managementAffected(result, rolePersistenceError(err))
}

func (w *roleManagementWriter) ReplacePermissions(ctx context.Context, id uint64, ids []uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w.tx == nil {
		return common.ErrDependencyUnavailable
	}
	if domain.ValidateManagementTarget(id) != nil {
		return common.ErrBadRequest
	}
	ids = append([]uint64(nil), ids...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i, permissionID := range ids {
		if domain.ValidateManagementTarget(permissionID) != nil || i > 0 && ids[i-1] == permissionID {
			return common.ErrBadRequest
		}
	}
	exec, ok := w.tx.(domain.StatementWriter)
	if !ok {
		return common.ErrDependencyUnavailable
	}
	result, err := exec.ExecContext(ctx, "DELETE FROM role_permissions WHERE role_id = $1", id)
	if err != nil {
		return err
	}
	if result == nil {
		return common.ErrDependencyUnavailable
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n < 0 {
		return common.ErrDependencyUnavailable
	}
	for _, permissionID := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := exec.ExecContext(ctx, "INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2)", id, permissionID)
		if err != nil {
			return err
		}
		if result == nil {
			return common.ErrDependencyUnavailable
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return common.ErrDependencyUnavailable
		}
	}
	return nil
}

func rolePersistenceError(err error) error {
	if err == nil {
		return nil
	}
	var dbErr *normalizeerr.DBError
	var pgErr *pq.Error
	if errors.As(err, &dbErr) && dbErr.Kind == normalizeerr.KindDuplicateRow || errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errors.Join(roleerrors.ErrRoleCodeAlreadyExists, err)
	}
	return err
}
