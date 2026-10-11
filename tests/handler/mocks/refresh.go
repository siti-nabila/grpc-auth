package mocks

import (
	"context"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
)

type Refresh struct {
	Call func(context.Context, string) (*authfeature.LoginResult, error)
}

func (s Refresh) Refresh(ctx context.Context, raw string) (*authfeature.LoginResult, error) {
	return s.Call(ctx, raw)
}
