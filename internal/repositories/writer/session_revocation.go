package writer

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"time"
)

type sessionRevocation struct {
	Status       types.SessionStatus `sql:"column:status"`
	RevokedAt    time.Time           `sql:"column:revoked_at"`
	RevokedBy    uint64              `sql:"column:revoked_by"`
	RevokeReason types.AuditReason   `sql:"column:revoke_reason"`
}

func (sessionRevocation) TableName() string { return "sessions" }
func NewSessionRevocationWriter(tx domain.Transaction) domain.SessionRevocationWriter {
	return &sessionWriter{tx: tx}
}
func (w *sessionWriter) RevokeSession(ctx context.Context, req domain.SessionRevocation) (bool, error) {
	if req.SessionID == "" {
		return false, common.ErrBadRequest
	}
	n, err := w.revoke(ctx, req)
	if err != nil {
		return false, err
	}
	if n > 1 {
		return false, common.ErrInternalServerError
	}
	return n == 1, nil
}
func (w *sessionWriter) RevokeUserSessions(ctx context.Context, req domain.SessionRevocation) (int64, error) {
	if req.SessionID != "" {
		return 0, common.ErrBadRequest
	}
	return w.revoke(ctx, req)
}
func (w *sessionWriter) revoke(ctx context.Context, req domain.SessionRevocation) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if w.tx == nil {
		return 0, common.ErrInternalServerError
	}
	if err := req.Validate(); err != nil {
		return 0, err
	}
	patch := sessionRevocation{types.SessionRevoked, req.At.UTC(), req.ActorID, req.Reason}
	query := w.tx.UseModel(&patch).WithContext(ctx).Where("user_id = ? AND status = ?", req.UserID, types.SessionActive)
	if req.SessionID != "" {
		query = query.Where("id = ?", req.SessionID)
	}
	result, err := query.Updates()
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
