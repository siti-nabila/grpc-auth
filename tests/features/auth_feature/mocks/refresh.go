package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"time"
)

type RefreshReader struct {
	Read func(context.Context, []byte) (domain.InitialRefresh, error)
}

func (r RefreshReader) GetByDigest(ctx context.Context, d []byte) (domain.InitialRefresh, error) {
	return r.Read(ctx, d)
}

type RefreshRotation struct {
	Insert       func(domain.InitialRefresh) error
	ConsumeToken func(context.Context, domain.RefreshConsumption) (bool, error)
}

func (w RefreshRotation) Create(r domain.InitialRefresh) error { return w.Insert(r) }
func (w RefreshRotation) Consume(ctx context.Context, r domain.RefreshConsumption) (bool, error) {
	return w.ConsumeToken(ctx, r)
}

type SessionStateReader struct {
	Read func(context.Context, string) (domain.SessionState, error)
}

func (r SessionStateReader) GetSessionState(ctx context.Context, id string) (domain.SessionState, error) {
	return r.Read(ctx, id)
}

type SessionRevocation struct {
	Single func(context.Context, domain.SessionRevocation) (bool, error)
	All    func(context.Context, domain.SessionRevocation) (int64, error)
}

func (w SessionRevocation) RevokeSession(ctx context.Context, r domain.SessionRevocation) (bool, error) {
	return w.Single(ctx, r)
}
func (w SessionRevocation) RevokeUserSessions(ctx context.Context, r domain.SessionRevocation) (int64, error) {
	return w.All(ctx, r)
}

type RefreshRevocation struct {
	Session func(context.Context, string, time.Time) (int64, error)
	Family  func(context.Context, domain.RefreshFamilyRevocation) (int64, error)
	All     func(context.Context, uint64, time.Time) (int64, error)
}

func (w RefreshRevocation) RevokeFamily(ctx context.Context, r domain.RefreshFamilyRevocation) (int64, error) {
	return w.Family(ctx, r)
}
func (w RefreshRevocation) RevokeUserRefresh(ctx context.Context, id uint64, at time.Time) (int64, error) {
	return w.All(ctx, id, at)
}

func (w RefreshRevocation) RevokeSessionRefresh(ctx context.Context, id string, at time.Time) (int64, error) {
	return w.Session(ctx, id, at)
}
