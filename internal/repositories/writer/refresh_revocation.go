package writer

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/query"
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
	q, err := w.refreshRevocationQuery(ctx, at)
	if err != nil {
		return 0, err
	}
	sessions := w.tx.UseModel(domain.SessionSummary{}).Select("id").Where("user_id = ?", userID)
	return refreshRevocationResult(q.WhereInQuery("session_id", sessions).Where("revoked_at IS NULL"))
}
func (w *initialRefreshWriter) revokeRefresh(ctx context.Context, at time.Time, condition string, args ...any) (int64, error) {
	q, err := w.refreshRevocationQuery(ctx, at)
	if err != nil {
		return 0, err
	}
	return refreshRevocationResult(q.Where(condition, args...).Where("revoked_at IS NULL"))
}
func (w *initialRefreshWriter) refreshRevocationQuery(ctx context.Context, at time.Time) (*query.QueryBuilder, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.tx == nil {
		return nil, common.ErrInternalServerError
	}
	patch := &refreshRevocation{RevokedAt: at.UTC()}
	return w.tx.UseModel(patch).WithContext(ctx), nil
}
func refreshRevocationResult(q *query.QueryBuilder) (int64, error) {
	result, err := q.Updates()
	if err != nil {
		return 0, helpers.HandleDBError(err)
	}
	if result == nil {
		return 0, common.ErrDependencyUnavailable
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
