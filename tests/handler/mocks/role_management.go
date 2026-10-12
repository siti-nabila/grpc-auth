package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type RoleManagement struct {
	Create  func(context.Context, domain.CreateRoleInput) (*domain.SafeCommandResult, error)
	Update  func(context.Context, domain.UpdateRoleInput) (*domain.SafeCommandResult, error)
	Status  func(context.Context, domain.ChangeRoleStatusInput) (*domain.SafeCommandResult, error)
	Replace func(context.Context, domain.ReplaceRolePermissionsInput) (*domain.SafeCommandResult, error)
}

func (m RoleManagement) CreateRole(c context.Context, i domain.CreateRoleInput) (*domain.SafeCommandResult, error) {
	return m.Create(c, i)
}
func (m RoleManagement) UpdateRole(c context.Context, i domain.UpdateRoleInput) (*domain.SafeCommandResult, error) {
	return m.Update(c, i)
}
func (m RoleManagement) ChangeRoleStatus(c context.Context, i domain.ChangeRoleStatusInput) (*domain.SafeCommandResult, error) {
	return m.Status(c, i)
}
func (m RoleManagement) ReplaceRolePermissions(c context.Context, i domain.ReplaceRolePermissionsInput) (*domain.SafeCommandResult, error) {
	return m.Replace(c, i)
}
