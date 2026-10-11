package handler

import (
	"context"
	"errors"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

type LogoutHandler struct {
	service  authfeature.LogoutService
	verifier *jwt.AccessVerifier
}

func NewLogoutHandler(service authfeature.LogoutService, verifier *jwt.AccessVerifier) (*LogoutHandler, error) {
	if service == nil || verifier == nil {
		return nil, errors.New("Logout handler requires service and verifier")
	}
	return &LogoutHandler{service: service, verifier: verifier}, nil
}
func (h *LogoutHandler) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	if req == nil {
		return nil, common.ErrBadRequest
	}
	proof, err := sessions.VerifyLogoutCredential(ctx, req.RefreshToken, h.verifier)
	if err != nil {
		return nil, err
	}
	r, err := h.service.Logout(ctx, proof)
	if err != nil {
		return nil, err
	}
	if r == nil || r.SessionID == "" {
		return nil, common.ErrInternalServerError
	}
	return &authv1.LogoutResponse{Changed: r.Changed}, nil
}
