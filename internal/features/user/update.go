package user

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *managementService) UpdateUser(ctx context.Context, input domain.UpdateUserInput) (*domain.SafeCommandResult, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	payload := featurecommon.FingerprintPayload{Name: input.Name, Address: input.Address, Phone: input.Phone, CountryCode: input.CountryCode, ExpectedVersion: input.ExpectedVersion, Presence: append([]string{"expected_user_version"}, input.Fields...)}
	c := managementCommand{method: userv1.UserService_UpdateUser_FullMethodName, permission: types.PermissionIAMUserUpdate, target: input.UserID, key: input.Key, payload: payload}
	return s.runManagement(ctx, c, func(ctx context.Context, r ManagementRepositories, p sessions.Principal, _ uint64, now time.Time) (domain.SafeCommandResult, error) {
		return s.updateUser(ctx, r, p, input, now)
	})
}

func (s *managementService) updateUser(ctx context.Context, r ManagementRepositories, p sessions.Principal, input domain.UpdateUserInput, now time.Time) (domain.SafeCommandResult, error) {
	before, err := r.Detail.Detail(ctx, input.UserID)
	if errors.Is(err, auth.ErrNotFound) {
		return domain.SafeCommandResult{}, auth.ErrNotFound
	}
	if err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if before.Version != input.ExpectedVersion {
		return domain.SafeCommandResult{}, usererrors.ErrVersionConflict
	}
	after := updatedUserProfile(before, input)
	changed := after.Name != before.Name || after.Address != before.Address || after.Phone != before.Phone || after.CountryCode != before.CountryCode
	action := types.AuditNoOp
	if changed {
		if err = persistManagedProfile(ctx, r, after, input, now); err != nil {
			return domain.SafeCommandResult{}, err
		}

		after.Version++
		action = types.AuditUserProfileUpdated
	}
	actor, err := domain.ActorForVerifiedUser(p.UserID())
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	afterSnapshot := managementProfileSnapshot(after)
	afterSnapshot.NoOp = !changed
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(input.UserID, 10), Before: managementProfileSnapshot(before), After: afterSnapshot, OccurredAt: now}); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if changed {
		if err = s.managementEvent(ctx, r, p.UserID(), input.UserID, after.Version, types.EventUserProfileUpdated, after.Status, nil, now); err != nil {
			return domain.SafeCommandResult{}, err
		}
	}
	return domain.UserDetailCommandResult(after, changed), nil
}

func managementProfileSnapshot(r domain.UserDetail) *domain.SafeSnapshot {
	return &domain.SafeSnapshot{Version: r.Version, Name: &r.Name, Address: &r.Address, Phone: &r.Phone, CountryCode: &r.CountryCode}
}

func updatedUserProfile(before domain.UserDetail, input domain.UpdateUserInput) domain.UserDetail {
	after := before
	if input.Name != nil {
		after.Name = *input.Name
	}
	if input.Address != nil {
		after.Address = *input.Address
	}
	if input.Phone != nil {
		after.Phone = *input.Phone
	}
	if input.CountryCode != nil {
		after.CountryCode = *input.CountryCode
	}
	return after
}

func persistManagedProfile(ctx context.Context, r ManagementRepositories, after domain.UserDetail, input domain.UpdateUserInput, now time.Time) error {
	if after.Version == math.MaxInt64 {
		return common.ErrInternalServerError
	}
	ok, err := r.ProfileMutation.AdvanceProfileVersion(ctx, input.UserID, input.ExpectedVersion, now)
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if !ok {
		return usererrors.ErrVersionConflict
	}
	ok, err = r.ProfileMutation.UpdateProfile(ctx, input.UserID, after)
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if !ok {
		return auth.ErrNotFound
	}
	return nil
}
