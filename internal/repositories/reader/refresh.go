package reader

import (
	"context"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type refreshReader struct{ handle domain.QueryReader }

func NewRefreshReader(handle domain.QueryReader) domain.RefreshReader {
	return &refreshReader{handle: handle}
}

func (r *refreshReader) GetByDigest(ctx context.Context, digest []byte) (token domain.InitialRefresh, err error) {
	if err = ctx.Err(); err != nil {
		return token, err
	}
	if r.handle == nil {
		return token, common.ErrInternalServerError
	}
	if len(digest) != 32 {
		return token, common.ErrBadRequest
	}
	err = r.handle.UseModel(domain.InitialRefresh{}).WithContext(ctx).
		Select("id", "session_id", "token_hash", "family_id", "issued_at", "expires_at", "consumed_at", "revoked_at", "replaced_by").
		Where("token_hash = ?", append([]byte(nil), digest...)).Limit(1).Scan(&token)
	return token, helpers.HandleDBError(err)
}
