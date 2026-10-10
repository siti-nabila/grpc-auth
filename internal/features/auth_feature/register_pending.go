package authfeature

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *pendingRegistrationService) Register(ctx context.Context, req domain.AuthRequest) (res *RegistrationResult, err error) {
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
	if req.Id != 0 {
		return nil, common.ErrBadRequest
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if err = req.Validate(); err != nil {
		return nil, err
	}
	hash, err := s.deps.HashPassword(req.Password)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if hash == "" {
		return nil, common.ErrInternalServerError
	}
	tx, err := s.deps.Transactions.Begin(ctx)
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, featurecommon.RegisterLockKeys(req.Email)...); err != nil {
		if errors.Is(err, auth.ErrResourceBusy) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	repos := s.deps.Bind(tx)
	if repos.Account == nil || repos.Role == nil || repos.Auth == nil || repos.Profile == nil || repos.Member == nil || repos.Audit == nil || repos.Outbox == nil {
		return nil, common.ErrInternalServerError
	}
	if _, err = repos.Account.GetByEmail(req.Email); err == nil {
		return nil, auth.ErrDataExists
	} else if !errors.Is(err, auth.ErrNotFound) {
		return nil, featurecommon.DependencyError(err)
	}
	member, err := repos.Role.DefaultMember()
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if member == 0 {
		return nil, common.ErrDependencyUnavailable
	}
	account := &domain.PendingRegistration{Email: req.Email, Password: hash, Status: types.UserPending, Version: 1, AuthVersion: 1}
	if err = repos.Auth.Create(account); err != nil {
		if errors.Is(err, auth.ErrDataExists) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	if account.ID == 0 || account.ID > 2147483647 {
		return nil, common.ErrInternalServerError
	}
	if err = repos.Profile.Create(&domain.ProfileRequest{UserId: account.ID}); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err = repos.Member.Assign(account.ID, member); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	actor, err := domain.ActorForVerifiedService(caller.ID())
	if err != nil {
		return nil, err
	}
	now := s.deps.Now().UTC()
	if err = repos.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditUserRegistered, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(account.ID, 10), After: &domain.SafeSnapshot{Version: 1, Status: string(types.UserPending), RoleIDs: []uint64{member}}, OccurredAt: now}); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	id, err := s.deps.EventID()
	if err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err = repos.Outbox.Create(featurecommon.UserEvent(ctx, id, types.EventUserCreated, account.ID, 1, nil, types.UserPending, now)); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &RegistrationResult{ID: account.ID, Status: types.UserPending}, nil
}
