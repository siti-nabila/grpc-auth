package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type Provider struct {
	BeginTransaction func(context.Context) (domain.Transaction, error)
}

func (p Provider) Begin(ctx context.Context) (domain.Transaction, error) {
	return p.BeginTransaction(ctx)
}

type Roles struct {
	Read        func(context.Context, uint64) (domain.RoleSummary, error)
	App         func(context.Context, string) (domain.ApplicationItem, error)
	Permissions func(context.Context, uint64) ([]domain.PermissionItem, error)
	Set         func(context.Context, uint64, []string) ([]domain.PermissionItem, error)
}

func (r Roles) Role(ctx context.Context, id uint64) (domain.RoleSummary, error) {
	return r.Read(ctx, id)
}
func (r Roles) Application(ctx context.Context, code string) (domain.ApplicationItem, error) {
	return r.App(ctx, code)
}
func (r Roles) RolePermissions(ctx context.Context, id uint64) ([]domain.PermissionItem, error) {
	return r.Permissions(ctx, id)
}
func (r Roles) PermissionSet(ctx context.Context, id uint64, codes []string) ([]domain.PermissionItem, error) {
	return r.Set(ctx, id, codes)
}

type Writer struct {
	Create  func(context.Context, domain.RoleSummary) (domain.RoleSummary, error)
	Update  func(context.Context, domain.RoleSummary, int64) (bool, error)
	Replace func(context.Context, uint64, []uint64) error
}

func (w Writer) CreateRole(ctx context.Context, r domain.RoleSummary) (domain.RoleSummary, error) {
	return w.Create(ctx, r)
}
func (w Writer) UpdateRole(ctx context.Context, r domain.RoleSummary, v int64) (bool, error) {
	return w.Update(ctx, r, v)
}
func (w Writer) ReplacePermissions(ctx context.Context, id uint64, ids []uint64) error {
	return w.Replace(ctx, id, ids)
}
