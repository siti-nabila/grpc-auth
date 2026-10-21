package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type Application struct {
	List   func(context.Context, domain.CatalogQuery) (domain.ApplicationAccessPage, error)
	Grant  func(context.Context, domain.ApplicationAccessInput) (*domain.SafeCommandResult, error)
	Revoke func(context.Context, domain.ApplicationAccessInput) (*domain.SafeCommandResult, error)
}

func (m Application) ListUserApplicationAccess(c context.Context, q domain.CatalogQuery) (domain.ApplicationAccessPage, error) {
	return m.List(c, q)
}
func (m Application) GrantApplicationAccess(c context.Context, r domain.ApplicationAccessInput) (*domain.SafeCommandResult, error) {
	return m.Grant(c, r)
}
func (m Application) RevokeApplicationAccess(c context.Context, r domain.ApplicationAccessInput) (*domain.SafeCommandResult, error) {
	return m.Revoke(c, r)
}
