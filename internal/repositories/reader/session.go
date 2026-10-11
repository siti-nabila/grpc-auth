package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type sessionStateReader struct{ handle domain.QueryReader }

// The caller supplies a primary adapter or its current feature-owned tx.
// Missing handles fail closed; this reader never falls back to another pool.
func NewSessionStateReader(handle domain.QueryReader) domain.SessionStateReader {
	return &sessionStateReader{handle: handle}
}

func (r *sessionStateReader) GetSessionState(ctx context.Context, id string) (state domain.SessionState, err error) {
	if err := ctx.Err(); err != nil {
		return state, err
	}
	if r.handle == nil {
		return state, common.ErrInternalServerError
	}
	err = r.handle.UseModel(domain.SessionState{}).WithContext(ctx).Select(
		"a.id AS user_id", "a.status AS user_status", "a.auth_version", "a.version AS user_version", "a.must_change_password", "a.deleted_at",
		"s.id AS session_id", "s.user_id AS session_user_id", "s.status AS session_status", "s.created_at", "s.expires_at", "s.revoked_at", "s.auth_version AS session_auth_version",
	).Join("auth a", "a.id = s.user_id").Where("s.id = ?", id).Limit(1).Scan(&state)
	return state, helpers.HandleDBError(err)
}
