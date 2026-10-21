package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
)

type Catalog struct {
	Permissions  func(context.Context, domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error)
	Applications func(context.Context, domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error)
}

func (m Catalog) ListPermissions(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.PermissionItem], error) {
	return m.Permissions(ctx, q)
}
func (m Catalog) ListApplications(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationItem], error) {
	return m.Applications(ctx, q)
}

type RoleRead struct {
	List func(context.Context, domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error)
	Get  func(context.Context, uint64) (domain.RoleDetail, error)
}

func (m RoleRead) ListRoles(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error) {
	return m.List(ctx, q)
}
func (m RoleRead) GetRole(ctx context.Context, id uint64) (domain.RoleDetail, error) {
	return m.Get(ctx, id)
}
