package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
)

type AdminSessions struct {
	Single func(context.Context, session.AdminSingleInput) (*session.AdminResult, error)
	All    func(context.Context, session.AdminGlobalInput) (*session.AdminResult, error)
}

func (s AdminSessions) RevokeUserSession(ctx context.Context, r session.AdminSingleInput) (*session.AdminResult, error) {
	return s.Single(ctx, r)
}
func (s AdminSessions) RevokeAllUserSessions(ctx context.Context, r session.AdminGlobalInput) (*session.AdminResult, error) {
	return s.All(ctx, r)
}
