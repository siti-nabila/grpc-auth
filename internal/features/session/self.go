package session

import (
	"context"
	"errors"
	"time"

	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func selfPrincipal(ctx context.Context, method string) (sessions.Principal, error) {
	if err := ctx.Err(); err != nil {
		return sessions.Principal{}, err
	}
	if _, err := serviceauth.CallerFromContext(ctx); err != nil {
		return sessions.Principal{}, err
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return p, err
	}
	return p, p.AuthorizeMethod(method)
}

func lockedPrincipal(ctx context.Context, r SelfRepositories, p sessions.Principal, now time.Time, method string) (sessions.Principal, error) {
	verified, err := authfeature.AuthenticateSession(ctx, p.Claims(), r.State, now)
	if err != nil {
		return sessions.Principal{}, err
	}
	p, err = sessions.PrincipalFromContext(verified)
	if err != nil {
		return p, err
	}
	return p, p.AuthorizeMethod(method)
}

func (s *selfService) RevokeMySession(ctx context.Context, id string) (res *RevokeResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = nil
		}
	}()
	p, err := selfPrincipal(ctx, sessionv1.SessionService_RevokeMySession_FullMethodName)
	if err != nil {
		return nil, err
	}
	if err = domain.ValidateSessionID(id); err != nil {
		return nil, err
	}
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, featurecommon.UserLockKey(p.UserID()), "auth:session:"+p.SessionID(), "auth:session:"+id); err != nil {
		if errors.Is(err, auth.ErrResourceBusy) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	r := s.deps.Bind(tx)
	if !r.complete() {
		return nil, common.ErrInternalServerError
	}
	now := s.deps.Now().UTC()
	p, err = lockedPrincipal(ctx, r, p, now, sessionv1.SessionService_RevokeMySession_FullMethodName)
	if err != nil {
		return nil, err
	}
	changed, err := s.revokeOwnedSession(ctx, r, p, id, now)
	if err != nil {
		return nil, err
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &RevokeResult{Changed: changed}, nil
}

func (s *selfService) persistSingle(ctx context.Context, r SelfRepositories, userID uint64, id string, now time.Time) error {
	if _, err := r.Refresh.RevokeSessionRefresh(ctx, id, now); err != nil {
		return featurecommon.DependencyError(err)
	}
	actor, err := domain.ActorForVerifiedUser(userID)
	if err != nil {
		return err
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditSessionRevoked, TargetType: types.AuditTargetSession, TargetID: id, Reason: types.AuditSelfSessionRevoked, After: &domain.SafeSnapshot{SessionID: id, Status: string(types.SessionRevoked)}, OccurredAt: now}); err != nil {
		return featurecommon.DependencyError(err)
	}
	eventID, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	return featurecommon.DependencyError(r.Outbox.Create(featurecommon.SessionRevokedEvent(ctx, eventID, userID, userID, id, types.AuditSelfSessionRevoked, now)))
}

func (s *selfService) revokeOwnedSession(ctx context.Context, r SelfRepositories, p sessions.Principal, id string, now time.Time) (bool, error) {
	target, err := r.Targets.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			return false, err
		}
		return false, featurecommon.DependencyError(err)
	}
	if target.ID != id || target.UserID != p.UserID() {
		return false, common.ErrForbidden
	}
	if target.Status != types.SessionActive && target.Status != types.SessionRevoked {
		return false, common.ErrInternalServerError
	}
	changed := target.Status == types.SessionActive
	if changed {
		changed, err = r.Sessions.RevokeSession(ctx, domain.SessionRevocation{UserID: p.UserID(), ActorID: p.UserID(), SessionID: id, At: now, Reason: types.AuditSelfSessionRevoked})
		if err != nil {
			return false, featurecommon.DependencyError(err)
		}
		if !changed {
			return false, auth.ErrInvalidSession
		}
		if err = s.persistSingle(ctx, r, p.UserID(), id, now); err != nil {
			return false, err
		}
	}
	return changed, nil
}
