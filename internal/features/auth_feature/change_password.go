package authfeature

import (
	"context"
	"errors"
	"strconv"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *changePasswordService) ChangePassword(ctx context.Context, req domain.ChangePasswordInput) (res *ChangePasswordResult, err error) {
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
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = p.AuthorizeMethod(authv1.AuthService_ChangePassword_FullMethodName); err != nil {
		return nil, err
	}
	if err = req.Validate(); err != nil {
		return nil, err
	}
	candidate, hash, err := s.preparePassword(ctx, p, req)
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
	verified, err := AuthenticateSession(ctx, p.Claims(), r.State, now)
	if err != nil {
		return nil, err
	}
	p, err = sessions.PrincipalFromContext(verified)
	if err != nil {
		return nil, err
	}
	if err = p.AuthorizeMethod(authv1.AuthService_ChangePassword_FullMethodName); err != nil {
		return nil, err
	}
	current, err := r.Account.GetByID(ctx, p.UserID())
	if errors.Is(err, auth.ErrNotFound) {
		return nil, auth.ErrInvalidSession
	}
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !loginEligible(current) || !sameLoginProof(candidate, current) || current.AuthVersion != p.AuthVersion() || current.Version != p.UserVersion() {
		return nil, auth.ErrInvalidSession
	}
	change := domain.PasswordChange{CredentialInvalidation: domain.CredentialInvalidation{UserID: p.UserID(), ExpectedVersion: p.UserVersion(), ExpectedAuthVersion: p.AuthVersion(), At: now}, PasswordHash: hash}
	if err = change.Validate(); err != nil {
		return nil, err
	}
	changed, err := r.Credentials.ChangePassword(ctx, change)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if !changed {
		return nil, user.ErrVersionConflict
	}
	count, err := r.Sessions.RevokeUserSessions(ctx, domain.SessionRevocation{UserID: p.UserID(), ActorID: p.UserID(), At: now, Reason: types.AuditPasswordChange})
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if count < 1 {
		return nil, auth.ErrInvalidSession
	}
	if _, err = r.Refresh.RevokeUserRefresh(ctx, p.UserID(), now); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err = s.persistPassword(ctx, r, change); err != nil {
		return nil, err
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &ChangePasswordResult{Changed: true, LoginRequired: true}, nil
}

func (s *changePasswordService) preparePassword(ctx context.Context, p sessions.Principal, req domain.ChangePasswordInput) (domain.LoginAccount, string, error) {
	a, err := s.deps.Candidate.GetByID(ctx, p.UserID())
	if errors.Is(err, auth.ErrNotFound) {
		return a, "", auth.ErrInvalidSession
	}
	if err != nil {
		return a, "", featurecommon.DependencyError(err)
	}
	if !loginEligible(a) || a.ID != p.UserID() || a.AuthVersion != p.AuthVersion() {
		return a, "", auth.ErrInvalidSession
	}
	valid, err := s.deps.VerifyPassword(a.Password, req.CurrentPassword)
	if err != nil {
		return a, "", featurecommon.DependencyError(err)
	}
	if !valid {
		return a, "", auth.ErrPasswordMismatch
	}
	hash, err := s.deps.HashPassword(req.NewPassword)
	if err != nil {
		return a, "", featurecommon.DependencyError(err)
	}
	// Reject an invalid producer result before taking any transaction locks.
	if hash == "" || len(hash) > 150 {
		return a, "", common.ErrInternalServerError
	}
	if err = ctx.Err(); err != nil {
		return a, "", err
	}
	return a, hash, nil
}

func (s *changePasswordService) persistPassword(ctx context.Context, r PasswordRepositories, c domain.PasswordChange) error {
	actor, err := domain.ActorForVerifiedUser(c.UserID)
	if err != nil {
		return err
	}
	for _, action := range []types.AuditAction{types.AuditPasswordChanged, types.AuditUserSessionsRevoked} {
		err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(c.UserID, 10), Reason: types.AuditPasswordChange, Before: &domain.SafeSnapshot{Version: c.ExpectedVersion}, After: &domain.SafeSnapshot{Version: c.ExpectedVersion + 1, ReasonCode: string(types.AuditPasswordChange)}, OccurredAt: c.At})
		if err != nil {
			return featurecommon.DependencyError(err)
		}
	}
	id, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	e := featurecommon.UserEvent(ctx, id, types.EventUserSessionsRevoked, c.UserID, c.ExpectedVersion+1, &c.UserID, types.UserActive, c.At)
	e.Data.ReasonCode = string(types.AuditPasswordChange)
	revoked := true
	e.Data.SessionsRevoked = &revoked
	return featurecommon.DependencyError(r.Outbox.Create(e))
}
