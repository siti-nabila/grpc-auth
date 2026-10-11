package handler

import (
	"context"
	"errors"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math"
)

type ListSessionsHandler struct{ service session.ListService }

func NewListSessionsHandler(s session.ListService) (*ListSessionsHandler, error) {
	if s == nil {
		return nil, errors.New("session list handler requires service")
	}
	return &ListSessionsHandler{service: s}, nil
}
func (h *ListSessionsHandler) ListMySessions(ctx context.Context, r *sessionv1.ListMySessionsRequest) (*sessionv1.ListSessionsResponse, error) {
	if r == nil {
		return nil, common.ErrBadRequest
	}
	q, err := sessionListInput(r.Query)
	if err != nil {
		return nil, err
	}
	page, err := h.service.ListMySessions(ctx, q)
	if err != nil {
		return nil, err
	}
	return sessionListResponse(page)
}
func (h *ListSessionsHandler) ListUserSessions(ctx context.Context, r *sessionv1.ListUserSessionsRequest) (*sessionv1.ListSessionsResponse, error) {
	if r == nil {
		return nil, common.ErrBadRequest
	}
	q, err := sessionListInput(r.Query)
	if err != nil {
		return nil, err
	}
	page, err := h.service.ListUserSessions(ctx, r.UserId, q)
	if err != nil {
		return nil, err
	}
	return sessionListResponse(page)
}
func sessionListInput(q *paginatorv1.PageQuery) (session.ListInput, error) {
	if q == nil {
		return session.ListInput{}, nil
	}
	if len(q.Fields) > 0 || q.Search != nil || len(q.Sort) > 2 {
		return session.ListInput{}, common.ErrBadRequest
	}
	r := session.ListInput{Page: int(q.Page), Limit: int(q.Limit), Cursor: q.LastId}
	if len(q.Sort) > 0 {
		if q.Sort[0] == nil || q.Sort[0].Field == "" {
			return r, common.ErrBadRequest
		}
		r.Field = domain.SessionSortField(q.Sort[0].Field)
		r.Desc = q.Sort[0].Desc
		if len(q.Sort) == 2 && (q.Sort[1] == nil || q.Sort[1].Field != "id" || q.Sort[1].Desc != r.Desc) {
			return r, common.ErrBadRequest
		}
	}
	return r, nil
}
func sessionListResponse(r *session.ListResult) (*sessionv1.ListSessionsResponse, error) {
	if r == nil || r.Limit < 1 || r.Page < 1 || r.Total < 0 {
		return nil, common.ErrInternalServerError
	}
	pages := r.Total / int64(r.Limit)
	if r.Total%int64(r.Limit) > 0 {
		pages++
	}
	if pages > math.MaxInt32 {
		pages = math.MaxInt32
	}
	res := &sessionv1.ListSessionsResponse{Items: make([]*sessionv1.Session, 0, len(r.Items)), NextCursor: r.NextCursor, Meta: &paginatorv1.PageMeta{Page: int32(r.Page), Limit: int32(r.Limit), Total: r.Total, TotalPages: int32(pages), HasNext: r.HasNext, HasPrev: r.HasPrev}}
	for _, item := range r.Items {
		s := item.Session
		row := &sessionv1.Session{Id: s.ID, UserId: s.UserID, Status: string(s.Status), CreatedAt: timestamppb.New(s.CreatedAt), ExpiresAt: timestamppb.New(s.ExpiresAt), IsCurrent: item.Current}
		if s.DeviceLabel.Valid {
			row.DeviceLabel = s.DeviceLabel.String
		}
		if s.UserAgent.Valid {
			row.UserAgent = s.UserAgent.String
		}
		if s.IPAddress.Valid {
			row.IpAddress = s.IPAddress.String
		}
		if s.LastSeenAt.Valid {
			row.LastSeenAt = timestamppb.New(s.LastSeenAt.Time)
		}
		if s.RevokedAt.Valid {
			row.RevokedAt = timestamppb.New(s.RevokedAt.Time)
		}
		res.Items = append(res.Items, row)
	}
	return res, nil
}
