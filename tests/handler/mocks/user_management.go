package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type UserManagement struct {
	Create func(context.Context, domain.CreateUserInput) (*domain.SafeCommandResult, error)
	Update func(context.Context, domain.UpdateUserInput) (*domain.SafeCommandResult, error)
	Status func(context.Context, domain.ChangeUserStatusInput) (*domain.SafeCommandResult, error)
}

func (m UserManagement) CreateUser(ctx context.Context, r domain.CreateUserInput) (*domain.SafeCommandResult, error) {
	return m.Create(ctx, r)
}
func (m UserManagement) UpdateUser(ctx context.Context, r domain.UpdateUserInput) (*domain.SafeCommandResult, error) {
	return m.Update(ctx, r)
}
func (m UserManagement) ChangeUserStatus(ctx context.Context, r domain.ChangeUserStatusInput) (*domain.SafeCommandResult, error) {
	return m.Status(ctx, r)
}
