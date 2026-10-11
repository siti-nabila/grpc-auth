package authfeature

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// refreshDigest accepts the exact opaque encoding emitted by Login, not an ID.
func refreshDigest(raw string) ([]byte, error) {
	if len(raw) != 43 {
		return nil, auth.ErrInvalidSession
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return nil, auth.ErrInvalidSession
	}
	digest := sha256.Sum256([]byte(raw))
	return digest[:], nil
}

func (s *refreshService) Refresh(ctx context.Context, raw string) (res *LoginResult, err error) {
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
	digest, err := refreshDigest(raw)
	if err != nil {
		return nil, err
	}
	candidate, state, err := readRefreshProof(ctx, s.deps.Tokens, s.deps.State, digest)
	if err != nil {
		return nil, err
	}
	if err = validateRefreshProof(candidate, state, s.deps.Now().UTC()); err != nil {
		return nil, err
	}
	// The bucket identity is derived from persisted proof, never request claims.
	if err = s.deps.Admit(ctx, state.SessionID); err != nil {
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
	if !r.complete() {
		return nil, common.ErrInternalServerError
	}
	current, locked, err := readRefreshProof(ctx, r.Tokens, r.State, digest)
	if err != nil {
		return nil, err
	}
	if current.ID != candidate.ID || current.SessionID != state.SessionID || current.FamilyID != candidate.FamilyID || locked.UserID != state.UserID {
		return nil, auth.ErrInvalidSession
	}
	now := s.deps.Now().UTC()
	if err = validateRefreshProof(current, locked, now); err != nil {
		return nil, err
	}
	now = now.Truncate(time.Second)
	if current.ConsumedAt.Valid {
		if err = s.revokeReusedRefresh(ctx, r, current, locked, now); err != nil {
			return nil, err
		}
		if err = owner.Commit(); err != nil {
			return nil, featurecommon.DependencyError(err)
		}
		return nil, auth.ErrRefreshTokenReused
	}
	res, err = s.rotateRefresh(ctx, r, current, locked, now)
	if err != nil {
		return nil, err
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return res, nil
}

func readRefreshProof(ctx context.Context, tokens domain.RefreshReader, states domain.SessionStateReader, digest []byte) (domain.InitialRefresh, domain.SessionState, error) {
	token, err := tokens.GetByDigest(ctx, digest)
	if errors.Is(err, auth.ErrNotFound) {
		return token, domain.SessionState{}, auth.ErrInvalidSession
	}
	if err != nil {
		return token, domain.SessionState{}, featurecommon.DependencyError(err)
	}
	if !bytes.Equal(token.TokenHash, digest) {
		return token, domain.SessionState{}, auth.ErrInvalidSession
	}
	state, err := states.GetSessionState(ctx, token.SessionID)
	if errors.Is(err, auth.ErrNotFound) {
		return token, state, auth.ErrInvalidSession
	}
	if err != nil {
		return token, state, featurecommon.DependencyError(err)
	}
	return token, state, nil
}

func validateRefreshProof(t domain.InitialRefresh, s domain.SessionState, now time.Time) error {
	if domain.ValidateSessionID(t.ID) != nil || domain.ValidateSessionID(t.FamilyID) != nil || domain.ValidateSessionID(t.SessionID) != nil || t.SessionID != s.SessionID || s.UserID == 0 || s.UserID > 2147483647 || s.UserID != s.SessionUserID || s.AuthVersion < 1 || !s.SessionAuthVersion.Valid || s.SessionAuthVersion.Int64 != s.AuthVersion || s.UserVersion < 1 || s.UserStatus != types.UserActive || s.DeletedAt.Valid {
		return auth.ErrInvalidSession
	}
	if s.CreatedAt.IsZero() || s.CreatedAt.After(now) || !s.ExpiresAt.After(s.CreatedAt) || s.ExpiresAt.Sub(s.CreatedAt) > jwt.MaxSessionLifetime || !now.Before(s.ExpiresAt) || !now.Before(t.ExpiresAt) || t.ExpiresAt.After(s.ExpiresAt) || t.IssuedAt.Before(s.CreatedAt) || t.IssuedAt.After(now) {
		return auth.ErrInvalidSession
	}
	if t.ConsumedAt.Valid {
		// Previously revoked consumed proof still detects reuse; never rotates.
		if s.SessionStatus != types.SessionActive && s.SessionStatus != types.SessionRevoked {
			return auth.ErrInvalidSession
		}
		return nil
	}
	if t.RevokedAt.Valid || t.ReplacedBy.Valid || s.SessionStatus != types.SessionActive || s.RevokedAt.Valid {
		return auth.ErrInvalidSession
	}
	if s.MustChangePassword {
		return common.ErrForbidden
	}
	return nil
}

func (s *refreshService) rotateRefresh(ctx context.Context, r RefreshRepositories, old domain.InitialRefresh, state domain.SessionState, now time.Time) (*LoginResult, error) {
	c, err := s.deps.Credentials()
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	digest, err := refreshDigest(c.RefreshToken)
	if err != nil || !bytes.Equal(digest, c.RefreshDigest) || bytes.Equal(digest, old.TokenHash) || c.RefreshID == old.ID {
		return nil, common.ErrInternalServerError
	}
	next := domain.InitialRefresh{ID: c.RefreshID, SessionID: old.SessionID, TokenHash: c.RefreshDigest, FamilyID: old.FamilyID, IssuedAt: now, ExpiresAt: old.ExpiresAt}
	if err = next.Validate(); err != nil {
		return nil, common.ErrInternalServerError
	}
	if err = r.Rotation.Create(next); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	changed, err := r.Rotation.Consume(ctx, domain.RefreshConsumption{ID: old.ID, SuccessorID: next.ID, At: now})
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !changed {
		return nil, auth.ErrInvalidSession
	}
	actor, err := domain.ActorForVerifiedUser(state.UserID)
	if err != nil {
		return nil, err
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditSessionRefreshRotated, TargetType: types.AuditTargetSession, TargetID: state.SessionID, OccurredAt: now}); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	access, err := s.deps.Issue(jwt.AccessIssue{UserID: state.UserID, SessionID: state.SessionID, TokenID: c.TokenID, AuthVersion: state.AuthVersion, IssuedAt: now, SessionExpiresAt: state.ExpiresAt})
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if access.Token == "" {
		return nil, common.ErrInternalServerError
	}
	return &LoginResult{AccessToken: access.Token, RefreshToken: c.RefreshToken, SessionID: state.SessionID, AccessExpiresAt: access.ExpiresAt, SessionExpiresAt: state.ExpiresAt}, nil
}
