package authfeature

import (
	"context"
	"errors"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *logoutService) Logout(ctx context.Context, credential sessions.LogoutCredential) (res *LogoutResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = nil
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = serviceauth.CallerFromContext(ctx); err != nil {
		return nil, err
	}
	state, err := logoutProof(ctx, credential, s.deps.Tokens, s.deps.State, s.deps.Now().UTC())
	if err != nil {
		return nil, err
	}
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, featurecommon.UserLockKey(state.UserID), "auth:session:"+state.SessionID); err != nil {
		if errors.Is(err, auth.ErrResourceBusy) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	r := s.deps.Bind(tx)
	if r.Tokens == nil || r.State == nil || r.Sessions == nil || r.Refresh == nil || r.Audit == nil || r.Outbox == nil {
		return nil, common.ErrInternalServerError
	}
	now := s.deps.Now().UTC()
	locked, err := logoutProof(ctx, credential, r.Tokens, r.State, now)
	if err != nil {
		return nil, err
	}
	if locked.UserID != state.UserID || locked.SessionID != state.SessionID {
		return nil, auth.ErrInvalidSession
	}
	changed, err := r.Sessions.RevokeSession(ctx, domain.SessionRevocation{UserID: locked.UserID, ActorID: locked.UserID, SessionID: locked.SessionID, At: now, Reason: types.AuditLogout})
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !changed && locked.SessionStatus == types.SessionActive {
		return nil, auth.ErrInvalidSession
	}
	if changed {
		if err = s.persistLogout(ctx, r, locked, now); err != nil {
			return nil, err
		}
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &LogoutResult{SessionID: locked.SessionID, Changed: changed}, nil
}

func logoutProof(ctx context.Context, c sessions.LogoutCredential, tokens domain.RefreshReader, states domain.SessionStateReader, now time.Time) (domain.SessionState, error) {
	if raw, ok := c.RefreshToken(); ok {
		digest, err := refreshDigest(raw)
		if err != nil {
			return domain.SessionState{}, err
		}
		token, state, err := readRefreshProof(ctx, tokens, states, digest)
		if err != nil {
			return state, err
		}
		if token.SessionID != state.SessionID || token.ConsumedAt.Valid || token.ReplacedBy.Valid || domain.ValidateSessionID(token.ID) != nil || domain.ValidateSessionID(token.FamilyID) != nil || token.IssuedAt.Before(state.CreatedAt) || token.IssuedAt.After(now) || !now.Before(token.ExpiresAt) || token.ExpiresAt.After(state.ExpiresAt) {
			return state, auth.ErrInvalidSession
		}
		if err = terminalSessionProof(state, now); err != nil {
			return state, err
		}
		if token.RevokedAt.Valid && state.SessionStatus != types.SessionRevoked {
			return state, auth.ErrInvalidSession
		}
		return state, nil
	}
	claims, ok := c.Access()
	if !ok {
		return domain.SessionState{}, auth.ErrInvalidSession
	}
	state, err := states.GetSessionState(ctx, claims.SessionID())
	if errors.Is(err, auth.ErrNotFound) {
		return state, auth.ErrInvalidSession
	}
	if err != nil {
		return state, featurecommon.DependencyError(err)
	}
	if err = terminalSessionProof(state, now); err != nil {
		return state, err
	}
	if claims.UserID() != state.UserID || claims.SessionID() != state.SessionID || claims.AuthVersion() != state.AuthVersion || claims.ExpiresAt().After(state.ExpiresAt) || !now.Before(claims.ExpiresAt().Add(jwt.AccessClockSkew)) || claims.IssuedAt().After(now.Add(jwt.AccessClockSkew)) {
		return state, auth.ErrInvalidSession
	}
	return state, nil
}

func terminalSessionProof(s domain.SessionState, now time.Time) error {
	if s.UserID == 0 || s.UserID > 2147483647 || s.UserID != s.SessionUserID || domain.ValidateSessionID(s.SessionID) != nil || s.UserStatus != types.UserActive || s.DeletedAt.Valid || s.UserVersion < 1 || !s.SessionAuthVersion.Valid || s.SessionAuthVersion.Int64 < 1 || s.SessionAuthVersion.Int64 != s.AuthVersion {
		return auth.ErrInvalidSession
	}
	if s.CreatedAt.IsZero() || s.CreatedAt.After(now) || !s.ExpiresAt.After(s.CreatedAt) || s.ExpiresAt.Sub(s.CreatedAt) > jwt.MaxSessionLifetime || !now.Before(s.ExpiresAt) {
		return auth.ErrInvalidSession
	}
	if s.SessionStatus == types.SessionActive && !s.RevokedAt.Valid || s.SessionStatus == types.SessionRevoked && s.RevokedAt.Valid {
		return nil
	}
	return auth.ErrInvalidSession
}

func (s *logoutService) persistLogout(ctx context.Context, r LogoutRepositories, state domain.SessionState, now time.Time) error {
	if _, err := r.Refresh.RevokeSessionRefresh(ctx, state.SessionID, now); err != nil {
		return featurecommon.DependencyError(err)
	}
	actor, err := domain.ActorForVerifiedUser(state.UserID)
	if err != nil {
		return err
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditSessionRevoked, TargetType: types.AuditTargetSession, TargetID: state.SessionID, Reason: types.AuditLogout, After: &domain.SafeSnapshot{SessionID: state.SessionID, Status: string(types.SessionRevoked)}, OccurredAt: now}); err != nil {
		return featurecommon.DependencyError(err)
	}
	id, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	return featurecommon.DependencyError(r.Outbox.Create(featurecommon.SessionRevokedEvent(ctx, id, state.UserID, state.UserID, state.SessionID, types.AuditLogout, now)))
}
