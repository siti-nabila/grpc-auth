package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type LoginAccountReader struct {
	Email func(context.Context, string) (domain.LoginAccount, error)
	ID    func(context.Context, uint64) (domain.LoginAccount, error)
}

func (r LoginAccountReader) GetByEmail(ctx context.Context, email string) (domain.LoginAccount, error) {
	return r.Email(ctx, email)
}
func (r LoginAccountReader) GetByID(ctx context.Context, id uint64) (domain.LoginAccount, error) {
	return r.ID(ctx, id)
}

type SessionWriter struct{ Write func(domain.Session) error }

func (w SessionWriter) Create(s domain.Session) error { return w.Write(s) }

type RefreshWriter struct {
	Write func(domain.InitialRefresh) error
}

func (w RefreshWriter) Create(s domain.InitialRefresh) error { return w.Write(s) }

type AuditWriter struct {
	Write func(domain.AuditRequest) error
}

func (w AuditWriter) Create(s domain.AuditRequest) error { return w.Write(s) }
