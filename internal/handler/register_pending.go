package handler

import (
	"context"
	"errors"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type PendingRegistrationHandler struct {
	service authfeature.PendingRegistrationService
}

func NewPendingRegistrationHandler(service authfeature.PendingRegistrationService) (*PendingRegistrationHandler, error) {
	if service == nil {
		return nil, errors.New("pending registration handler requires service")
	}
	return &PendingRegistrationHandler{service: service}, nil
}
func (h *PendingRegistrationHandler) Register(ctx context.Context, input *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	if input == nil {
		return nil, common.ErrBadRequest
	}
	result, err := h.service.Register(ctx, domain.AuthRequest{Email: input.Email, Password: input.Password})
	if err != nil {
		return nil, err
	}
	if result == nil || result.ID == 0 || result.Status != types.UserPending {
		return nil, common.ErrInternalServerError
	}
	return &userv1.UserTokenResponse{RegistrationId: result.ID, RegistrationStatus: string(result.Status)}, nil
}
