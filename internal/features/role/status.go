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

func (s *managementService) ChangeRoleStatus(ctx context.Context, input domain.ChangeRoleStatusInput) (*domain.SafeCommandResult, error) {
	input = input.Normalized()
	if err := input.Validate(); err != nil {
		return nil, err
	}
	c := roleCommand{method: rolev1.RoleService_ChangeRoleStatus_FullMethodName, permission: types.PermissionIAMRoleUpdate, target: input.RoleID, key: input.Key, payload: featurecommon.FingerprintPayload{Status: string(*input.Status), Reason: input.Reason, ExpectedVersion: input.ExpectedVersion, Presence: []string{"status", "expected_role_version"}}}
	return s.run(ctx, c, func(ctx context.Context, r ManagementRepositories, p sessions.Principal, now time.Time) (domain.SafeCommandResult, error) {
		before, err := editableRole(ctx, r, input.RoleID, input.ExpectedVersion)
		if err != nil {
			return domain.SafeCommandResult{}, err
		}
		after := before
		after.Status = *input.Status
		changed := after.Status != before.Status
		if changed {
			if err = advanceRole(ctx, r, &after, input.ExpectedVersion, now); err != nil {
				return domain.SafeCommandResult{}, err
			}
		}
		return s.outcome(ctx, r, p, &domain.RoleDetail{Role: before}, domain.RoleDetail{Role: after}, changed, types.AuditRoleUpdated, types.EventRoleUpdated, input.Reason, now)
	})
}
