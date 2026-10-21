package interceptors

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	userdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/user"

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
		res, err = invokeAttempt(ctx, req, next)
		if err == nil {
			if info.FullMethod != authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName {
				return res, nil
			}
			decision, ok := res.(*authorizationv1.EvaluateAccessResponse)
			if !ok || !validAccessDecision(decision) {
				res, err = nil, common.ErrInternalServerError
			} else if decision.Allowed {
				return res, nil
			} else {
				caller, callerErr := serviceauth.CallerFromContext(ctx)
				if callerErr == nil {
					recordSafeAttempt(ctx, caller.ID(), info.FullMethod, types.AuditRequestDenied, record, observeFailure, now)
				}
				return res, nil
			}
		}
		// Quota/capacity rejections use the limiter's bounded aggregate telemetry;
		// do not turn attack traffic into one database transaction per rejection.
		decoded, encoded := grpcerror.Decode(err)
		if errors.Is(err, common.ErrRateLimited) || encoded && decoded.Reason == common.ErrRateLimited.Key() {
			return res, err
		}
		// Login already records its generic credential denial independently.
		if info.FullMethod == userv1.UserService_Login_FullMethodName && errors.Is(err, auth.ErrLoginFailed) ||
			info.FullMethod == authv1.AuthService_RefreshSession_FullMethodName && errors.Is(err, auth.ErrRefreshTokenReused) {
			return res, err
		}
		caller, callerErr := serviceauth.CallerFromContext(ctx)
		if callerErr != nil {
			// The schema requires a verified actor. Never manufacture one for an
			// unknown caller or trust a user identity from rejected credentials.
			return res, err
		}
		recordSafeAttempt(ctx, caller.ID(), info.FullMethod, safeAttemptReason(err), record, observeFailure, now)
		return res, err
	}, nil
}

func validAccessDecision(res *authorizationv1.EvaluateAccessResponse) bool {
	if res == nil || domain.ValidateManagementTarget(res.UserId) != nil || domain.ValidateSessionID(res.Sid) != nil || res.UserVersion == 0 || res.UserVersion > math.MaxInt64 || res.EvaluatedAt.CheckValid() != nil {
		return false
	}
	if res.Allowed {
		return res.Reason == authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_ALLOWED
	}
	return res.Reason >= authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING && res.Reason <= authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_ACCESS_REQUIRED
}

func recordSafeAttempt(ctx context.Context, service, method string, reason types.AuditReason, record func(context.Context, domain.AuditRequest) error, observeFailure func(), now func() time.Time) {
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
	request := domain.AuditRequest{Actor: actor, Action: types.AuditSecurityAttemptFailed, TargetType: types.AuditTargetRPC, TargetID: method, Reason: reason, OccurredAt: now().UTC()}
	if err = record(attemptCtx, request); err != nil {
		observeFailure()
	}
}

// Recover only after downstream defers have cleaned up their feature-owned tx.
func invokeAttempt(ctx context.Context, req any, next grpc.UnaryHandler) (res any, err error) {
	defer func() {
		if recover() != nil {
			res = nil
			err = common.ErrInternalServerError
		}
	}()
	return next(ctx, req)
}

// Persist catalog identity/category, never err.Error() or arbitrary status text.
func safeAttemptReason(err error) types.AuditReason {
	if errors.Is(err, context.Canceled) {
		return types.AuditRequestCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return types.AuditDeadlineExceeded
	}
	var fields dictionary.FieldErrors
	if errors.As(err, &fields) {
		return types.AuditValidationFailed
	}
	var catalog *dictionary.Error
	key := ""
	if errors.As(err, &catalog) {
		key = catalog.Key()
	} else if decoded, ok := grpcerror.Decode(err); ok {
		key = decoded.Reason
	}
	if len(key) > 100 {
		return types.AuditInternalFailure
	}
	for _, registry := range []dictionary.Registry{common.Registry(), auth.Registry(), userdictionary.Registry(), roleerrors.Registry(), applicationerrors.Registry()} {
		if _, ok := registry.Lookup(key); ok {
			return types.AuditReason(strings.ToUpper(strings.ReplaceAll(key, ".", "_")))
		}
	}
	return types.AuditInternalFailure
}
