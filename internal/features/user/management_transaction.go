package user

import (
	"context"
	"errors"
	"strconv"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type managementCommand struct {
	method       string
	permission   types.PermissionCode
	target       uint64
	account, key string
	payload      featurecommon.FingerprintPayload
	password     []byte
}

type managementApply func(context.Context, ManagementRepositories, sessions.Principal, uint64, time.Time) (domain.SafeCommandResult, error)

func (s *managementService) runManagement(ctx context.Context, c managementCommand, apply managementApply) (res *domain.SafeCommandResult, err error) {
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
	scope := domain.IdempotencyScope{ActorID: p.UserID(), Method: c.method, Target: "user/" + strconv.FormatUint(c.target, 10), Key: c.key}
	if c.account != "" {
		scope.Target = "account/" + c.account
	}
	keys := append([]string{"auth:guard:policy:global"}, featurecommon.UserLockKeys([]uint64{p.UserID()})...)
	if c.target != 0 {
		keys = append(keys, featurecommon.UserLockKey(c.target))
	}
	if c.account != "" {
		keys = append(keys, featurecommon.RegisterLockKeys(c.account)...)
	}
	if c.method == userv1.UserService_ChangeUserStatus_FullMethodName {
		keys = append(keys, "auth:guard:last-admin:global")
	}
	var hash []byte
	if c.key != "" {
		lock, e := featurecommon.IdempotencyLockKey(scope)
		if e != nil {
			return nil, e
		}
		keys = append(keys, lock)
		if len(c.password) > 0 {
			hash, err = s.deps.Fingerprinter.PasswordFingerprint(scope, c.payload, c.password)
		} else {
			hash, err = featurecommon.Fingerprint(scope, c.payload)
		}
		if err != nil {
			return nil, err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		var discovered []string
		res, discovered, err = s.managementTransaction(ctx, c, p, scope, hash, keys, apply)
		if err != nil || len(discovered) == 0 {
			return res, err
		}
		keys = featurecommon.CanonicalLockKeys(append(keys, discovered...)...)
	}
	return nil, usererrors.ErrResourceBusy
}

func (s *managementService) managementTransaction(ctx context.Context, c managementCommand, p sessions.Principal, scope domain.IdempotencyScope, hash []byte, keys []string, apply managementApply) (result *domain.SafeCommandResult, restart []string, err error) {
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = managementLocks(owner, keys...); err != nil {
		return nil, nil, err
	}
	r := s.deps.Bind(tx)
	if !r.complete() {
		return nil, nil, common.ErrInternalServerError
	}
	member := uint64(0)
	if c.account != "" {
		member, err = r.Role.DefaultMember()
		if err != nil {
			return nil, nil, featurecommon.DependencyError(err)
		}
		if domain.ValidateManagementTarget(member) != nil {
			return nil, nil, common.ErrDependencyUnavailable
		}
	}
	restart, err = featurecommon.LockDiscoveredAuthority(owner, keys, func() ([]string, error) { return managementAuthorityKeys(ctx, r, p, c.target, member) }, usererrors.ErrResourceBusy)
	if err != nil || len(restart) > 0 {
		return nil, restart, err
	}
	now := s.deps.Now().UTC()
	p, err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), c.method, c.permission, r.State, r.Permissions, now)
	if err != nil {
		return nil, nil, err
	}
	result, err = s.replayManagement(r, c, scope, hash, now)
	if err != nil {
		return nil, nil, err
	}
	if result == nil {
		result, err = s.executeManagement(ctx, r, p, c, member, scope, hash, now, apply)
		if err != nil {
			return nil, nil, err
		}
	}
	if err = owner.Commit(); err != nil {
		return nil, nil, featurecommon.DependencyError(err)
	}
	return result, nil, nil
}

func (s *managementService) replayManagement(r ManagementRepositories, c managementCommand, scope domain.IdempotencyScope, hash []byte, now time.Time) (*domain.SafeCommandResult, error) {
	if c.key == "" {
		return nil, nil
	}
	record, err := r.Idempotency.Lookup(scope)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if len(c.password) > 0 {
		return s.deps.Fingerprinter.ReplayPassword(record, scope, c.payload, c.password, now)
	}
	return featurecommon.Replay(record, hash, now)
}

func (s *managementService) executeManagement(ctx context.Context, r ManagementRepositories, p sessions.Principal, c managementCommand, member uint64, scope domain.IdempotencyScope, hash []byte, now time.Time, apply managementApply) (*domain.SafeCommandResult, error) {
	out, err := apply(ctx, r, p, member, now)
	if err != nil {
		return nil, err
	}
	if c.key != "" {
		if err = r.Idempotency.Save(scope, hash, out, now); err != nil {
			return nil, featurecommon.DependencyError(err)
		}
	}
	return &out, nil
}

func managementLocks(owner *featurecommon.TransactionOwner, keys ...string) error {
	err := featurecommon.AcquireLocks(owner, usererrors.ErrResourceBusy, keys...)
	if errors.Is(err, usererrors.ErrResourceBusy) {
		return err
	}
	return featurecommon.DependencyError(err)
}

func managementAuthorityKeys(ctx context.Context, r ManagementRepositories, p sessions.Principal, target, member uint64) ([]string, error) {
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
	if member != 0 {
		app, e := r.Policy.RoleApplication(ctx, member)
		if e != nil {
			return nil, featurecommon.DependencyError(e)
		}
		if domain.ValidateManagementTarget(app) != nil {
			return nil, common.ErrDependencyUnavailable
		}
		keys = append(keys, "auth:role:"+strconv.FormatUint(member, 10), "auth:application:"+strconv.FormatUint(app, 10))
	}
	if target != 0 {
		ids, e := r.Targets.ListIDs(ctx, target)
		if e != nil {
			return nil, featurecommon.DependencyError(e)
		}
		for _, id := range ids {
			if domain.ValidateSessionID(id) != nil {
				return nil, common.ErrDependencyUnavailable
			}
			keys = append(keys, "auth:session:"+id)
		}
	}
	return featurecommon.CanonicalLockKeys(keys...), nil
}
