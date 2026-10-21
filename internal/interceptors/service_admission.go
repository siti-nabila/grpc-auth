package interceptors

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"google.golang.org/grpc"
)

// Service admission is a separate nonrefunded stage, outside Attempt and Session.
func NewServiceAdmission(store *ratelimit.Store, encoder ErrorEncoder) (grpc.UnaryServerInterceptor, error) {
	if store == nil || encoder == nil {
		return nil, errors.New("service admission requires limiter and encoder")
	}
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		caller, err := serviceauth.CallerFromContext(ctx)
		if err != nil {
			return nil, err
		}
		decision, err := store.Admit(ratelimit.Key{Group: ratelimit.ServiceIngress, Value: caller.ID()})
		if err != nil {
			return nil, common.ErrInternalServerError
		}
		if !decision.Allowed {
			return nil, rateError(ctx, encoder, decision)
		}
		return next(ctx, req)
	}, nil
}
