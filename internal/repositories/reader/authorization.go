package reader

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type permissionReader struct{ handle domain.QueryReader }

func NewPermissionReader(handle domain.QueryReader) domain.PermissionReader {
	return &permissionReader{handle: handle}
}

func (r *permissionReader) Permissions(ctx context.Context, userID uint64) ([]domain.PermissionContribution, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.handle == nil {
		return nil, common.ErrInternalServerError
	}
	var rows []domain.PermissionContribution
	err := r.handle.UseModel(domain.PermissionContribution{}).WithContext(ctx).Select(
		"p.code AS permission_code", "p.application_id AS permission_application_id",
		"r.id AS role_id", "r.code AS role_code", "r.application_id AS role_application_id", "r.status AS role_status",
		"ur.status AS assignment_status", "app.code AS application_code", "app.status AS application_status", "access.status AS access_status",
	).Join("role r", "r.id = ur.role_id").Join("role_permissions rp", "rp.role_id = r.id").Join("permissions p", "p.id = rp.permission_id").
		Join("applications app", "app.id = r.application_id").LeftJoin("user_application_access access", "access.user_id = ur.user_id AND access.application_id = app.id").
		Where("ur.user_id = ?", userID).OrderBy("p.code ASC, r.id ASC").Scan(&rows)
	return rows, helpers.HandleDBError(err)
}
