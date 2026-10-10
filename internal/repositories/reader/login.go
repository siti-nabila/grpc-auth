package reader

import (
	"context"
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type loginAccountReader struct{ handle domain.QueryReader }

// The feature supplies either its primary candidate adapter or locked tx.
func NewLoginAccountReader(handle domain.QueryReader) domain.LoginAccountReader {
	return &loginAccountReader{handle: handle}
}

func (r *loginAccountReader) GetByEmail(ctx context.Context, email string) (domain.LoginAccount, error) {
	return r.read(ctx, "email = ?", strings.ToLower(strings.TrimSpace(email)))
}
func (r *loginAccountReader) GetByID(ctx context.Context, id uint64) (domain.LoginAccount, error) {
	return r.read(ctx, "id = ?", id)
}
func (r *loginAccountReader) read(ctx context.Context, where string, value any) (account domain.LoginAccount, err error) {
	if err = ctx.Err(); err != nil {
		return account, err
	}
	if r.handle == nil {
		return account, common.ErrInternalServerError
	}
	err = r.handle.UseModel(domain.LoginAccount{}).WithContext(ctx).Select("id", "email", "password", "status", "auth_version", "version", "must_change_password", "deleted_at").Where(where, value).Limit(1).Scan(&account)
	return account, helpers.HandleDBError(err)
}
