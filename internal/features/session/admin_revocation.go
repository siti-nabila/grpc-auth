package session

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"strconv"
	"time"
)

func (s *adminService) revokeAdminSingle(ctx context.Context, r AdminRepositories, actorID uint64, target domain.SessionUserState, id string, now time.Time) (domain.SafeCommandResult, error) {
	result := domain.SafeCommandResult{UserID: target.ID, SessionID: id, Version: target.Version}
	session, err := r.Targets.GetByID(ctx, id)
	if errors.Is(err, auth.ErrNotFound) {
		return result, err
	}
	if err != nil {
		return result, featurecommon.DependencyError(err)
	}
	if session.ID != id || session.UserID != target.ID {
		return result, common.ErrForbidden
	}
	if session.Status != types.SessionActive && session.Status != types.SessionRevoked {
		return result, common.ErrInternalServerError
	}
	if session.Status == types.SessionActive {
		result.Changed, err = r.Sessions.RevokeSession(ctx, domain.SessionRevocation{UserID: target.ID, ActorID: actorID, SessionID: id, At: now, Reason: types.AuditAdminSessionRevoked})
		if err != nil {
			return result, featurecommon.DependencyError(err)
		}
		if !result.Changed {
			return result, auth.ErrInvalidSession
		}
		if _, err = r.Refresh.RevokeSessionRefresh(ctx, id, now); err != nil {
			return result, featurecommon.DependencyError(err)
		}
	}
	actor, err := domain.ActorForVerifiedUser(actorID)
	if err != nil {
		return result, err
	}
	action := types.AuditSessionRevoked
	if !result.Changed {
		action = types.AuditNoOp
	}
	err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetSession, TargetID: id, Reason: types.AuditAdminSessionRevoked, After: &domain.SafeSnapshot{SessionID: id, Status: string(types.SessionRevoked), NoOp: !result.Changed}, OccurredAt: now})
	if err != nil {
		return result, featurecommon.DependencyError(err)
	}
	if result.Changed {
		idEvent, e := s.deps.EventID()
		if e != nil {
			return result, featurecommon.DependencyError(e)
		}
		err = r.Outbox.Create(featurecommon.SessionRevokedEvent(ctx, idEvent, target.ID, actorID, id, types.AuditAdminSessionRevoked, now))
		if err != nil {
			return result, featurecommon.DependencyError(err)
		}
	}
	return result, nil
}

func (s *adminService) revokeAdminGlobal(ctx context.Context, r AdminRepositories, actorID uint64, target domain.SessionUserState, expected int64, now time.Time) (domain.SafeCommandResult, error) {
	result := domain.SafeCommandResult{UserID: target.ID, Version: target.Version + 1, Changed: true}
	if target.Version != expected {
		return result, user.ErrVersionConflict
	}
	c := domain.CredentialInvalidation{UserID: target.ID, ExpectedVersion: expected, ExpectedAuthVersion: target.AuthVersion, At: now}
	if err := c.Validate(); err != nil {
		return result, err
	}
	changed, err := r.Counters.Invalidate(ctx, c)
	if err != nil {
		return result, featurecommon.DependencyError(err)
	}
	if !changed {
		return result, user.ErrVersionConflict
	}
	if _, err = r.Sessions.RevokeUserSessions(ctx, domain.SessionRevocation{UserID: target.ID, ActorID: actorID, At: now, Reason: types.AuditAdminSessionsRevoked}); err != nil {
		return result, featurecommon.DependencyError(err)
	}
	if _, err = r.Refresh.RevokeUserRefresh(ctx, target.ID, now); err != nil {
		return result, featurecommon.DependencyError(err)
	}
	actor, err := domain.ActorForVerifiedUser(actorID)
	if err != nil {
		return result, err
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditUserSessionsRevoked, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(target.ID, 10), Reason: types.AuditAdminSessionsRevoked, Before: &domain.SafeSnapshot{Version: target.Version}, After: &domain.SafeSnapshot{Version: result.Version, ReasonCode: string(types.AuditAdminSessionsRevoked)}, OccurredAt: now}); err != nil {
		return result, featurecommon.DependencyError(err)
	}
	id, err := s.deps.EventID()
	if err != nil {
		return result, featurecommon.DependencyError(err)
	}
	e := featurecommon.UserEvent(ctx, id, types.EventUserSessionsRevoked, target.ID, result.Version, &actorID, target.Status, now)
	e.Data.ReasonCode = string(types.AuditAdminSessionsRevoked)
	revoked := true
	e.Data.SessionsRevoked = &revoked
	return result, featurecommon.DependencyError(r.Outbox.Create(e))
}
