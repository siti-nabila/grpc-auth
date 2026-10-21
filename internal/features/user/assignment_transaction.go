package user

import (
	"context"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"strconv"
	"time"
)

type assignmentCommand struct {
	method     string
	permission types.PermissionCode
	apply      func(context.Context, AssignmentRepositories, sessions.Principal, domain.UserRoleAssignmentInput, time.Time) (domain.SafeCommandResult, error)
}

func assignmentPrincipal(ctx context.Context, method string) (sessions.Principal, error) {
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return sessions.Principal{}, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return sessions.Principal{}, err
	}
	p, err := sessions.PrincipalFromContext(ctx)
	if err != nil {
		return p, err
	}
	return p, p.AuthorizeMethod(method)
}

func (s *assignmentService) runAssignment(ctx context.Context, input domain.UserRoleAssignmentInput, c assignmentCommand) (result *domain.SafeCommandResult, err error) {
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
	if err = input.Validate(); err != nil {
		return nil, err
	}
	input = input.Normalized()
	p, err := assignmentPrincipal(ctx, c.method)
	if err != nil {
		return nil, err
	}
	scope := domain.IdempotencyScope{ActorID: p.UserID(), Method: c.method, Target: "user/" + strconv.FormatUint(input.UserID, 10), Key: input.Key}
	lock, err := featurecommon.IdempotencyLockKey(scope)
	if err != nil {
		return nil, err
	}
	hash, err := featurecommon.Fingerprint(scope, featurecommon.FingerprintPayload{RoleIDs: input.RoleIDs, Reason: input.Reason, ExpectedVersion: input.ExpectedVersion, Presence: []string{"expected_user_version"}})
	if err != nil {
		return nil, err
	}
	keys := append([]string{lock, "auth:guard:policy:global"}, featurecommon.UserLockKeys([]uint64{p.UserID(), input.UserID})...)
	if c.method == userv1.UserService_RevokeUserRoles_FullMethodName {
		keys = append(keys, "auth:guard:last-admin:global")
	}
	for _, id := range input.RoleIDs {
		keys = append(keys, "auth:role:"+strconv.FormatUint(id, 10))
	}
	for attempt := 0; attempt < 3; attempt++ {
		var restart []string
		result, restart, err = s.assignmentTransaction(ctx, p, input, c, scope, hash, keys)
		if err != nil || len(restart) == 0 {
			return result, err
		}
		keys = featurecommon.CanonicalLockKeys(append(keys, restart...)...)
	}
	return nil, usererrors.ErrResourceBusy
}

func (s *assignmentService) assignmentTransaction(ctx context.Context, p sessions.Principal, input domain.UserRoleAssignmentInput, c assignmentCommand, scope domain.IdempotencyScope, hash []byte, keys []string) (result *domain.SafeCommandResult, restart []string, err error) {
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
	restart, err = featurecommon.LockDiscoveredAuthority(owner, keys, func() ([]string, error) { return assignmentAuthorityKeys(ctx, r, p, input) }, usererrors.ErrResourceBusy)
	if err != nil || len(restart) > 0 {
		return nil, restart, err
	}
	now := s.deps.Now().UTC()
	p, err = authfeature.AuthorizeCurrentPermission(ctx, p.Claims(), c.method, c.permission, r.State, r.Permissions, now)
	if err != nil {
		return nil, nil, err
	}
	result, err = replayAssignment(r, input, scope, hash, now)
	if err != nil {
		return nil, nil, err
	}
	if result == nil {
		result, err = executeAssignment(ctx, r, p, input, c, scope, hash, now)
		if err != nil {
			return nil, nil, err
		}
	}
	if err = owner.Commit(); err != nil {
		return nil, nil, featurecommon.DependencyError(err)
	}
	return result, nil, nil
}

func replayAssignment(r AssignmentRepositories, input domain.UserRoleAssignmentInput, scope domain.IdempotencyScope, hash []byte, now time.Time) (*domain.SafeCommandResult, error) {
	record, err := r.Idempotency.Lookup(scope)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	result, err := featurecommon.Replay(record, hash, now)
	if err != nil {
		return nil, err
	}
	if result != nil {
		if err = result.ValidateAssignmentResult(input.UserID); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func executeAssignment(ctx context.Context, r AssignmentRepositories, p sessions.Principal, input domain.UserRoleAssignmentInput, c assignmentCommand, scope domain.IdempotencyScope, hash []byte, now time.Time) (*domain.SafeCommandResult, error) {
	out, err := c.apply(ctx, r, p, input, now)
	if err != nil {
		return nil, err
	}
	if err = out.ValidateAssignmentResult(input.UserID); err != nil {
		return nil, err
	}
	if err = r.Idempotency.Save(scope, hash, out, now); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &out, nil
}

func assignmentAuthorityKeys(ctx context.Context, r AssignmentRepositories, p sessions.Principal, input domain.UserRoleAssignmentInput) ([]string, error) {
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
	roles, err := r.Assignments.AssignmentRoles(ctx, input.UserID, input.RoleIDs)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	for _, row := range roles {
		if domain.ValidateManagementTarget(row.RoleID) != nil || domain.ValidateManagementTarget(row.ApplicationID) != nil {
			return nil, common.ErrDependencyUnavailable
		}
		keys = append(keys, "auth:role:"+strconv.FormatUint(row.RoleID, 10), "auth:application:"+strconv.FormatUint(row.ApplicationID, 10))
	}
	return featurecommon.CanonicalLockKeys(keys...), nil
}
