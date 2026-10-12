package reader

import (
	"context"
	"errors"

	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
	"github.com/siti-nabila/orm/query"
)

type roleManagementReader struct{ handle domain.QueryReader }

func NewRoleManagementReader(handle domain.QueryReader) domain.RoleManagementReader {
	return &roleManagementReader{handle}
}

const affectedRoleUsers = "(SELECT COUNT(DISTINCT ur.user_id) FROM user_role ur JOIN auth u ON u.id = ur.user_id WHERE ur.role_id = r.id AND ur.status = 'ACTIVE')"

func roleSummaryQuery(handle domain.QueryReader) *query.QueryBuilder {
	return handle.UseModel(domain.RoleSummary{}).Join("applications a", "a.id = r.application_id").Select(
		"r.id", "r.code", "r.role_name AS name", "r.role_description AS description", "r.application_id", "a.code AS application_code", "r.status", "r.is_system", "r.version", "r.role_code", "r.created_at", "r.updated_at", affectedRoleUsers+" AS affected_user_count")
}

func (r *roleManagementReader) Role(ctx context.Context, id uint64) (row domain.RoleSummary, err error) {
	if err = ctx.Err(); err != nil {
		return row, err
	}
	if domain.ValidateManagementTarget(id) != nil {
		return row, common.ErrBadRequest
	}
	if r.handle == nil {
		return row, common.ErrDependencyUnavailable
	}
	err = roleSummaryQuery(r.handle).WithContext(ctx).Where("r.id = ?", id).Limit(1).Scan(&row)
	if err != nil {
		return domain.RoleSummary{}, roleLookupError(err, roleerrors.ErrRoleNotFound)
	}
	if row.ID == 0 {
		return row, roleerrors.ErrRoleNotFound
	}
	if row.ID != id {
		return domain.RoleSummary{}, common.ErrDependencyUnavailable
	}
	if err = row.Validate(); err != nil {
		return domain.RoleSummary{}, errors.Join(common.ErrDependencyUnavailable, err)
	}
	return row, nil
}

func (r *roleManagementReader) Application(ctx context.Context, code string) (row domain.ApplicationItem, err error) {
	if err = ctx.Err(); err != nil {
		return row, err
	}
	if r.handle == nil {
		return row, common.ErrDependencyUnavailable
	}
	err = r.handle.UseModel(domain.ApplicationItem{}).WithContext(ctx).Select("a.id", "a.code", "a.name", "a.status", "a.is_system").Where("a.code = ?", code).Limit(1).Scan(&row)
	if err != nil {
		return domain.ApplicationItem{}, roleLookupError(err, applicationerrors.ErrApplicationNotFound)
	}
	if row.ID == 0 {
		return row, applicationerrors.ErrApplicationNotFound
	}
	if domain.ValidateManagementTarget(row.ID) != nil {
		return domain.ApplicationItem{}, common.ErrDependencyUnavailable
	}
	return row, nil
}

func roleLookupError(err, missing error) error {
	var dbErr *normalizeerr.DBError
	if errors.As(err, &dbErr) && dbErr.Kind == normalizeerr.KindRowNotFound {
		return errors.Join(missing, err)
	}
	return helpers.HandleDBError(err)
}

func permissionItemsQuery(handle domain.QueryReader) *query.QueryBuilder {
	return handle.UseModel(domain.PermissionItem{}).Join("applications a", "a.id = p.application_id").Select("p.id", "p.application_id", "a.code AS application_code", "p.code", "p.description")
}

func (r *roleManagementReader) RolePermissions(ctx context.Context, id uint64) ([]domain.PermissionItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if domain.ValidateManagementTarget(id) != nil {
		return nil, common.ErrBadRequest
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	rows := []domain.PermissionItem{}
	err := permissionItemsQuery(r.handle).WithContext(ctx).Join("role_permissions rp", "rp.permission_id = p.id").Where("rp.role_id = ?", id).OrderBy(`p.code COLLATE "C" ASC`, "p.id ASC").Scan(&rows)
	if err != nil {
		return nil, helpers.HandleDBError(err)
	}
	return rows, nil
}

func (r *roleManagementReader) PermissionSet(ctx context.Context, appID uint64, codes []string) ([]domain.PermissionItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if domain.ValidateManagementTarget(appID) != nil {
		return nil, common.ErrBadRequest
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	codes = domain.NormalizePermissionCodes(codes)
	if err := domain.ValidatePermissionCodes(codes, ""); err != nil {
		return nil, err
	}
	rows := []domain.PermissionItem{}
	if len(codes) == 0 {
		return rows, nil
	}
	err := permissionItemsQuery(r.handle).WithContext(ctx).Where("p.application_id = ?", appID).WhereIn("p.code", codes).OrderBy(`p.code COLLATE "C" ASC`, "p.id ASC").Scan(&rows)
	if err != nil {
		return nil, helpers.HandleDBError(err)
	}
	if len(rows) != len(codes) {
		return nil, roleerrors.ErrInvalidPermissionSet
	}
	for i, row := range rows {
		if row.Code != codes[i] || row.ApplicationID != appID || domain.ValidateManagementTarget(row.ID) != nil {
			return nil, roleerrors.ErrInvalidPermissionSet
		}
	}
	return rows, nil
}
