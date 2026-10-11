package writer

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"time"
)

type refreshRevocation struct {
	RevokedAt time.Time `sql:"column:revoked_at"`
}

func (refreshRevocation) TableName() string { return "refresh_tokens" }
func NewRefreshRevocationWriter(tx domain.Transaction) domain.RefreshRevocationWriter {
	return &initialRefreshWriter{tx: tx}
}
func (w *initialRefreshWriter) RevokeFamily(ctx context.Context, req domain.RefreshFamilyRevocation) (int64, error) {
	if err := req.Validate(); err != nil {
		return 0, err
	}
	return w.revokeRefresh(ctx, req.At, "session_id = ? AND family_id = ?", req.SessionID, req.FamilyID)
}
func (w *initialRefreshWriter) RevokeUserRefresh(ctx context.Context, userID uint64, at time.Time) (int64, error) {
	if userID == 0 || userID > 2147483647 || at.IsZero() {
		return 0, common.ErrBadRequest
	}
	return w.revokeRefresh(ctx, at, "session_id IN (SELECT id FROM sessions WHERE user_id = ?)", userID)
}
func (w *initialRefreshWriter) revokeRefresh(ctx context.Context, at time.Time, condition string, args ...any) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if w.tx == nil {
		return 0, common.ErrInternalServerError
	}
	patch := refreshRevocation{RevokedAt: at.UTC()}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).Where(condition, args...).Where("revoked_at IS NULL").Updates()
	if err != nil {
		return 0, helpers.HandleDBError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, common.ErrInternalServerError
	}
	return count, nil
}

func (w *initialRefreshWriter) RevokeSessionRefresh(ctx context.Context, id string, at time.Time) (int64, error) {
	if err := domain.ValidateSessionID(id); err != nil {
		return 0, err
	}
	if at.IsZero() {
		return 0, common.ErrBadRequest
	}
	return w.revokeRefresh(ctx, at, "session_id = ?", id)
}
