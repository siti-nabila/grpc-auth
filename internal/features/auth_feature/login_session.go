package authfeature

import (
	"context"
	"errors"
	"strings"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

const loginFullMethod = userv1.UserService_Login_FullMethodName

func (s *sessionLoginService) Login(ctx context.Context, req domain.AuthRequest) (res *LoginResult, err error) {
	// Registered last: transaction cleanup runs before failed-attempt recording.
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = nil
		}
		if errors.Is(err, auth.ErrLoginFailed) {
			s.recordDenial(ctx)
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = serviceauth.CallerFromContext(ctx); err != nil {
		return nil, err
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if err = req.Validate(); err != nil {
		return nil, err
	}
	candidate, err := s.deps.Candidate.GetByEmail(ctx, req.Email)
	if errors.Is(err, auth.ErrNotFound) {
		return nil, auth.ErrLoginFailed
	}
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !loginEligible(candidate) {
		return nil, auth.ErrLoginFailed
	}
	valid, err := s.deps.VerifyPassword(candidate.Password, req.Password)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !valid {
		return nil, auth.ErrLoginFailed
	}
	credentials, err := s.deps.Credentials()
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, featurecommon.UserLockKey(candidate.ID), "auth:session:"+credentials.SessionID); err != nil {
		if errors.Is(err, auth.ErrResourceBusy) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	repos := s.deps.Bind(tx)
	if repos.Account == nil || repos.Session == nil || repos.Refresh == nil || repos.Audit == nil {
		return nil, common.ErrInternalServerError
	}
	current, err := repos.Account.GetByID(ctx, candidate.ID)
	if errors.Is(err, auth.ErrNotFound) {
		return nil, auth.ErrLoginFailed
	}
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !loginEligible(current) || !sameLoginProof(candidate, current) {
		return nil, auth.ErrLoginFailed
	}
	res, err = s.persistLogin(ctx, repos, current, credentials)
	if err != nil {
		return nil, err
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return res, nil
}

func loginEligible(a domain.LoginAccount) bool {
	return a.ID > 0 && a.ID <= 2147483647 && a.Status == types.UserActive && !a.DeletedAt.Valid && a.AuthVersion > 0 && a.Version > 0
}

func sameLoginProof(a, b domain.LoginAccount) bool {
	return a.ID == b.ID && a.Email == b.Email && a.Password == b.Password && a.Status == b.Status && a.AuthVersion == b.AuthVersion && a.Version == b.Version && a.MustChangePassword == b.MustChangePassword
}

func (s *sessionLoginService) persistLogin(ctx context.Context, repos LoginRepositories, account domain.LoginAccount, c InitialCredentials) (*LoginResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := s.deps.Now().UTC().Truncate(time.Second)
	expires := now.Add(jwt.MaxSessionLifetime)
	session := domain.Session{ID: c.SessionID, UserID: account.ID, Status: types.SessionActive, CreatedAt: now, LastSeenAt: now, ExpiresAt: expires, AuthVersion: account.AuthVersion}
	refresh := domain.InitialRefresh{ID: c.RefreshID, SessionID: c.SessionID, TokenHash: c.RefreshDigest, FamilyID: c.FamilyID, IssuedAt: now, ExpiresAt: expires}
	if err := repos.Session.Create(session); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err := repos.Refresh.Create(refresh); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	actor, err := domain.ActorForVerifiedUser(account.ID)
	if err != nil {
		return nil, err
	}
	if err = repos.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditSessionCreated, TargetType: types.AuditTargetSession, TargetID: c.SessionID, After: &domain.SafeSnapshot{SessionID: c.SessionID, Status: string(types.SessionActive)}, OccurredAt: now}); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	access, err := s.deps.Issue(jwt.AccessIssue{UserID: account.ID, SessionID: c.SessionID, TokenID: c.TokenID, AuthVersion: account.AuthVersion, IssuedAt: now, SessionExpiresAt: expires})
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if access.Token == "" || c.RefreshToken == "" {
		return nil, common.ErrInternalServerError
	}
	return &LoginResult{AccessToken: access.Token, RefreshToken: c.RefreshToken, SessionID: c.SessionID, AccessExpiresAt: access.ExpiresAt, SessionExpiresAt: expires, MustChangePassword: account.MustChangePassword}, nil
}
