package interceptors

import (
	"context"
	"errors"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"google.golang.org/grpc"
)

const attemptAuditTimeout = 2 * time.Second

// NewSecurityAttemptInterceptor belongs inside service authentication and outside
// session/rate/command handling. Commands finish rollback before it records an
// independent attempt. Its observer must be a bounded local counter, not labels
// or logs derived from credentials, errors or request payloads.
func NewSecurityAttemptInterceptor(record func(context.Context, domain.AuditRequest) error, observeFailure func(), now func() time.Time) (grpc.UnaryServerInterceptor, error) {
	if record == nil || observeFailure == nil || now == nil {
		return nil, errors.New("incomplete security attempt dependencies")
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (res any, err error) {
		res, err = next(ctx, req)
		if err == nil {
			return res, nil
		}
		// Quota/capacity rejections use the limiter's bounded aggregate telemetry;
		// do not turn attack traffic into one database transaction per rejection.
		decoded, encoded := grpcerror.Decode(err)
		if errors.Is(err, common.ErrRateLimited) || encoded && decoded.Reason == common.ErrRateLimited.Key() {
			return res, err
		}
		// Login already records its generic credential denial independently.
		if info.FullMethod == userv1.UserService_Login_FullMethodName && errors.Is(err, auth.ErrLoginFailed) {
			return res, err
		}
		caller, callerErr := serviceauth.CallerFromContext(ctx)
		if callerErr != nil {
			// The schema requires a verified actor. Never manufacture one for an
			// unknown caller or trust a user identity from rejected credentials.
			return res, err
		}
		recordSafeAttempt(ctx, caller.ID(), info.FullMethod, record, observeFailure, now)
		return res, err
	}, nil
}

func recordSafeAttempt(ctx context.Context, service, method string, record func(context.Context, domain.AuditRequest) error, observeFailure func(), now func() time.Time) {
	defer func() {
		if recover() != nil {
			observeFailure()
		}
	}()
	actor, err := domain.ActorForVerifiedService(service)
	if err != nil {
		observeFailure()
		return
	}
	// Preserve correlation/locale metadata while bounding a separate transaction
	// even when the failed request has already been canceled.
	attemptCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), attemptAuditTimeout)
	defer cancel()
	request := domain.AuditRequest{Actor: actor, Action: types.AuditSecurityAttemptFailed, TargetType: types.AuditTargetRPC, TargetID: method, Reason: types.AuditRequestDenied, OccurredAt: now().UTC()}
	if err = record(attemptCtx, request); err != nil {
		observeFailure()
	}
}
