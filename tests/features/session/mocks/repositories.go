package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"time"
)

type Targets struct {
	Get  func(context.Context, string) (domain.SessionSummary, error)
	IDs  func(context.Context, uint64) ([]string, error)
	Page func(context.Context, domain.SessionListQuery) (domain.SessionListPage, error)
}

func (r Targets) GetByID(ctx context.Context, id string) (domain.SessionSummary, error) {
	return r.Get(ctx, id)
}
func (r Targets) ListIDs(ctx context.Context, id uint64) ([]string, error) { return r.IDs(ctx, id) }
func (r Targets) ListPage(ctx context.Context, q domain.SessionListQuery) (domain.SessionListPage, error) {
	return r.Page(ctx, q)
}

type Snapshot struct {
	Begin func(context.Context) (domain.Transaction, error)
}

func (s Snapshot) BeginSnapshot(ctx context.Context) (domain.Transaction, error) { return s.Begin(ctx) }

type Permissions struct {
	Read func(context.Context, uint64) ([]domain.PermissionContribution, error)
}

func (r Permissions) Permissions(ctx context.Context, id uint64) ([]domain.PermissionContribution, error) {
	return r.Read(ctx, id)
}

type Counters struct {
	Write func(context.Context, domain.CredentialInvalidation) (bool, error)
}

type Users struct {
	Read func(context.Context, uint64) (domain.SessionUserState, error)
}

func (r Users) GetByID(ctx context.Context, id uint64) (domain.SessionUserState, error) {
	return r.Read(ctx, id)
}

type Idempotency struct {
	Read  func(domain.IdempotencyScope) (*domain.IdempotencyRecord, error)
	Write func(domain.IdempotencyScope, []byte, domain.SafeCommandResult, time.Time) error
}

func (s Idempotency) Lookup(scope domain.IdempotencyScope) (*domain.IdempotencyRecord, error) {
	return s.Read(scope)
}
func (s Idempotency) Save(scope domain.IdempotencyScope, hash []byte, r domain.SafeCommandResult, now time.Time) error {
	return s.Write(scope, hash, r, now)
}

func (w Counters) Invalidate(ctx context.Context, c domain.CredentialInvalidation) (bool, error) {
	return w.Write(ctx, c)
}
func (Counters) ChangePassword(context.Context, domain.PasswordChange) (bool, error) {
	panic("unexpected password write")
}
