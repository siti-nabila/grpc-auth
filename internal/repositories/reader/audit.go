package reader

import (
	"context"
	"math"
	"strconv"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/orm"
	"github.com/siti-nabila/orm/query"
)

type auditReader struct{ handle domain.QueryReader }

func NewAuditReader(handle domain.QueryReader) domain.AuditLogReader { return &auditReader{handle} }

func (r *auditReader) base(ctx context.Context, q domain.AuditLogQuery) *query.QueryBuilder {
	b := r.handle.UseModel(domain.AuditRecord{}).WithContext(ctx)
	if q.ActorUserID != nil {
		b = b.Where("actor_user_id = ?", *q.ActorUserID)
	}
	for _, f := range []struct{ column, value string }{{"actor_service", q.ActorService}, {"action", q.Action}, {"target_type", q.TargetType}, {"target_id", q.TargetID}} {
		if f.value != "" {
			b = b.Where(f.column+" = ?", f.value)
		}
	}
	if q.OccurredFrom != nil {
		b = b.Where("occurred_at >= ?", auditBound(*q.OccurredFrom))
	}
	if q.OccurredBefore != nil {
		b = b.Where("occurred_at < ?", auditBound(*q.OccurredBefore))
	}
	return b
}

// ceil to a stored microsecond preserves both >= and < comparisons for
// nanosecond input bounds instead of letting PostgreSQL round them to nearest.
func auditBound(t time.Time) time.Time {
	b := t.UTC().Truncate(time.Microsecond)
	if t.After(b) {
		return b.Add(time.Microsecond)
	}
	return b
}

func (r *auditReader) List(ctx context.Context, input domain.AuditLogQuery) (page orm.PageData[domain.AuditLogItem], err error) {
	if err = ctx.Err(); err != nil {
		return page, err
	}
	if r.handle == nil {
		return page, common.ErrDependencyUnavailable
	}
	q, err := input.Normalized()
	if err != nil {
		return page, err
	}
	var count struct {
		Total int64 `sql:"column:total"`
	}
	if err = r.base(ctx, q).Select("COUNT(*) AS total").Scan(&count); err != nil {
		return page, helpers.HandleDBError(err)
	}
	if count.Total < 0 {
		return page, common.ErrDependencyUnavailable
	}
	pages := count.Total / int64(q.Limit)
	if count.Total%int64(q.Limit) != 0 {
		pages++
	}
	if pages > math.MaxInt32 {
		return page, common.ErrDependencyUnavailable
	}
	c, err := q.Cursor()
	if err != nil {
		return page, err
	}
	b := r.base(ctx, q).Select("id", "actor_user_id", "actor_service", "action", "target_type", "target_id", "safe_before", "safe_after", "reason", "request_id", "trace_id", "occurred_at")
	if c != nil {
		at, _ := time.Parse(time.RFC3339Nano, c.OccurredAt)
		id, _ := strconv.ParseInt(c.ID, 10, 64)
		b = b.Where("(occurred_at < ? OR (occurred_at = ? AND id < ?))", at, at, id)
	} else {
		b = b.Offset((q.Page - 1) * q.Limit)
	}
	var rows []domain.AuditRecord
	if err = b.OrderBy("occurred_at DESC").OrderBy("id DESC").Limit(q.Limit + 1).Scan(&rows); err != nil {
		return page, helpers.HandleDBError(err)
	}
	items := make([]domain.AuditLogItem, 0, len(rows))
	for _, row := range rows {
		item, e := row.SafeItem()
		if e != nil {
			return page, e
		}
		items = append(items, item)
	}
	page.HasNext = len(items) > q.Limit
	if page.HasNext {
		items = items[:q.Limit]
		page.NextCursor = q.NextCursor(items[len(items)-1])
	}
	page.Items = items
	page.Total = count.Total
	page.TotalPages = int(pages)
	page.Page = q.Page
	page.Limit = q.Limit
	page.HasPrev = c != nil || q.Page > 1
	return page, nil
}
