package reader

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// selfAccessReader only queries the supplied snapshot; the feature owns its lifecycle.
type selfAccessReader struct{ handle domain.QueryReader }

func NewSelfAccessReader(handle domain.QueryReader) *selfAccessReader {
	return &selfAccessReader{handle: handle}
}

func (r *selfAccessReader) Roles(ctx context.Context, userID uint64) (rows []domain.SelfRole, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	err = r.handle.UseModel(domain.SelfRole{}).WithContext(ctx).
		Select("DISTINCT r.id AS role_id", "r.code", "r.role_name AS name", "ur.status", "r.status AS role_status", "a.code AS application_code").
		Join("role r", "r.id = ur.role_id").Join("applications a", "a.id = r.application_id").
		LeftJoin("user_application_access access", "access.user_id = ur.user_id AND access.application_id = a.id").
		Where("ur.user_id = ?", userID).
		Where("ur.status = ? AND r.status = ? AND a.status = ?", types.AssignmentActive, types.RoleActive, types.ApplicationActive).
		Where("(a.code = ? OR access.status = ?)", types.ApplicationIAM, types.ApplicationAccessActive).
		OrderBy("r.id ASC").Scan(&rows)
	return rows, helpers.HandleDBError(err)
}

func (r *selfAccessReader) Applications(ctx context.Context, userID uint64) (rows []domain.SelfApplicationAccess, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if r.handle == nil {
		return nil, common.ErrDependencyUnavailable
	}
	err = r.handle.UseModel(domain.SelfApplicationAccess{}).WithContext(ctx).
		Select("DISTINCT a.id AS application_id", "a.code", "a.name", "access.status", "a.status AS application_status").
		Join("applications a", "a.id = access.application_id").Where("access.user_id = ?", userID).
		Where("access.status = ? AND a.status = ?", types.ApplicationAccessActive, types.ApplicationActive).
		OrderBy("a.id ASC").Scan(&rows)
	return rows, helpers.HandleDBError(err)
}
