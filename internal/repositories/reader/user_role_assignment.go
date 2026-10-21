package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/orm"
)

type userRoleAssignmentReader struct{ handle domain.QueryReader }

func NewUserRoleAssignmentReader(handle domain.QueryReader) domain.UserRoleAssignmentReader {
	return &userRoleAssignmentReader{handle}
}

func assignmentExpressions() map[string]string {
	return map[string]string{"id": "ur.id", "role_id": "r.id", "code": "r.code", "role_code": "r.role_code", "name": "r.role_name", "status": "ur.status", "role_status": "r.status", "application_id": "a.id", "application_code": "a.code", "application_status": "a.status", "is_system": "r.is_system", "application_access_status": "access.status", "granted_by": "ur.granted_by", "granted_at": "ur.granted_at", "revoked_by": "ur.revoked_by", "revoked_at": "ur.revoked_at", "revoke_reason": "ur.revoke_reason"}
}

type assignmentStateRow domain.UserRoleAssignmentItem

func (assignmentStateRow) TableName() string { return "auth target" }

func (r *userRoleAssignmentReader) AssignmentRoles(ctx context.Context, user uint64, ids []uint64) ([]domain.UserRoleAssignmentItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	if domain.ValidateManagementTarget(user) != nil || len(ids) < 1 || len(ids) > 20 {
		return nil, common.ErrBadRequest
	}
	for _, id := range ids {
		if domain.ValidateManagementTarget(id) != nil {
			return nil, common.ErrBadRequest
		}
	}
	rows, err := r.readAssignmentRoles(ctx, user, ids)
	if err != nil {
		return nil, helpers.HandleDBError(err)
	}
	items := make([]domain.UserRoleAssignmentItem, len(rows))
	for i, row := range rows {
		items[i] = domain.UserRoleAssignmentItem(row)
	}
	return items, nil
}

func (r *userRoleAssignmentReader) ListAssignments(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.UserRoleAssignmentItem], error) {
	page, err := readCatalog[domain.UserRoleAssignmentItem](ctx, r.handle, domain.UserRoleAssignmentCatalog, q)
	if err != nil {
		return orm.PageData[domain.UserRoleAssignmentItem]{}, err
	}
	for i, row := range page.Items {
		page.Items[i] = row.Project(q)
	}
	return page, nil
}

func (r *userRoleAssignmentReader) readAssignmentRoles(ctx context.Context, user uint64, ids []uint64) ([]assignmentStateRow, error) {
	expressions := assignmentExpressions()
	expressions["id"] = "COALESCE(ur.id,0)"
	expressions["status"] = "COALESCE(ur.status,'')"
	columns := []string{}
	for _, f := range domain.UserRoleAssignmentCatalog.Fields() {
		columns = append(columns, expressions[f.Name]+" AS "+f.Name)
	}
	var rows []assignmentStateRow
	// Anchor on the bound target so both LEFT JOIN conditions remain static;
	// requested roles are bounded and parameterized, including absent pairs.
	err := r.handle.UseModel(assignmentStateRow{}).WithContext(ctx).
		Join("role r", "TRUE").Join("applications a", "a.id = r.application_id").
		LeftJoin("user_role ur", "ur.user_id = target.id AND ur.role_id = r.id").
		LeftJoin("user_application_access access", "access.user_id = target.id AND access.application_id = a.id").
		Where("target.id = ?", user).WhereIn("r.id", ids).Select(columns...).OrderBy("r.id ASC").Scan(&rows)
	return rows, err
}
