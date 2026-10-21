package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type Assignment struct {
	List   func(context.Context, domain.CatalogQuery) (domain.UserRoleAssignmentPage, error)
	Grant  func(context.Context, domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error)
	Revoke func(context.Context, domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error)
}

func (m Assignment) ListUserRoles(c context.Context, q domain.CatalogQuery) (domain.UserRoleAssignmentPage, error) {
	return m.List(c, q)
}
func (m Assignment) GrantUserRoles(c context.Context, r domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error) {
	return m.Grant(c, r)
}
func (m Assignment) RevokeUserRoles(c context.Context, r domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error) {
	return m.Revoke(c, r)
}
