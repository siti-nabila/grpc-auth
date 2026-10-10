package handler

import (
	"context"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	profilefeat "github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type (
	ProfileHandler struct {
		profilev1.UnimplementedProfileServiceServer
		NewProfileService func(context.Context) profilefeat.ProfileService
	}
)

func (p *ProfileHandler) UpdateProfile(ctx context.Context, in *profilev1.ProfileRequest) (*profilev1.ProfileResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	newService := p.NewProfileService
	if newService == nil {
		newService = profilefeat.NewProfileService
	}
	feat := newService(ctx)

	request := domain.UpdateProfileRequest{
		Name:    in.Name,
		Address: in.Address,
		Phone:   in.Phone,
	}
	result, err := feat.UpdateProfile(request)
	if err != nil {
		return nil, err
	}

	return &profilev1.ProfileResponse{Profile: result.ToProfileResponse()}, nil
}
