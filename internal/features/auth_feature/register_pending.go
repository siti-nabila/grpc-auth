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
	req, hash, err := s.prepareRegistration(req)
	if err != nil {
		return nil, err
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
	member, err := registrationMember(repos, req.Email)
	if err != nil {
		return nil, err
	}
	account, err := createPendingAccount(repos, req, hash, member)
	if err != nil {
		return nil, err
	}
	if err = s.registrationEvidence(ctx, repos, caller.ID(), account.ID, member); err != nil {
		return nil, err
	}
	if err = owner.Commit(); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return &RegistrationResult{ID: account.ID, Status: types.UserPending}, nil
}

func (s *pendingRegistrationService) prepareRegistration(req domain.AuthRequest) (domain.AuthRequest, string, error) {
	if req.Id != 0 {
		return req, "", common.ErrBadRequest
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if err := req.Validate(); err != nil {
		return req, "", err
	}
	hash, err := s.deps.HashPassword(req.Password)
	if err != nil {
		return req, "", featurecommon.DependencyError(err)
	}
	if hash == "" {
		return req, "", common.ErrInternalServerError
	}
	return req, hash, nil
}

func registrationMember(repos PendingRegistrationRepositories, email string) (uint64, error) {
	if _, err := repos.Account.GetByEmail(email); err == nil {
		return 0, auth.ErrDataExists
	} else if !errors.Is(err, auth.ErrNotFound) {
		return 0, featurecommon.DependencyError(err)
	}
	member, err := repos.Role.DefaultMember()
	if err != nil {
		return 0, featurecommon.DependencyError(err)
	}
	if member == 0 {
		return 0, common.ErrDependencyUnavailable
	}
	return member, nil
}

func createPendingAccount(repos PendingRegistrationRepositories, req domain.AuthRequest, hash string, member uint64) (*domain.PendingRegistration, error) {
	account := &domain.PendingRegistration{Email: req.Email, Password: hash, Status: types.UserPending, Version: 1, AuthVersion: 1}
	if err := repos.Auth.Create(account); err != nil {
		if errors.Is(err, auth.ErrDataExists) {
			return nil, err
		}
		return nil, featurecommon.DependencyError(err)
	}
	if account.ID == 0 || account.ID > 2147483647 {
		return nil, common.ErrInternalServerError
	}
	if err := repos.Profile.Create(&domain.ProfileRequest{UserId: account.ID}); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	if err := repos.Member.Assign(account.ID, member); err != nil {
		return nil, featurecommon.DependencyError(err)
	}
	return account, nil
}

func (s *pendingRegistrationService) registrationEvidence(ctx context.Context, repos PendingRegistrationRepositories, callerID string, accountID, member uint64) error {
	actor, err := domain.ActorForVerifiedService(callerID)
	if err != nil {
		return err
	}
	now := s.deps.Now().UTC()
	if err = repos.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditUserRegistered, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(accountID, 10), After: &domain.SafeSnapshot{Version: 1, Status: string(types.UserPending), RoleIDs: []uint64{member}}, OccurredAt: now}); err != nil {
		return featurecommon.DependencyError(err)
	}
	id, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if err = repos.Outbox.Create(featurecommon.UserEvent(ctx, id, types.EventUserCreated, accountID, 1, nil, types.UserPending, now)); err != nil {
		return featurecommon.DependencyError(err)
	}
	return nil
}
