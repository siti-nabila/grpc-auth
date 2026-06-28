package handler

import (
	"context"

	profilefeat "github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pb/profile"
)

type (
	ProfileHandler struct {
		profile.UnimplementedProfileServiceServer
	}
)

func (p *ProfileHandler) UpdateProfile(ctx context.Context, in *profile.ProfileRequest) (*profile.ProfileResponse, error) {
	var (
		feat = profilefeat.NewProfileService(ctx)
	)

	request := domain.UpdateProfileRequest{
		Name:    in.Name,
		Address: in.Address,
		Phone:   in.Phone,
	}
	err := feat.UpdateProfile(request)
	if err != nil {
		return nil, err
	}

	return &profile.ProfileResponse{
		Profile: &profile.Profile{
			Id:      request.Id,
			UserId:  0,
			Name:    request.Name,
			Address: request.Address,
			Phone:   request.Phone,
		},
	}, nil
}
