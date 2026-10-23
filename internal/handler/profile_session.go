package handler

import (
	"context"
	"errors"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

// SessionProfileHandler adapts the locked self-profile command.
type SessionProfileHandler struct{ service profile.SessionProfileService }

func NewSessionProfileHandler(service profile.SessionProfileService) (*SessionProfileHandler, error) {
	if service == nil {
		return nil, errors.New("session profile handler requires service")
	}
	return &SessionProfileHandler{service: service}, nil
}

func (h *SessionProfileHandler) UpdateProfile(ctx context.Context, input *profilev1.ProfileRequest) (*profilev1.ProfileResponse, error) {
	if input == nil {
		return nil, common.ErrBadRequest
	}
	req := domain.ProfileUpdate{Name: input.Name, Address: input.Address, Phone: input.Phone, CountryCode: input.CountryCode, MaskPresent: input.FieldMask != nil, Mask: append([]string(nil), input.GetFieldMask().GetPaths()...)}
	result, err := h.service.UpdateProfile(ctx, req)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Profile.Id == 0 || result.Profile.UserId == 0 || result.Version < 1 {
		return nil, common.ErrInternalServerError
	}
	return &profilev1.ProfileResponse{Profile: result.Profile.ToProfileResponse(), Version: uint64(result.Version), Changed: result.Changed}, nil
}
