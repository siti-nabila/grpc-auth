package session

import (
	"context"
	"errors"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"strconv"
	"time"
)

type adminCommand struct {
	method         string
	target         uint64
	sessionID, key string
	expected       int64
}

func (s *adminService) RevokeUserSession(ctx context.Context, r AdminSingleInput) (*AdminResult, error) {
	if err := domain.ValidateSessionID(r.SessionID); err != nil {
		return nil, err
	}
	return s.runAdmin(ctx, adminCommand{method: sessionv1.SessionService_RevokeUserSession_FullMethodName, target: r.UserID, sessionID: r.SessionID, key: r.Key})
}
func (s *adminService) RevokeAllUserSessions(ctx context.Context, r AdminGlobalInput) (*AdminResult, error) {
	if r.ExpectedVersion < 1 {
		return nil, common.ErrBadRequest
	}
	return s.runAdmin(ctx, adminCommand{method: sessionv1.SessionService_RevokeAllUserSessions_FullMethodName, target: r.UserID, key: r.Key, expected: r.ExpectedVersion})
}

func (c adminCommand) scope(actor uint64) (domain.IdempotencyScope, error) {
	if c.target == 0 || c.target > 2147483647 {
		return domain.IdempotencyScope{}, common.ErrBadRequest
	}
	target := "user/" + strconv.FormatUint(c.target, 10)
	if c.sessionID != "" {
		target += "/session/" + c.sessionID
	} else {
		target += "/sessions"
	}
	scope := domain.IdempotencyScope{ActorID: actor, Method: c.method, Target: target, Key: c.key}
	return scope, scope.Validate()
}

func (s *adminService) runAdmin(ctx context.Context, c adminCommand) (res *AdminResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = nil
		}
	}()
	p, err := selfPrincipal(ctx, c.method)
	if err != nil {
		return nil, err
	}
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return nil, err
	}
	scope, err := c.scope(p.UserID())
	if err != nil {
		return nil, err
	}
	payload := featurecommon.FingerprintPayload{ExpectedVersion: c.expected}
	if c.expected > 0 {
		payload.Presence = []string{"expected_user_version"}
	}
	hash, err := featurecommon.Fingerprint(scope, payload)
	if err != nil {
		return nil, err
	}
	idemKey, err := featurecommon.IdempotencyLockKey(scope)
	if err != nil {
		return nil, err
	}
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	keys := append([]string{idemKey, "auth:guard:policy:global"}, featurecommon.UserLockKeys([]uint64{p.UserID(), c.target})...)
	if err = adminLocks(owner, keys...); err != nil {
		return nil, err
	}
	r := s.deps.Bind(tx)
	if !r.complete() {
		return nil, common.ErrInternalServerError
	}
	now := s.deps.Now().UTC()
	p, err = lockAdminAuthority(ctx, owner, r, p, c, now)
	if err != nil {
		return nil, err
	}
	record, err := r.Idempotency.Lookup(scope)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	saved, err := featurecommon.Replay(record, hash, now)
	if err != nil {
		return nil, err
	}
	if saved != nil {
		if saved.UserID != c.target || saved.SessionID != c.sessionID || saved.Version < 1 {
			return nil, common.ErrInternalServerError
		}
		if err = owner.Commit(); err != nil {
			return nil, featurecommon.DependencyError(err)
		}
		return &AdminResult{Changed: saved.Changed, Version: saved.Version}, nil
	}
	target, err := r.Users.GetByID(ctx, c.target)
	if errors.Is(err, auth.ErrNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if target.ID != c.target || target.DeletedAt.Valid {
		return nil, auth.ErrNotFound
	}
	if target.Version < 1 || target.AuthVersion < 1 {
		return nil, common.ErrInternalServerError
	}
	var result domain.SafeCommandResult
	if c.sessionID != "" {
		result, err = s.revokeAdminSingle(ctx, r, p.UserID(), target, c.sessionID, now)
	} else {
		result, err = s.revokeAdminGlobal(ctx, r, p.UserID(), target, c.expected, now)
	}
	if err != nil {
		return nil, err
	}
	if err = r.Idempotency.Save(scope, hash, result, now); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &AdminResult{Changed: result.Changed, Version: result.Version}, nil
}

func adminLocks(owner *featurecommon.TransactionOwner, keys ...string) error {
	err := featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, keys...)
	if errors.Is(err, auth.ErrResourceBusy) {
		return err
	}
	return featurecommon.DependencyError(err)
}

func lockAdminAuthority(ctx context.Context, owner *featurecommon.TransactionOwner, r AdminRepositories, p sessions.Principal, c adminCommand, now time.Time) (sessions.Principal, error) {
	rows, err := r.Permissions.Permissions(ctx, p.UserID())
	if err != nil {
		return p, featurecommon.DependencyError(err)
	}
	keys := []string{"auth:session:" + p.SessionID()}
	for _, row := range rows {
		if row.RoleID == 0 || row.RoleApplicationID == 0 {
			return p, common.ErrInternalServerError
		}
		keys = append(keys, "auth:role:"+strconv.FormatUint(row.RoleID, 10), "auth:application:"+strconv.FormatUint(row.RoleApplicationID, 10))
	}
	if c.sessionID != "" {
		keys = append(keys, "auth:session:"+c.sessionID)
	} else {
		ids, e := r.Targets.ListIDs(ctx, c.target)
		if e != nil {
			return p, featurecommon.DependencyError(e)
		}
		for _, id := range ids {
			if domain.ValidateSessionID(id) != nil {
				return p, common.ErrInternalServerError
			}
			keys = append(keys, "auth:session:"+id)
		}
	}
	if err = adminLocks(owner, keys...); err != nil {
		return p, err
	}
	verified, err := authfeature.AuthenticateSession(ctx, p.Claims(), r.State, now)
	if err != nil {
		return p, err
	}
	p, err = sessions.PrincipalFromContext(verified)
	if err != nil {
		return p, err
	}
	if err = p.AuthorizeMethod(c.method); err != nil {
		return p, err
	}
	rows, err = r.Permissions.Permissions(ctx, p.UserID())
	if err != nil {
		return p, featurecommon.DependencyError(err)
	}
	if !featurecommon.HasPermission(featurecommon.ResolvePermissions(rows), string(types.PermissionIAMSessionRevoke)) {
		return p, common.ErrForbidden
	}
	return p, nil
}
