package mocks

import (
	"context"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type ChangePassword struct {
	Call func(context.Context, domain.ChangePasswordInput) (*authfeature.ChangePasswordResult, error)
}

func (s ChangePassword) ChangePassword(ctx context.Context, r domain.ChangePasswordInput) (*authfeature.ChangePasswordResult, error) {
	return s.Call(ctx, r)
}
