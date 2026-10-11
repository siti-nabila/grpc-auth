package reader

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/orm/query"
)

type sessionLifecycleReader struct{ handle domain.QueryReader }

func NewSessionLifecycleReader(handle domain.QueryReader) domain.SessionLifecycleReader {
	return &sessionLifecycleReader{handle: handle}
}
func (r *sessionLifecycleReader) check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.handle == nil {
		return common.ErrInternalServerError
	}
	return nil
}
func (r *sessionLifecycleReader) safeQuery(ctx context.Context) *query.QueryBuilder {
	return r.handle.UseModel(domain.SessionSummary{}).WithContext(ctx).Select("id", "user_id", "status", "created_at", "last_seen_at", "expires_at", "revoked_at", "device_label", "user_agent", "ip_address")
}
func (r *sessionLifecycleReader) GetByID(ctx context.Context, id string) (row domain.SessionSummary, err error) {
	if err = r.check(ctx); err != nil {
		return row, err
	}
	if err = domain.ValidateSessionID(id); err != nil {
		return row, err
	}
	err = r.safeQuery(ctx).Where("id = ?", id).Limit(1).Scan(&row)
	return row, helpers.HandleDBError(err)
}
func (r *sessionLifecycleReader) ListIDs(ctx context.Context, userID uint64) ([]string, error) {
	if err := r.check(ctx); err != nil {
		return nil, err
	}
	if userID == 0 || userID > 2147483647 {
		return nil, common.ErrBadRequest
	}
	var rows []domain.SessionSummary
	err := r.handle.UseModel(domain.SessionSummary{}).WithContext(ctx).Select("id").Where("user_id = ?", userID).OrderBy("id ASC").Scan(&rows)
	if err != nil {
		return nil, helpers.HandleDBError(err)
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids, nil
}
func (r *sessionLifecycleReader) ListPage(ctx context.Context, q domain.SessionListQuery) (page domain.SessionListPage, err error) {
	if err = r.check(ctx); err != nil {
		return page, err
	}
	if err = q.Validate(); err != nil {
		return page, err
	}
	var count struct {
		Total int64 `sql:"column:total"`
	}
	err = r.handle.UseModel(domain.SessionSummary{}).WithContext(ctx).Select("COUNT(*) AS total").Where("user_id = ?", q.UserID).Scan(&count)
	if err != nil {
		return page, helpers.HandleDBError(err)
	}
	direction, op := "ASC", ">"
	if q.Desc {
		direction, op = "DESC", "<"
	}
	// These three field names are exhaustively validated by the domain contract.
	field := string(q.Field)
	query := r.safeQuery(ctx).Where("user_id = ?", q.UserID)
	if q.After != nil {
		if !q.After.At.Valid {
			query = query.Where(field+" IS NULL AND id "+op+" ?", q.After.ID)
		} else {
			query = query.Where("("+field+" "+op+" ? OR ("+field+" = ? AND id "+op+" ?) OR "+field+" IS NULL)", q.After.At.Time.UTC(), q.After.At.Time.UTC(), q.After.ID)
		}
	}
	rows := make([]domain.SessionSummary, 0, q.Limit+1)
	err = query.OrderBy(field+" "+direction+" NULLS LAST", "id "+direction).Limit(q.Limit + 1).Scan(&rows)
	if err != nil {
		return page, helpers.HandleDBError(err)
	}
	page.Total = count.Total
	page.HasNext = len(rows) > q.Limit
	if page.HasNext {
		rows = rows[:q.Limit]
	}
	page.Items = rows
	return page, nil
}
