package handler

import (
	"context"
	"errors"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"google.golang.org/grpc/metadata"
	"math"
)

const idempotencyMetadataKey = "idempotency-key"

type AdminSessionsHandler struct{ service session.AdminService }

func NewAdminSessionsHandler(s session.AdminService) (*AdminSessionsHandler, error) {
	if s == nil {
		return nil, errors.New("admin sessions handler requires service")
	}
	return &AdminSessionsHandler{service: s}, nil
}
func adminIdempotencyKey(ctx context.Context) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	keys := md.Get(idempotencyMetadataKey)
	if len(keys) != 1 || keys[0] == "" {
		return "", common.ErrBadRequest
	}
	return keys[0], nil
}
func (h *AdminSessionsHandler) RevokeUserSession(ctx context.Context, r *sessionv1.RevokeUserSessionRequest) (*sessionv1.RevokeSessionResponse, error) {
	if r == nil {
		return nil, common.ErrBadRequest
	}
	key, e := adminIdempotencyKey(ctx)
	if e != nil {
		return nil, e
	}
	res, e := h.service.RevokeUserSession(ctx, session.AdminSingleInput{UserID: r.UserId, SessionID: r.SessionId, Key: key})
	if e != nil {
		return nil, e
	}
	if res == nil {
		return nil, common.ErrInternalServerError
	}
	return &sessionv1.RevokeSessionResponse{Changed: res.Changed}, nil
}
func (h *AdminSessionsHandler) RevokeAllUserSessions(ctx context.Context, r *sessionv1.RevokeAllUserSessionsRequest) (*sessionv1.RevokeAllUserSessionsResponse, error) {
	if r == nil || r.ExpectedUserVersion == nil || *r.ExpectedUserVersion == 0 || *r.ExpectedUserVersion > math.MaxInt64 {
		return nil, common.ErrBadRequest
	}
	key, e := adminIdempotencyKey(ctx)
	if e != nil {
		return nil, e
	}
	res, e := h.service.RevokeAllUserSessions(ctx, session.AdminGlobalInput{UserID: r.UserId, ExpectedVersion: int64(*r.ExpectedUserVersion), Key: key})
	if e != nil {
		return nil, e
	}
	if res == nil || res.Version < 1 {
		return nil, common.ErrInternalServerError
	}
	return &sessionv1.RevokeAllUserSessionsResponse{Changed: res.Changed, Version: uint64(res.Version)}, nil
}
