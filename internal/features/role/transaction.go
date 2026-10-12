package role

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type roleCommand struct {
	method                 string
	permission             types.PermissionCode
	target                 uint64
	code, application, key string
	payload                featurecommon.FingerprintPayload
}
type roleApply func(context.Context, ManagementRepositories, sessions.Principal, time.Time) (domain.SafeCommandResult, error)

func (s *managementService) run(ctx context.Context, c roleCommand, apply roleApply) (result *domain.SafeCommandResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			result = nil
		}
	}()
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return nil, err
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = p.AuthorizeMethod(c.method); err != nil {
		return nil, err
	}
	scope := domain.IdempotencyScope{ActorID: p.UserID(), Method: c.method, Target: "role/" + strconv.FormatUint(c.target, 10), Key: c.key}
	keys := []string{"auth:guard:policy:global", featurecommon.UserLockKey(p.UserID())}
	if c.code != "" {
		scope.Target = "role-code/" + c.code
		digest := sha256.Sum256([]byte(c.code))
		keys = append(keys, fmt.Sprintf("auth:role-code:%x", digest))
	}
	if c.target != 0 {
		keys = append(keys, "auth:role:"+strconv.FormatUint(c.target, 10))
	}
	var hash []byte
	if c.key != "" {
		lock, e := featurecommon.IdempotencyLockKey(scope)
		if e != nil {
			return nil, e
		}
		keys = append(keys, lock)
		hash, err = featurecommon.Fingerprint(scope, c.payload)
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		var restart []string
		result, restart, err = s.commandTransaction(ctx, c, p, scope, hash, keys, apply)
		if err != nil || len(restart) == 0 {
			return result, err
		}
		keys = featurecommon.CanonicalLockKeys(append(keys, restart...)...)
	}
	return nil, roleerrors.ErrResourceBusy
}

func (s *managementService) commandTransaction(ctx context.Context, c roleCommand, p sessions.Principal, scope domain.IdempotencyScope, hash []byte, keys []string, apply roleApply) (result *domain.SafeCommandResult, restart []string, err error) {
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, nil, featurecommon.DependencyError(err)
	}
	if tx == nil {
		return nil, nil, common.ErrDependencyUnavailable
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = roleLocks(owner, keys...); err != nil {
		return nil, nil, err
	}
	r := s.deps.Bind(tx)
	if !r.complete() {
		return nil, nil, common.ErrDependencyUnavailable
	}
	held := featurecommon.CanonicalLockKeys(keys...)
	for check := 0; check < 3; check++ {
		discovered, e := roleAuthorityKeys(ctx, r, p, c)
		if e != nil {
			return nil, nil, e
		}
		missing := []string{}
		for _, key := range discovered {
			if !slices.Contains(held, key) {
				missing = append(missing, key)
			}
		}
		if featurecommon.RequiresLockRestart(held, missing) {
			return nil, append(held, missing...), nil
		}
		if err = roleLocks(owner, missing...); err != nil {
			return nil, nil, err
		}
		held = featurecommon.CanonicalLockKeys(append(held, missing...)...)
		next, e := roleAuthorityKeys(ctx, r, p, c)
		if e != nil {
			return nil, nil, e
		}
		if slices.Equal(discovered, next) {
			break
		}
		if check == 2 {
			return nil, append(held, next...), nil
		}
	}
	now := s.deps.Now().UTC()
	if err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), c.method, c.permission, r.State, r.Permissions, now); err != nil {
		return nil, nil, err
	}
	if c.key != "" {
		record, e := r.Idempotency.Lookup(scope)
		if e != nil {
			return nil, nil, featurecommon.DependencyError(e)
		}
		result, err = featurecommon.Replay(record, hash, now)
		if errors.Is(err, usererrors.ErrIdempotencyConflict) {
			return nil, nil, roleerrors.ErrIdempotencyConflict
		}
		if err != nil {
			return nil, nil, err
		}
		if result != nil {
			if result.Role == nil || c.target != 0 && result.RoleID != c.target || c.code != "" && result.Role.Code != c.code {
				return nil, nil, common.ErrDependencyUnavailable
			}
			if err = owner.Commit(); err != nil {
				return nil, nil, featurecommon.DependencyError(err)
			}
			return result, nil, nil
		}
	}
	out, err := apply(ctx, r, p, now)
	if err != nil {
		return nil, nil, err
	}
	if out.Validate() != nil {
		return nil, nil, common.ErrInternalServerError
	}
	if c.key != "" {
		if err = r.Idempotency.Save(scope, hash, out, now); err != nil {
			return nil, nil, featurecommon.DependencyError(err)
		}
	}
	if err = owner.Commit(); err != nil {
		return nil, nil, featurecommon.DependencyError(err)
	}
	return &out, nil, nil
}

func roleLocks(owner *featurecommon.TransactionOwner, keys ...string) error {
	err := featurecommon.AcquireLocks(owner, roleerrors.ErrResourceBusy, keys...)
	if errors.Is(err, roleerrors.ErrResourceBusy) {
		return err
	}
	return featurecommon.DependencyError(err)
}

func roleAuthorityKeys(ctx context.Context, r ManagementRepositories, p sessions.Principal, c roleCommand) ([]string, error) {
	rows, err := r.Permissions.Permissions(ctx, p.UserID())
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	keys := []string{"auth:session:" + p.SessionID()}
	for _, row := range rows {
		if domain.ValidateManagementTarget(row.RoleID) != nil || domain.ValidateManagementTarget(row.RoleApplicationID) != nil {
			return nil, common.ErrDependencyUnavailable
		}
		keys = append(keys, "auth:role:"+strconv.FormatUint(row.RoleID, 10), "auth:application:"+strconv.FormatUint(row.RoleApplicationID, 10))
	}
	if c.target != 0 {
		role, err := r.Roles.Role(ctx, c.target)
		if err != nil && !errors.Is(err, roleerrors.ErrRoleNotFound) {
			return nil, featurecommon.DependencyError(err)
		}
		if err == nil {
			keys = append(keys, "auth:application:"+strconv.FormatUint(role.ApplicationID, 10))
		}
	}
	if c.application != "" {
		app, err := r.Roles.Application(ctx, c.application)
		if err != nil && !errors.Is(err, applicationerrors.ErrApplicationNotFound) {
			return nil, featurecommon.DependencyError(err)
		}
		if err == nil {
			keys = append(keys, "auth:application:"+strconv.FormatUint(app.ID, 10))
		}
	}
	return featurecommon.CanonicalLockKeys(keys...), nil
}
