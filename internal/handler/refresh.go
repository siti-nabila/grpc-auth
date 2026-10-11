package handler

import (
	"context"
	"errors"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// RefreshHandler is composed into AuthService by the foundation registration.
type RefreshHandler struct{ service authfeature.RefreshService }

func NewRefreshHandler(service authfeature.RefreshService) (*RefreshHandler, error) {
	if service == nil {
		return nil, errors.New("refresh handler requires service")
	}
	return &RefreshHandler{service: service}, nil
}
func (h *RefreshHandler) RefreshSession(ctx context.Context, req *authv1.RefreshSessionRequest) (*userv1.UserTokenResponse, error) {
	if req == nil {
		return nil, common.ErrBadRequest
	}
	r, err := h.service.Refresh(ctx, req.RefreshToken)
	if err != nil {
		return nil, err
	}
	if r == nil || r.AccessToken == "" || r.RefreshToken == "" || r.SessionID == "" || r.AccessExpiresAt.IsZero() || r.SessionExpiresAt.IsZero() || r.MustChangePassword {
		return nil, common.ErrInternalServerError
	}
	return &userv1.UserTokenResponse{Token: r.AccessToken, RefreshToken: r.RefreshToken, SessionId: r.SessionID, AccessExpiresAt: timestamppb.New(r.AccessExpiresAt), SessionExpiresAt: timestamppb.New(r.SessionExpiresAt)}, nil
}
