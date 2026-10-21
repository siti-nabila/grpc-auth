package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/orm/orm"
	"time"
)

type AssignmentReader struct {
	Roles func(context.Context, uint64, []uint64) ([]domain.UserRoleAssignmentItem, error)
	List  func(context.Context, domain.CatalogQuery) (orm.PageData[domain.UserRoleAssignmentItem], error)
}

func (r AssignmentReader) AssignmentRoles(c context.Context, u uint64, ids []uint64) ([]domain.UserRoleAssignmentItem, error) {
	return r.Roles(c, u, ids)
}
func (r AssignmentReader) ListAssignments(c context.Context, q domain.CatalogQuery) (orm.PageData[domain.UserRoleAssignmentItem], error) {
	return r.List(c, q)
}

type AssignmentWriter struct {
	Grant   func(context.Context, uint64, domain.UserRoleAssignmentItem, uint64, time.Time) (bool, error)
	Remove  func(context.Context, uint64, domain.UserRoleAssignmentItem, uint64, string, time.Time) (bool, error)
	Version func(context.Context, uint64, int64, time.Time) (bool, error)
}

func (w AssignmentWriter) Activate(c context.Context, u uint64, r domain.UserRoleAssignmentItem, a uint64, at time.Time) (bool, error) {
	return w.Grant(c, u, r, a, at)
}
func (w AssignmentWriter) Revoke(c context.Context, u uint64, r domain.UserRoleAssignmentItem, a uint64, reason string, at time.Time) (bool, error) {
	return w.Remove(c, u, r, a, reason, at)
}
func (w AssignmentWriter) AdvanceVersion(c context.Context, u uint64, v int64, at time.Time) (bool, error) {
	return w.Version(c, u, v, at)
}

type AssignmentProvider struct {
	Tx     *shared.Transaction
	Error  error
	Calls  int
	Before func(int)
}

func (p *AssignmentProvider) Begin(c context.Context) (domain.Transaction, error) {
	p.Calls++
	if p.Before != nil {
		p.Before(p.Calls)
	}
	if c != p.Tx.Context {
		panic("begin context")
	}
	p.Tx.Record("begin")
	return p.Tx, p.Error
}
func (p *AssignmentProvider) BeginSnapshot(c context.Context) (domain.Transaction, error) {
	p.Calls++
	if c != p.Tx.Context {
		panic("snapshot context")
	}
	p.Tx.Record("snapshot")
	return p.Tx, p.Error
}
