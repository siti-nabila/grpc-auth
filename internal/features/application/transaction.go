package application

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"strconv"
	"time"
)

type command struct {
	method      string
	permissions []types.PermissionCode
	validate    func(domain.ApplicationAccessInput) error
	apply       func(context.Context, Repositories, sessions.Principal, domain.ApplicationAccessInput, domain.ApplicationItem, time.Time) (domain.SafeCommandResult, error)
}

func (s *applicationService) run(ctx context.Context, input domain.ApplicationAccessInput, c command) (res *domain.SafeCommandResult, err error) {
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
	if err = c.validate(input); err != nil {
		return nil, err
	}
	input = input.Normalized()
	p, err := principal(ctx, c.method)
	if err != nil {
		return nil, err
	}
	var appID uint64
	var keys []string
	for attempt := 0; attempt < 3; attempt++ {
		var restart []string
		res, restart, appID, err = s.transaction(ctx, p, input, c, appID, keys)
		if err != nil || len(restart) == 0 {
			return res, err
		}
		keys = restart
	}
	return nil, usererrors.ErrResourceBusy
}

// Routing reads share the bound transaction. They determine the numeric scope;
// only the later locked authority check can permit replay or target disclosure.
func (s *applicationService) transaction(ctx context.Context, p sessions.Principal, input domain.ApplicationAccessInput, c command, appID uint64, keys []string) (res *domain.SafeCommandResult, restart []string, routedID uint64, err error) {
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, nil, appID, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	r := s.deps.Bind(tx)
	if !r.complete() {
		return nil, nil, appID, common.ErrInternalServerError
	}
	_, err = authfeature.AuthorizeCurrentPermissions(ctx, p.Claims(), c.method, c.permissions, r.State, r.Permissions, s.deps.Now().UTC())
	if err != nil {
		return nil, nil, appID, err
	}
	app, err := lookupApplication(ctx, r, input.ApplicationCode)
	if err != nil {
		return nil, nil, appID, err
	}
	if appID != 0 && appID != app.ID {
		return nil, nil, appID, common.ErrDependencyUnavailable
	}
	appID = app.ID
	scope := domain.IdempotencyScope{ActorID: p.UserID(), Method: c.method, Target: "user/" + strconv.FormatUint(input.UserID, 10) + "/application/" + strconv.FormatUint(appID, 10), Key: input.Key}
	lock, err := featurecommon.IdempotencyLockKey(scope)
	if err != nil {
		return nil, nil, appID, err
	}
	hash, err := featurecommon.Fingerprint(scope, featurecommon.FingerprintPayload{ApplicationID: appID, RoleIDs: input.RoleIDs, Reason: input.Reason, ExpectedVersion: input.ExpectedVersion, Presence: []string{"expected_user_version"}})
	if err != nil {
		return nil, nil, appID, err
	}
	keys = append(keys, lock, "auth:guard:policy:global")
	keys = append(keys, featurecommon.UserLockKeys([]uint64{p.UserID(), input.UserID})...)
	for _, id := range input.RoleIDs {
		keys = append(keys, "auth:role:"+strconv.FormatUint(id, 10))
	}
	// Application locks are discovered with contributing roles in canonical order.
	keys = featurecommon.CanonicalLockKeys(keys...)
	if err = featurecommon.AcquireLocks(owner, usererrors.ErrResourceBusy, keys...); err != nil {
		return nil, nil, appID, err
	}
	restart, err = featurecommon.LockDiscoveredAuthority(owner, keys, func() ([]string, error) { return authorityKeys(ctx, r, p, input, appID) }, usererrors.ErrResourceBusy)
	if err != nil || len(restart) > 0 {
		return nil, restart, appID, err
	}
	now := s.deps.Now().UTC()
	p, err = authfeature.AuthorizeCurrentPermissions(ctx, p.Claims(), c.method, c.permissions, r.State, r.Permissions, now)
	if err != nil {
		return nil, nil, appID, err
	}
	app, err = lookupApplication(ctx, r, input.ApplicationCode)
	if err != nil {
		return nil, nil, appID, err
	}
	if app.ID != appID {
		return nil, nil, appID, common.ErrDependencyUnavailable
	}
	record, err := r.Idempotency.Lookup(scope)
	if err != nil {
		return nil, nil, appID, featurecommon.DependencyError(err)
	}
	res, err = featurecommon.Replay(record, hash, now)
	if err != nil {
		return nil, nil, appID, err
	}
	if res == nil {
		out, e := c.apply(ctx, r, p, input, app, now)
		if e != nil {
			return nil, nil, appID, e
		}
		res = &out
		if err = res.ValidateApplicationAccessResult(input.UserID, appID); err != nil {
			return nil, nil, appID, err
		}
		if err = r.Idempotency.Save(scope, hash, *res, now); err != nil {
			return nil, nil, appID, featurecommon.DependencyError(err)
		}
	}
	if err = res.ValidateApplicationAccessResult(input.UserID, appID); err != nil {
		return nil, nil, appID, err
	}
	if err = owner.Commit(); err != nil {
		return nil, nil, appID, featurecommon.DependencyError(err)
	}
	return res, nil, appID, nil
}

func authorityKeys(ctx context.Context, r Repositories, p sessions.Principal, input domain.ApplicationAccessInput, appID uint64) ([]string, error) {
	contributions, err := r.Permissions.Permissions(ctx, p.UserID())
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	keys := []string{"auth:session:" + p.SessionID(), "auth:application:" + strconv.FormatUint(appID, 10)}
	for _, row := range contributions {
		if domain.ValidateManagementTarget(row.RoleID) != nil || domain.ValidateManagementTarget(row.RoleApplicationID) != nil {
			return nil, common.ErrDependencyUnavailable
		}
		keys = append(keys, "auth:role:"+strconv.FormatUint(row.RoleID, 10), "auth:application:"+strconv.FormatUint(row.RoleApplicationID, 10))
	}
	var rows []domain.UserRoleAssignmentItem
	if len(input.RoleIDs) > 0 {
		rows, err = r.Assignments.AssignmentRoles(ctx, input.UserID, input.RoleIDs)
	} else {
		rows, err = r.Access.ApplicationAssignments(ctx, input.UserID, appID)
	}
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	for _, row := range rows {
		if domain.ValidateManagementTarget(row.RoleID) != nil || domain.ValidateManagementTarget(row.ApplicationID) != nil {
			return nil, common.ErrDependencyUnavailable
		}
		keys = append(keys, "auth:role:"+strconv.FormatUint(row.RoleID, 10), "auth:application:"+strconv.FormatUint(row.ApplicationID, 10))
	}
	return featurecommon.CanonicalLockKeys(keys...), nil
}
