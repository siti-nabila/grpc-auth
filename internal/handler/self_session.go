package handler

import (
	"context"
	"errors"

	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"google.golang.org/protobuf/types/known/emptypb"
)

type SelfSessionHandler struct{ service session.SelfService }

func NewSelfSessionHandler(service session.SelfService) (*SelfSessionHandler, error) {
	if service == nil {
		return nil, errors.New("self session handler requires service")
	}
	return &SelfSessionHandler{service: service}, nil
}

func (h *SelfSessionHandler) RevokeMySession(ctx context.Context, req *sessionv1.RevokeMySessionRequest) (*sessionv1.RevokeSessionResponse, error) {
	if req == nil {
		return nil, common.ErrBadRequest
	}
	r, err := h.service.RevokeMySession(ctx, req.SessionId)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, common.ErrInternalServerError
	}
	return &sessionv1.RevokeSessionResponse{Changed: r.Changed}, nil
}

func (h *SelfSessionHandler) RevokeAllMySessions(ctx context.Context, req *emptypb.Empty) (*sessionv1.RevokeAllMySessionsResponse, error) {
	if req == nil {
		return nil, common.ErrBadRequest
	}
	r, err := h.service.RevokeAllMySessions(ctx)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, common.ErrInternalServerError
	}
	return &sessionv1.RevokeAllMySessionsResponse{Changed: r.Changed, LoginRequired: r.LoginRequired}, nil
}
