package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type outboxMetricsReader struct{ handle domain.QueryReader }

func NewOutboxMetricsReader(handle domain.QueryReader) domain.OutboxMetricsReader {
	return &outboxMetricsReader{handle}
}

func (r *outboxMetricsReader) Pending(ctx context.Context) (domain.PendingOutbox, error) {
	var v domain.PendingOutbox
	if err := ctx.Err(); err != nil {
		return v, err
	}
	if r.handle == nil {
		return v, common.ErrDependencyUnavailable
	}
	err := r.handle.UseModel(domain.OutboxRecord{}).WithContext(ctx).Select("COUNT(*) AS pending_count", "MIN(created_at) AS oldest_created_at").Where("published_at IS NULL").Scan(&v)
	if err != nil {
		return domain.PendingOutbox{}, helpers.HandleDBError(err)
	}
	if v.Count < 0 || v.Count == 0 && v.Oldest.Valid || v.Count > 0 && !v.Oldest.Valid {
		return domain.PendingOutbox{}, common.ErrDependencyUnavailable
	}
	return v, nil
}
