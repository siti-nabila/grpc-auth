package handler

import (
	"context"
	"errors"
	"time"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/authorization"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuthorizationHandler struct {
	authorizationv1.UnimplementedAuthorizationServiceServer
	snapshots domain.ReadSnapshotProvider
	now       func() time.Time
}

func NewAuthorizationHandler(snapshots domain.ReadSnapshotProvider, now func() time.Time) (*AuthorizationHandler, error) {
	if snapshots == nil || now == nil {
		return nil, errors.New("authorization requires snapshot provider and clock")
	}
	return &AuthorizationHandler{snapshots: snapshots, now: now}, nil
}

func (h *AuthorizationHandler) EvaluateAccess(ctx context.Context, in *authorizationv1.EvaluateAccessRequest) (*authorizationv1.EvaluateAccessResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	input := domain.EvaluateAccessInput{ApplicationCode: in.ApplicationCode, PermissionCode: in.PermissionCode}
	res, err := authorization.EvaluateAccessSnapshot(ctx, p.Claims(), input, h.snapshots, h.now())
	if err != nil {
		return nil, err
	}
	return &authorizationv1.EvaluateAccessResponse{Allowed: res.Allowed, Reason: authorizationv1.AccessDecisionReason(res.Reason), UserId: res.UserID, Sid: res.SessionID, UserVersion: uint64(res.Version), EvaluatedAt: timestamppb.New(res.EvaluatedAt)}, nil
}
