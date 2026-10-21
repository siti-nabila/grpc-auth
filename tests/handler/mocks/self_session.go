package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
)

type SelfSession struct {
	Single func(context.Context, string) (*session.RevokeResult, error)
	All    func(context.Context) (*session.RevokeAllResult, error)
}

func (s SelfSession) RevokeMySession(ctx context.Context, id string) (*session.RevokeResult, error) {
	return s.Single(ctx, id)
}
func (s SelfSession) RevokeAllMySessions(ctx context.Context) (*session.RevokeAllResult, error) {
	return s.All(ctx)
}
