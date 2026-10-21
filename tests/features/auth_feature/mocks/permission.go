package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type Permissions struct {
	Read func(context.Context, uint64) ([]domain.PermissionContribution, error)
}

func (r Permissions) Permissions(ctx context.Context, id uint64) ([]domain.PermissionContribution, error) {
	return r.Read(ctx, id)
}
