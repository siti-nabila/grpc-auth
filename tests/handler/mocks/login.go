package mocks

import (
	"context"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type SessionLogin struct {
	Call func(context.Context, domain.AuthRequest) (*authfeature.LoginResult, error)
}

func (s SessionLogin) Login(ctx context.Context, r domain.AuthRequest) (*authfeature.LoginResult, error) {
	return s.Call(ctx, r)
}
