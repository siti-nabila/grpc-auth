package authfeature

import (
	"context"
	"time"

	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *refreshService) revokeReusedRefresh(ctx context.Context, r RefreshRepositories, token domain.InitialRefresh, state domain.SessionState, now time.Time) error {
	changed, err := r.Sessions.RevokeSession(ctx, domain.SessionRevocation{UserID: state.UserID, ActorID: state.UserID, SessionID: state.SessionID, At: now, Reason: types.AuditRefreshReuse})
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if _, err = r.Family.RevokeFamily(ctx, domain.RefreshFamilyRevocation{SessionID: state.SessionID, FamilyID: token.FamilyID, At: now}); err != nil {
		return featurecommon.DependencyError(err)
	}
	actor, err := domain.ActorForVerifiedUser(state.UserID)
	if err != nil {
		return err
	}
	request := domain.AuditRequest{Actor: actor, Action: types.AuditRefreshReuseDetected, TargetType: types.AuditTargetSession, TargetID: state.SessionID, Reason: types.AuditRefreshReuse, OccurredAt: now}
	if err = r.Audit.Create(request); err != nil {
		return featurecommon.DependencyError(err)
	}
	if !changed {
		return nil
	}
	request.Action = types.AuditSessionRevoked
	request.After = &domain.SafeSnapshot{SessionID: state.SessionID, Status: string(types.SessionRevoked)}
	if err = r.Audit.Create(request); err != nil {
		return featurecommon.DependencyError(err)
	}
	id, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	event := featurecommon.SessionRevokedEvent(ctx, id, state.UserID, state.UserID, state.SessionID, types.AuditRefreshReuse, now)
	return featurecommon.DependencyError(r.Outbox.Create(event))
}
