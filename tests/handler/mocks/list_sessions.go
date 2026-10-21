package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
)

type ListSessions struct {
	Self func(context.Context, session.ListInput) (*session.ListResult, error)
	User func(context.Context, uint64, session.ListInput) (*session.ListResult, error)
}

func (s ListSessions) ListMySessions(ctx context.Context, q session.ListInput) (*session.ListResult, error) {
	return s.Self(ctx, q)
}
func (s ListSessions) ListUserSessions(ctx context.Context, id uint64, q session.ListInput) (*session.ListResult, error) {
	return s.User(ctx, id, q)
}
