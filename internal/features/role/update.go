package role

import (
	"context"
	"time"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *managementService) UpdateRole(ctx context.Context, input domain.UpdateRoleInput) (*domain.SafeCommandResult, error) {
	input = input.Normalized()
	if err := input.Validate(); err != nil {
		return nil, err
	}
	c := roleCommand{method: rolev1.RoleService_UpdateRole_FullMethodName, permission: types.PermissionIAMRoleUpdate, target: input.RoleID, key: input.Key, payload: featurecommon.FingerprintPayload{Name: input.Name, Description: input.Description, ExpectedVersion: input.ExpectedVersion, Presence: append([]string{"expected_role_version"}, input.Fields...)}}
	return s.run(ctx, c, func(ctx context.Context, r ManagementRepositories, p sessions.Principal, now time.Time) (domain.SafeCommandResult, error) {
		before, err := editableRole(ctx, r, input.RoleID, input.ExpectedVersion)
		if err != nil {
			return domain.SafeCommandResult{}, err
		}
		after := before
		if input.Name != nil {
			after.Name = *input.Name
		}
		if input.Description != nil {
			after.Description = clearedDescription(input.Description)
		}
		changed := after.Name != before.Name || !sameDescription(before.Description, after.Description)
		if changed {
			if err = advanceRole(ctx, r, &after, input.ExpectedVersion, now); err != nil {
				return domain.SafeCommandResult{}, err
			}
		}
		return s.outcome(ctx, r, p, &domain.RoleDetail{Role: before}, domain.RoleDetail{Role: after}, changed, types.AuditRoleUpdated, types.EventRoleUpdated, "", now)
	})
}
