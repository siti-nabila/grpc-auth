package mocks

import (
	"context"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
)

type Logout struct {
	Call func(context.Context, sessions.LogoutCredential) (*authfeature.LogoutResult, error)
}

func (s Logout) Logout(ctx context.Context, proof sessions.LogoutCredential) (*authfeature.LogoutResult, error) {
	return s.Call(ctx, proof)
}
