package reader

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type userDetailReader struct{ handle domain.QueryReader }

func NewUserDetailReader(handle domain.QueryReader) domain.UserDetailReader {
	return &userDetailReader{handle: handle}
}

func (r *userDetailReader) Detail(ctx context.Context, id uint64) (detail domain.UserDetail, err error) {
	if err = ctx.Err(); err != nil {
		return detail, err
	}
	if err = domain.ValidateManagementTarget(id); err != nil {
		return detail, err
	}
	if r.handle == nil {
		return detail, common.ErrDependencyUnavailable
	}
	err = r.handle.UseModel(domain.UserDetail{}).WithContext(ctx).
		LeftJoin("profile p", "p.user_id = a.id").
		Select("a.id", "a.email", `COALESCE(p."name", '') AS name`, "COALESCE(p.address, '') AS address", "COALESCE(p.phone, '') AS phone", "a.status", "a.version", "a.created_at", "a.must_change_password").
		Where("a.id = ? AND a.deleted_at IS NULL", id).Limit(1).Scan(&detail)
	if err != nil {
		return detail, helpers.HandleDBError(err)
	}
	if detail.ID != id {
		return domain.UserDetail{}, auth.ErrNotFound
	}
	if detail.Version < 1 || detail.CreatedAt.IsZero() || !domain.ManagementUserStatus(detail.Status) {
		return domain.UserDetail{}, common.ErrDependencyUnavailable
	}
	rows := []domain.UserSearchRow{{AuthID: int64(id)}}
	if err = readUserCollections(ctx, r.handle, rows); err != nil {
		return domain.UserDetail{}, err
	}
	detail.Roles = rows[0].Roles
	detail.ApplicationAccess = rows[0].ApplicationAccess
	return detail, nil
}
