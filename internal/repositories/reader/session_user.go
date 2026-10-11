package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type sessionUserReader struct{ handle domain.QueryReader }

func NewSessionUserReader(handle domain.QueryReader) domain.SessionUserReader {
	return &sessionUserReader{handle: handle}
}
func (r *sessionUserReader) GetByID(ctx context.Context, id uint64) (row domain.SessionUserState, err error) {
	if err = ctx.Err(); err != nil {
		return row, err
	}
	if id == 0 || id > 2147483647 {
		return row, common.ErrBadRequest
	}
	if r.handle == nil {
		return row, common.ErrInternalServerError
	}
	err = r.handle.UseModel(domain.SessionUserState{}).WithContext(ctx).Select("id", "status", "version", "auth_version", "deleted_at").Where("id = ?", id).Limit(1).Scan(&row)
	return row, helpers.HandleDBError(err)
}
