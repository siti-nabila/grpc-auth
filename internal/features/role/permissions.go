package role

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *managementService) ReplaceRolePermissions(ctx context.Context, input domain.ReplaceRolePermissionsInput) (*domain.SafeCommandResult, error) {
	input = input.Normalized()
	if err := input.Validate(); err != nil {
		return nil, err
	}
	c := roleCommand{method: rolev1.RoleService_ReplaceRolePermissions_FullMethodName, permission: types.PermissionIAMRolePermissionUpdate, target: input.RoleID, key: input.Key, payload: featurecommon.FingerprintPayload{PermissionCodes: input.Permissions.Codes, Reason: input.Reason, ExpectedVersion: input.ExpectedVersion, Presence: []string{"permissions", "expected_role_version"}}}
	return s.run(ctx, c, func(ctx context.Context, r ManagementRepositories, p sessions.Principal, now time.Time) (domain.SafeCommandResult, error) {
		before, err := editableRole(ctx, r, input.RoleID, input.ExpectedVersion)
		if err != nil {
			return domain.SafeCommandResult{}, err
		}
		if err = domain.ValidatePermissionCodes(input.Permissions.Codes, string(before.ApplicationCode)); err != nil {
			return domain.SafeCommandResult{}, err
		}
		permissions, err := r.Roles.PermissionSet(ctx, before.ApplicationID, input.Permissions.Codes)
		if err != nil {
			if errors.Is(err, roleerrors.ErrInvalidPermissionSet) {
				return domain.SafeCommandResult{}, err
			}
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
		old, err := r.Roles.RolePermissions(ctx, before.ID)
		if err != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
		changed := !slices.Equal(permissionCodes(old), permissionCodes(permissions))
		after := before
		if changed {
			if before.Version == math.MaxInt64 {
				return domain.SafeCommandResult{}, common.ErrInternalServerError
			}
			if err = r.Writer.ReplacePermissions(ctx, before.ID, permissionIDs(permissions)); err != nil {
				return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
			}
			if err = advanceRole(ctx, r, &after, input.ExpectedVersion, now); err != nil {
				return domain.SafeCommandResult{}, err
			}
		}
		return s.outcome(ctx, r, p, &domain.RoleDetail{Role: before, Permissions: old}, domain.RoleDetail{Role: after, Permissions: permissions}, changed, types.AuditRolePermissionsChanged, types.EventRolePermissionsChanged, input.Reason, now)
	})
}
