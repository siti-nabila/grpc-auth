package handler

import (
	"context"
	"errors"

	auditv1 "github.com/siti-nabila/api-contracts/pb/audit/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/audit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuditHandler struct {
	auditv1.UnimplementedAuditServiceServer
	service audit.Service
}

func NewAuditHandler(service audit.Service) (*AuditHandler, error) {
	if service == nil {
		return nil, errors.New("audit handler requires service")
	}
	return &AuditHandler{service: service}, nil
}
func (h *AuditHandler) ListAuditLogs(ctx context.Context, in *auditv1.ListAuditLogsRequest) (*auditv1.ListAuditLogsResponse, error) {
	if _, err := sessions.PrincipalFromContext(ctx); err != nil {
		return nil, err
	}
	q, err := auditQuery(in)
	if err != nil {
		return nil, err
	}
	page, err := h.service.ListAuditLogs(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]*auditv1.AuditLogItem, 0, len(page.Items))
	for _, r := range page.Items {
		items = append(items, &auditv1.AuditLogItem{Id: r.ID, ActorUserId: r.ActorUserID, ActorService: r.ActorService, Action: r.Action, TargetType: r.TargetType, TargetId: r.TargetID, SafeBefore: auditSnapshotResponse(r.Before), SafeAfter: auditSnapshotResponse(r.After), Reason: r.Reason, RequestId: r.RequestID, TraceId: r.TraceID, OccurredAt: timestamppb.New(r.OccurredAt.UTC())})
	}
	return &auditv1.ListAuditLogsResponse{Items: items, Meta: catalogMeta(page.Page, page.Limit, page.Total, page.TotalPages, page.HasNext, page.HasPrev), NextCursor: page.NextCursor}, nil
}
func auditQuery(in *auditv1.ListAuditLogsRequest) (domain.AuditLogQuery, error) {
	var q domain.AuditLogQuery
	if in == nil {
		return q, common.ErrBadRequest
	}
	if p := in.Query; p != nil {
		q.Page = int(p.Page)
		q.Limit = int(p.Limit)
		q.LastID = p.LastId
		q.Unsupported = len(p.Fields) > 0 || len(p.Sort) > 0 || p.Search != nil
	}
	if f := in.Filter; f != nil {
		q.ActorUserID = f.ActorUserId
		q.ActorService = f.ActorService
		q.Action = f.Action
		q.TargetType = f.TargetType
		q.TargetID = f.TargetId
		if f.OccurredFrom != nil {
			if f.OccurredFrom.CheckValid() != nil {
				return q, common.ErrBadRequest
			}
			at := f.OccurredFrom.AsTime()
			q.OccurredFrom = &at
		}
		if f.OccurredBefore != nil {
			if f.OccurredBefore.CheckValid() != nil {
				return q, common.ErrBadRequest
			}
			at := f.OccurredBefore.AsTime()
			q.OccurredBefore = &at
		}
	}
	return q.Normalized()
}
func auditSnapshotResponse(s *domain.SafeSnapshot) *auditv1.AuditSnapshot {
	if s == nil {
		return nil
	}
	return &auditv1.AuditSnapshot{Version: s.Version, Status: s.Status, Name: s.Name, Description: s.Description, Address: s.Address, Phone: s.Phone, RoleIds: append([]uint64(nil), s.RoleIDs...), PermissionCodes: append([]string(nil), s.PermissionCodes...), ApplicationId: s.ApplicationID, SessionId: s.SessionID, ReasonCode: s.ReasonCode, NoOp: s.NoOp}
}
