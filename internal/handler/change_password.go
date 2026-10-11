package handler

import (
	"context"
	"errors"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type ChangePasswordHandler struct {
	service authfeature.ChangePasswordService
}

func NewChangePasswordHandler(s authfeature.ChangePasswordService) (*ChangePasswordHandler, error) {
	if s == nil {
		return nil, errors.New("ChangePassword handler requires service")
	}
	return &ChangePasswordHandler{service: s}, nil
}
func (h *ChangePasswordHandler) ChangePassword(ctx context.Context, req *authv1.ChangePasswordRequest) (*authv1.ChangePasswordResponse, error) {
	if req == nil {
		return nil, common.ErrBadRequest
	}
	r, err := h.service.ChangePassword(ctx, domain.ChangePasswordInput{CurrentPassword: req.CurrentPassword, NewPassword: req.NewPassword})
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, common.ErrInternalServerError
	}
	return &authv1.ChangePasswordResponse{Changed: r.Changed, LoginRequired: r.LoginRequired}, nil
}
