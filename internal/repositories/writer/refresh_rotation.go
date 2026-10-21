package writer

import (
	"context"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type refreshConsumption struct {
	ConsumedAt time.Time `sql:"column:consumed_at"`
	ReplacedBy string    `sql:"column:replaced_by"`
}

func (refreshConsumption) TableName() string { return "refresh_tokens" }

func NewRefreshRotationWriter(tx domain.Transaction) domain.RefreshRotationWriter {
	return &initialRefreshWriter{tx: tx}
}

// No transaction lifecycle or locks belong here. Insert the successor using
// Create first; the immediate replaced_by FK is checked by PostgreSQL.
func (w *initialRefreshWriter) Consume(ctx context.Context, req domain.RefreshConsumption) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if w.tx == nil {
		return false, common.ErrInternalServerError
	}
	if err := req.Validate(); err != nil {
		return false, err
	}
	patch := refreshConsumption{ConsumedAt: req.At.UTC(), ReplacedBy: req.SuccessorID}
	result, err := w.tx.UseModel(&patch).WithContext(ctx).
		Where("id = ? AND consumed_at IS NULL AND revoked_at IS NULL AND replaced_by IS NULL AND expires_at > ?", req.ID, req.At.UTC()).Updates()
	if err != nil {
		return false, helpers.HandleDBError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count < 0 || count > 1 {
		return false, common.ErrInternalServerError
	}
	return count == 1, nil
}
