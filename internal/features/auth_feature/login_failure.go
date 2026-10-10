package authfeature

import (
	"context"

	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *sessionLoginService) recordDenial(ctx context.Context) {
	// Observers are trusted local counters, never labels derived from the request.
	defer func() {
		if recover() != nil {
			s.deps.ObserveAuditFailure()
		}
	}()
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		s.deps.ObserveAuditFailure()
		return
	}
	actor, err := domain.ActorForVerifiedService(caller.ID())
	if err == nil {
		err = s.deps.RecordFailure(ctx, domain.AuditRequest{Actor: actor, Action: types.AuditLoginFailed, TargetType: types.AuditTargetRPC, TargetID: loginFullMethod, Reason: types.AuditCredentialDenied, OccurredAt: s.deps.Now().UTC()})
	}
	if err != nil {
		s.deps.ObserveAuditFailure()
	}
}

// RecordFailedLogin owns an independent feature transaction; the caller invokes
// it only after the issuance transaction has finished rolling back.
func RecordFailedLogin(ctx context.Context, provider domain.TransactionProvider, request domain.AuditRequest) (err error) {
	return featurecommon.RecordSecurityAttempt(ctx, provider, request)
}
