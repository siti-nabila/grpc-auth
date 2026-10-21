package interceptors

import (
	"context"
	"errors"
	"time"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func NewMetricsInterceptor(store *telemetry.Store, now func() time.Time) (grpc.UnaryServerInterceptor, error) {
	if store == nil || now == nil {
		return nil, errors.New("metrics require store and clock")
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (res any, err error) {
		start := now()
		ctx = common.WithFinalizationObserver(ctx, func() { store.AuditFailure(telemetry.FinalizationUnknown) })
		finished := false
		defer func() {
			code := status.Code(err)
			outcome := telemetry.AuthNone
			if !finished {
				code = codes.Internal
			}
			switch code {
			case codes.Unauthenticated:
				outcome = telemetry.AuthFailure
			case codes.PermissionDenied:
				outcome = telemetry.AuthDenied
			case codes.Unavailable:
				outcome = telemetry.AuthDependency
			}
			if err == nil && info.FullMethod == authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName {
				if decision, ok := res.(*authorizationv1.EvaluateAccessResponse); ok && validAccessDecision(decision) && !decision.Allowed {
					outcome = telemetry.AuthDenied
				}
			}
			store.Record(info.FullMethod, code, now().Sub(start), outcome)
		}()
		res, err = next(ctx, req)
		finished = true
		return res, err
	}, nil
}
