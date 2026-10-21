package interceptors

import (
	"context"
	"errors"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"google.golang.org/grpc"
	"sync"
)

type refreshAdmissionKey struct{}
type refreshAdmission struct {
	mu        sync.Mutex
	attempted bool
	ip        string
}

// NewRefreshAdmission joins the IP bucket with a session bucket only after the
// feature proves its identity from primary persistence. Early proof failures
// consume only the IP bucket. No request-provided identity selects a bucket.
func NewRefreshAdmission(store *ratelimit.Store, encoder ErrorEncoder) (grpc.UnaryServerInterceptor, func(context.Context, string) error, error) {
	if store == nil || encoder == nil {
		return nil, nil, errors.New("refresh admission requires store and encoder")
	}
	admit := func(ctx context.Context, sessionID string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		state, ok := ctx.Value(refreshAdmissionKey{}).(*refreshAdmission)
		if !ok {
			return common.ErrInternalServerError
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		if state.attempted {
			return common.ErrInternalServerError
		}
		keys := []ratelimit.Key{{Group: ratelimit.RefreshIP, Value: state.ip}}
		if sessionID != "" {
			if err := domain.ValidateSessionID(sessionID); err != nil {
				return common.ErrInternalServerError
			}
			keys = append(keys, ratelimit.Key{Group: ratelimit.RefreshSession, Value: sessionID})
		}
		state.attempted = true
		decision, err := store.Admit(keys...)
		if err != nil {
			return common.ErrInternalServerError
		}
		if !decision.Allowed {
			return rateError(ctx, encoder, decision)
		}
		return nil
	}
	boundary := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if info.FullMethod != authv1.AuthService_RefreshSession_FullMethodName {
			return next(ctx, req)
		}
		caller, err := serviceauth.CallerFromContext(ctx)
		if err != nil {
			return nil, err
		}
		state := &refreshAdmission{ip: trustedIPKey(ctx, caller)}
		ctx = context.WithValue(ctx, refreshAdmissionKey{}, state)
		res, err := next(ctx, req)
		state.mu.Lock()
		attempted := state.attempted
		state.mu.Unlock()
		if !attempted {
			if admissionErr := admit(ctx, ""); admissionErr != nil {
				if errors.Is(err, common.ErrDependencyUnavailable) {
					return nil, err
				}
				return nil, admissionErr
			}
		}
		if !attempted && err == nil {
			return nil, common.ErrInternalServerError
		}
		return res, err
	}
	return boundary, admit, nil
}
