package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type CredentialInvalidation struct {
	Password func(context.Context, domain.PasswordChange) (bool, error)
}

func (w CredentialInvalidation) ChangePassword(ctx context.Context, c domain.PasswordChange) (bool, error) {
	return w.Password(ctx, c)
}
func (CredentialInvalidation) Invalidate(context.Context, domain.CredentialInvalidation) (bool, error) {
	panic("unexpected generic invalidation")
}
