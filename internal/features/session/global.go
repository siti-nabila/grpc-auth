package session

import (
	"context"
	"errors"
	"strconv"
	"time"

	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *selfService) RevokeAllMySessions(ctx context.Context) (res *RevokeAllResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = nil
		}
	}()
	p, err := selfPrincipal(ctx, sessionv1.SessionService_RevokeAllMySessions_FullMethodName)
	if err != nil {
		return nil, err
	}
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, featurecommon.UserLockKey(p.UserID())); err != nil {
		if errors.Is(err, auth.ErrResourceBusy) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	r := s.deps.Bind(tx)
	if !r.complete() {
		return nil, common.ErrInternalServerError
	}
	ids, err := r.Targets.ListIDs(ctx, p.UserID())
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	// Login and all revocation paths share the user lock. No new owned session
	// can appear while this command discovers and locks the complete set.
	keys := []string{"auth:session:" + p.SessionID()}
	for _, id := range ids {
		if domain.ValidateSessionID(id) != nil {
			return nil, common.ErrInternalServerError
		}
		keys = append(keys, "auth:session:"+id)
	}
	if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, keys...); err != nil {
		if errors.Is(err, auth.ErrResourceBusy) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	now := s.deps.Now().UTC()
	p, err = lockedPrincipal(ctx, r, p, now, sessionv1.SessionService_RevokeAllMySessions_FullMethodName)
	if err != nil {
		return nil, err
	}
	change := domain.CredentialInvalidation{UserID: p.UserID(), ExpectedVersion: p.UserVersion(), ExpectedAuthVersion: p.AuthVersion(), At: now}
	if err = change.Validate(); err != nil {
		return nil, err
	}
	changed, err := r.Counters.Invalidate(ctx, change)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !changed {
		return nil, user.ErrVersionConflict
	}
	count, err := r.Sessions.RevokeUserSessions(ctx, domain.SessionRevocation{UserID: p.UserID(), ActorID: p.UserID(), At: now, Reason: types.AuditSelfSessionsRevoked})
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if count < 1 {
		return nil, auth.ErrInvalidSession
	}
	if _, err = r.Refresh.RevokeUserRefresh(ctx, p.UserID(), now); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err = s.persistGlobal(ctx, r, change, now); err != nil {
		return nil, err
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &RevokeAllResult{Changed: true, LoginRequired: true}, nil
}

func (s *selfService) persistGlobal(ctx context.Context, r SelfRepositories, c domain.CredentialInvalidation, now time.Time) error {
	actor, err := domain.ActorForVerifiedUser(c.UserID)
	if err != nil {
		return err
	}
	reason := types.AuditSelfSessionsRevoked
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditUserSessionsRevoked, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(c.UserID, 10), Before: &domain.SafeSnapshot{Version: c.ExpectedVersion}, After: &domain.SafeSnapshot{Version: c.ExpectedVersion + 1, ReasonCode: string(reason)}, Reason: reason, OccurredAt: now}); err != nil {
		return featurecommon.DependencyError(err)
	}
	id, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	e := featurecommon.UserEvent(ctx, id, types.EventUserSessionsRevoked, c.UserID, c.ExpectedVersion+1, &c.UserID, types.UserActive, now)
	e.Data.ReasonCode = string(reason)
	revoked := true
	e.Data.SessionsRevoked = &revoked
	return featurecommon.DependencyError(r.Outbox.Create(e))
}
