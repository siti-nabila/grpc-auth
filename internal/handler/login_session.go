package handler

import (
	"context"
	"errors"
	"strings"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SessionLoginHandler is an adapter component, not an independently registered
// UserService. The default foundation registration composes all consumers.
type SessionLoginHandler struct {
	service authfeature.SessionLoginService
}

func NewSessionLoginHandler(service authfeature.SessionLoginService) (*SessionLoginHandler, error) {
	if service == nil {
		return nil, errors.New("Login handler requires session producer")
	}
	return &SessionLoginHandler{service: service}, nil
}

func (h *SessionLoginHandler) Login(ctx context.Context, input *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	if input == nil {
		return nil, common.ErrBadRequest
	}
	req := domain.AuthRequest{Email: strings.ToLower(strings.TrimSpace(input.Email)), Password: input.Password}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	res, err := h.service.Login(ctx, req)
	if err != nil {
		return nil, err
	}
	if res == nil || res.AccessToken == "" || res.RefreshToken == "" || res.SessionID == "" || res.AccessExpiresAt.IsZero() || res.SessionExpiresAt.IsZero() {
		return nil, common.ErrInternalServerError
	}
	return &userv1.UserTokenResponse{Token: res.AccessToken, RefreshToken: res.RefreshToken, SessionId: res.SessionID, AccessExpiresAt: timestamppb.New(res.AccessExpiresAt), SessionExpiresAt: timestamppb.New(res.SessionExpiresAt), MustChangePassword: res.MustChangePassword}, nil
}
