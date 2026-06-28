package profile

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

type (
	ProfileService interface {
		UpdateProfile(req domain.UpdateProfileRequest) (err error)
	}

	profileService struct {
		ctx           context.Context
		profileWriter domain.ProfileWriter
		profileReader domain.ProfileReader
		// authReader   domain.AuthReader
	}
)

func NewProfileService(ctx context.Context) ProfileService {
	return &profileService{
		ctx:           ctx,
		profileWriter: writer.NewProfileWriter(ctx),
		profileReader: reader.NewProfileReader(ctx),
	}
}
