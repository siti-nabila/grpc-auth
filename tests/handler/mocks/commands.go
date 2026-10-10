package mocks

import (
	"context"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type PendingRegistration struct {
	Call func(context.Context, domain.AuthRequest) (*authfeature.RegistrationResult, error)
}

func (s PendingRegistration) Register(ctx context.Context, req domain.AuthRequest) (*authfeature.RegistrationResult, error) {
	return s.Call(ctx, req)
}

type SessionProfile struct {
	Call func(context.Context, domain.ProfileUpdate) (*profile.UpdateResult, error)
}

func (s SessionProfile) UpdateProfile(ctx context.Context, req domain.ProfileUpdate) (*profile.UpdateResult, error) {
	return s.Call(ctx, req)
}
