package user

import (
	"context"
	"errors"
	"strconv"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *managementService) CreateUser(ctx context.Context, input domain.CreateUserInput) (*domain.SafeCommandResult, error) {
	input = input.Normalized()
	if err := input.Validate(); err != nil {
		return nil, err
	}
	payload := featurecommon.FingerprintPayload{Email: input.Email, Name: &input.Name, Address: &input.Address, Phone: &input.Phone, CountryCode: input.CountryCode, Status: string(*input.Status), Presence: []string{"status", "name", "address", "phone"}}
	c := managementCommand{method: userv1.UserService_CreateUser_FullMethodName, permission: types.PermissionIAMUserCreate, account: input.Email, key: input.Key, payload: payload, password: []byte(input.Password)}
	return s.runManagement(ctx, c, func(ctx context.Context, r ManagementRepositories, p sessions.Principal, member uint64, now time.Time) (domain.SafeCommandResult, error) {
		return s.createUser(ctx, r, p, member, input, now)
	})
}

func (s *managementService) createUser(ctx context.Context, r ManagementRepositories, p sessions.Principal, member uint64, input domain.CreateUserInput, now time.Time) (domain.SafeCommandResult, error) {
	if _, err := r.Account.GetByEmail(input.Email); err == nil {
		return domain.SafeCommandResult{}, auth.ErrDataExists
	} else if !errors.Is(err, auth.ErrNotFound) {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	// Replay has already returned before expensive hashing or any domain write.
	hash, err := s.deps.HashPassword(input.Password)
	if err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if hash == "" || len(hash) > 150 {
		return domain.SafeCommandResult{}, common.ErrInternalServerError
	}
	account := &domain.PendingRegistration{Email: input.Email, Password: hash, Status: *input.Status, Version: 1, AuthVersion: 1, MustChangePassword: true}
	if err = r.Create.Create(account); err != nil {
		if errors.Is(err, auth.ErrDataExists) {
			return domain.SafeCommandResult{}, auth.ErrDataExists
		}
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if domain.ValidateManagementTarget(account.ID) != nil {
		return domain.SafeCommandResult{}, common.ErrInternalServerError
	}
	if err = r.Profile.Create(&domain.ProfileRequest{UserId: account.ID, Name: input.Name, Address: input.Address, Phone: input.Phone, CountryCode: domain.NullableCountryCode(input.CountryCode)}); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if err = r.Member.Assign(account.ID, member); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	detail, err := r.Detail.Detail(ctx, account.ID)
	if err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	actor, err := domain.ActorForVerifiedUser(p.UserID())
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: types.AuditUserCreated, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(account.ID, 10), After: &domain.SafeSnapshot{Version: 1, Status: string(*input.Status), RoleIDs: []uint64{member}}, OccurredAt: now}); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if err = s.managementEvent(ctx, r, p.UserID(), account.ID, 1, types.EventUserCreated, *input.Status, nil, now); err != nil {
		return domain.SafeCommandResult{}, err
	}
	return domain.UserDetailCommandResult(detail, true), nil
}

func (s *managementService) managementEvent(ctx context.Context, r ManagementRepositories, actor, target uint64, version int64, event types.EventType, status types.UserStatus, revoked *bool, now time.Time) error {
	id, err := s.deps.EventID()
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	envelope := featurecommon.UserEvent(ctx, id, event, target, version, &actor, status, now)
	envelope.Data.SessionsRevoked = revoked
	return featurecommon.DependencyError(r.Outbox.Create(envelope))
}
