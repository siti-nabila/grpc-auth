package reader

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type userIdentityReader struct{ handle domain.QueryReader }

func NewUserIdentityReader(handle domain.QueryReader) *userIdentityReader {
	return &userIdentityReader{handle: handle}
}

func (r *userIdentityReader) Identity(ctx context.Context, userID uint64) (res domain.UserIdentity, err error) {
	if err = ctx.Err(); err != nil {
		return res, err
	}
	if r.handle == nil {
		return res, common.ErrDependencyUnavailable
	}
	err = r.handle.UseModel(domain.UserIdentity{}).WithContext(ctx).Select("id", "email", "status", "version", "must_change_password", "created_at").Where("id = ?", userID).Limit(1).Scan(&res)
	return res, helpers.HandleDBError(err)
}

func (r *userIdentityReader) ActiveRoles(ctx context.Context, userID uint64) (res []domain.Role, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	err = r.handle.UseModel(domain.UserRole{}).WithContext(ctx).Select("r.id", "r.role_name", "r.role_code", "r.role_description").
		Join("role r", "r.id = user_role.role_id").Where("user_role.user_id = ?", userID).
		Where("user_role.status = ?", types.AssignmentActive).Where("r.status = ?", types.RoleActive).OrderBy("r.id ASC").Scan(&res)
	return res, helpers.HandleDBError(err)
}
