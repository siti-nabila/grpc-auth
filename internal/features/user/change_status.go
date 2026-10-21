package user

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
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

func (s *managementService) ChangeUserStatus(ctx context.Context, input domain.ChangeUserStatusInput) (*domain.SafeCommandResult, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if err := input.Validate(); err != nil {
		return nil, err
	}
	payload := featurecommon.FingerprintPayload{Status: string(*input.Status), Reason: input.Reason, ExpectedVersion: input.ExpectedVersion, Presence: []string{"status", "expected_user_version"}}
	c := managementCommand{method: userv1.UserService_ChangeUserStatus_FullMethodName, permission: types.PermissionIAMUserStatusUpdate, target: input.UserID, key: input.Key, payload: payload}
	return s.runManagement(ctx, c, func(ctx context.Context, r ManagementRepositories, p sessions.Principal, _ uint64, now time.Time) (domain.SafeCommandResult, error) {
		return s.changeUserStatus(ctx, r, p, input, now)
	})
}

func (s *managementService) changeUserStatus(ctx context.Context, r ManagementRepositories, p sessions.Principal, input domain.ChangeUserStatusInput, now time.Time) (domain.SafeCommandResult, error) {
	before, err := r.Users.GetByID(ctx, input.UserID)
	if errors.Is(err, auth.ErrNotFound) {
		return domain.SafeCommandResult{}, auth.ErrNotFound
	}
	if err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if before.ID != input.UserID || before.DeletedAt.Valid {
		return domain.SafeCommandResult{}, auth.ErrNotFound
	}
	if before.Version != input.ExpectedVersion {
		return domain.SafeCommandResult{}, usererrors.ErrVersionConflict
	}
	status := *input.Status
	if !domain.UserStatusTransitionAllowed(before.Status, status) {
		return domain.SafeCommandResult{}, usererrors.ErrInvalidStatusTransition
	}
	invalidate := status == types.UserSuspended || status == types.UserDeactivated
	if invalidate && input.UserID == p.UserID() {
		return domain.SafeCommandResult{}, usererrors.ErrSelfInactivationForbidden
	}
	changed := before.Status != status
	result := domain.SafeCommandResult{UserID: input.UserID, Version: before.Version, Status: string(status), Changed: changed}
	action := types.AuditNoOp
	if changed {
		if before.Version < 1 || before.Version == math.MaxInt64 || before.AuthVersion < 1 || invalidate && before.AuthVersion == math.MaxInt64 {
			return domain.SafeCommandResult{}, common.ErrInternalServerError
		}
		if invalidate && before.Status == types.UserActive {
			count, admin, e := r.Policy.AdminCount(ctx, input.UserID)
			if e != nil {
				return domain.SafeCommandResult{}, featurecommon.DependencyError(e)
			}
			if admin && count <= 1 {
				return domain.SafeCommandResult{}, usererrors.ErrLastAdminRequired
			}
		}
		ok, e := r.StatusMutation.ChangeStatus(ctx, before, status, invalidate, now)
		if e != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(e)
		}
		if !ok {
			return domain.SafeCommandResult{}, usererrors.ErrVersionConflict
		}
		if invalidate {
			if _, e = r.Sessions.RevokeUserSessions(ctx, domain.SessionRevocation{UserID: input.UserID, ActorID: p.UserID(), At: now, Reason: types.AuditUserStatusChange}); e != nil {
				return domain.SafeCommandResult{}, featurecommon.DependencyError(e)
			}
			if _, e = r.Refresh.RevokeUserRefresh(ctx, input.UserID, now); e != nil {
				return domain.SafeCommandResult{}, featurecommon.DependencyError(e)
			}
		}
		result.Version++
		action = types.AuditUserStatusChanged
	}
	actor, err := domain.ActorForVerifiedUser(p.UserID())
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(input.UserID, 10), Before: &domain.SafeSnapshot{Version: before.Version, Status: string(before.Status)}, After: &domain.SafeSnapshot{Version: result.Version, Status: string(status), NoOp: !changed}, Reason: types.AuditReason(input.Reason), OccurredAt: now}); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if changed {
		if err = s.managementEvent(ctx, r, p.UserID(), input.UserID, result.Version, types.EventUserStatusChanged, status, &invalidate, now); err != nil {
			return domain.SafeCommandResult{}, err
		}
	}
	return result, nil
}
