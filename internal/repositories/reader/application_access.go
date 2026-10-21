package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/orm"
)

type applicationAccessReader struct{ handle domain.QueryReader }

func NewApplicationAccessReader(handle domain.QueryReader) domain.ApplicationAccessReader {
	return &applicationAccessReader{handle}
}

func accessExpressions() map[string]string {
	return map[string]string{"application_id": "a.id", "application_code": "a.code", "name": "a.name", "application_status": "a.status", "is_system": "a.is_system", "status": "access.status", "granted_by": "access.granted_by", "granted_at": "access.granted_at", "revoked_by": "access.revoked_by", "revoked_at": "access.revoked_at", "revoke_reason": "access.revoke_reason"}
}

func (r *applicationAccessReader) Access(ctx context.Context, userID, applicationID uint64) (*domain.ApplicationAccessItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	if domain.ValidateManagementTarget(userID) != nil || domain.ValidateManagementTarget(applicationID) != nil {
		return nil, common.ErrBadRequest
	}
	expressions := accessExpressions()
	columns := []string{"access.user_id AS user_id"}
	for _, field := range domain.UserApplicationAccessCatalog.Fields() {
		columns = append(columns, expressions[field.Name]+" AS "+field.Name)
	}
	var rows []domain.ApplicationAccessItem
	err := r.handle.UseModel(domain.ApplicationAccessItem{}).WithContext(ctx).
		Join("applications a", "a.id = access.application_id").
		Where("access.user_id = ? AND access.application_id = ?", userID, applicationID).
		Select(columns...).Limit(2).Scan(&rows)
	if err != nil {
		return nil, helpers.HandleDBError(err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 || rows[0].UserID != userID || rows[0].ApplicationID != applicationID || !validAccessRow(rows[0]) {
		return nil, common.ErrDependencyUnavailable
	}
	return &rows[0], nil
}

func (r *applicationAccessReader) ApplicationAssignments(ctx context.Context, userID, applicationID uint64) ([]domain.UserRoleAssignmentItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	if domain.ValidateManagementTarget(userID) != nil || domain.ValidateManagementTarget(applicationID) != nil {
		return nil, common.ErrBadRequest
	}
	expressions := assignmentExpressions()
	var columns []string
	for _, field := range domain.UserRoleAssignmentCatalog.Fields() {
		columns = append(columns, expressions[field.Name]+" AS "+field.Name)
	}
	var rows []domain.UserRoleAssignmentItem
	err := r.handle.UseModel(domain.UserRoleAssignmentItem{}).WithContext(ctx).
		Join("role r", "r.id = ur.role_id").Join("applications a", "a.id = r.application_id").
		LeftJoin("user_application_access access", "access.user_id = ur.user_id AND access.application_id = a.id").
		Where("ur.user_id = ? AND r.application_id = ? AND ur.status = ?", userID, applicationID, "ACTIVE").
		Select(columns...).OrderBy("r.id ASC").Scan(&rows)
	if err != nil {
		return nil, helpers.HandleDBError(err)
	}
	for i, row := range rows {
		if domain.ValidateManagementTarget(row.ID) != nil || domain.ValidateManagementTarget(row.RoleID) != nil || row.ApplicationID != applicationID || row.Status != "ACTIVE" || row.Code == "" || row.ApplicationCode == "" || i > 0 && row.RoleID <= rows[i-1].RoleID {
			return nil, common.ErrDependencyUnavailable
		}
		if row.RoleStatus != "ACTIVE" && row.RoleStatus != "DISABLED" || row.ApplicationStatus != "ACTIVE" && row.ApplicationStatus != "DISABLED" {
			return nil, common.ErrDependencyUnavailable
		}
	}
	return rows, nil
}

func (r *applicationAccessReader) ListAccess(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationAccessItem], error) {
	page, err := readCatalog[domain.ApplicationAccessItem](ctx, r.handle, domain.UserApplicationAccessCatalog, q)
	if err != nil {
		return orm.PageData[domain.ApplicationAccessItem]{}, err
	}
	for i, row := range page.Items {
		if !validAccessRow(row) {
			return orm.PageData[domain.ApplicationAccessItem]{}, common.ErrDependencyUnavailable
		}
		page.Items[i] = row.Project(q)
	}
	return page, nil
}

func validAccessRow(row domain.ApplicationAccessItem) bool {
	if domain.ValidateManagementTarget(row.ApplicationID) != nil || row.ApplicationCode == "" || row.Name == "" || row.GrantedAt == nil || row.GrantedAt.IsZero() {
		return false
	}
	if row.ApplicationStatus != "ACTIVE" && row.ApplicationStatus != "DISABLED" {
		return false
	}
	for _, id := range []*uint64{row.GrantedBy, row.RevokedBy} {
		if id != nil && domain.ValidateManagementTarget(*id) != nil {
			return false
		}
	}
	if row.Status == "ACTIVE" {
		return row.RevokedAt == nil
	}
	return row.Status == "REVOKED" && row.RevokedAt != nil && !row.RevokedAt.IsZero() && row.RevokeReason != nil && domain.ValidateAssignmentReason(*row.RevokeReason) == nil
}
