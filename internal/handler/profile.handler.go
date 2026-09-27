package handler

import (
	"context"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	profilefeat "github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type (
	ProfileHandler struct {
		profilev1.UnimplementedProfileServiceServer
	}
)

func (p *ProfileHandler) UpdateProfile(ctx context.Context, in *profilev1.ProfileRequest) (*profilev1.ProfileResponse, error) {
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

	return &profilev1.ProfileResponse{
		Profile: &profilev1.Profile{
			Id:      request.Id,
			UserId:  0,
			Name:    request.Name,
			Address: request.Address,
			Phone:   request.Phone,
		},
	}, nil
}
