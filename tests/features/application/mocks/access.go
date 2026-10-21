package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
	"time"
)

type Reader struct {
	Pair  func(context.Context, uint64, uint64) (*domain.ApplicationAccessItem, error)
	Roles func(context.Context, uint64, uint64) ([]domain.UserRoleAssignmentItem, error)
	List  func(context.Context, domain.CatalogQuery) (orm.PageData[domain.ApplicationAccessItem], error)
}

func (r Reader) Access(c context.Context, u, a uint64) (*domain.ApplicationAccessItem, error) {
	return r.Pair(c, u, a)
}
func (r Reader) ApplicationAssignments(c context.Context, u, a uint64) ([]domain.UserRoleAssignmentItem, error) {
	return r.Roles(c, u, a)
}
func (r Reader) ListAccess(c context.Context, q domain.CatalogQuery) (orm.PageData[domain.ApplicationAccessItem], error) {
	return r.List(c, q)
}

type Lookup struct {
	Read func(context.Context, string) (domain.ApplicationItem, error)
}

func (r Lookup) Application(c context.Context, code string) (domain.ApplicationItem, error) {
	return r.Read(c, code)
}

type Writer struct {
	Grant  func(context.Context, uint64, uint64, *domain.ApplicationAccessItem, uint64, time.Time) (bool, error)
	Remove func(context.Context, uint64, domain.ApplicationAccessItem, uint64, string, time.Time) (bool, error)
}

func (w Writer) Activate(c context.Context, u, a uint64, b *domain.ApplicationAccessItem, actor uint64, at time.Time) (bool, error) {
	return w.Grant(c, u, a, b, actor, at)
}
func (w Writer) Revoke(c context.Context, u uint64, b domain.ApplicationAccessItem, actor uint64, reason string, at time.Time) (bool, error) {
	return w.Remove(c, u, b, actor, reason, at)
}
