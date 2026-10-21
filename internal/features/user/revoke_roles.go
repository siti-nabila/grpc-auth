package user

import (
	"context"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"math"
	"time"
)

func (s *assignmentService) RevokeUserRoles(ctx context.Context, input domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error) {
	return s.runAssignment(ctx, input, assignmentCommand{method: userv1.UserService_RevokeUserRoles_FullMethodName, permission: types.PermissionIAMUserRoleRevoke, apply: s.revokeRoles})
}

func (s *assignmentService) revokeRoles(ctx context.Context, r AssignmentRepositories, p sessions.Principal, input domain.UserRoleAssignmentInput, now time.Time) (domain.SafeCommandResult, error) {
	before, rows, err := assignmentBatch(ctx, r, input)
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	for _, row := range rows {
		if row.Code != string(types.RoleIAMPlatformAdmin) {
			continue
		}
		if input.UserID == p.UserID() {
			return domain.SafeCommandResult{}, usererrors.ErrSelfAdminRoleRevokeForbidden
		}
		if row.Status == "ACTIVE" {
			count, admin, e := r.Policy.AdminCount(ctx, input.UserID)
			if e != nil {
				return domain.SafeCommandResult{}, featurecommon.DependencyError(e)
			}
			if admin && count <= 1 {
				return domain.SafeCommandResult{}, usererrors.ErrLastAdminRequired
			}
		}
	}
	var changed []uint64
	for _, row := range rows {
		if row.Status != "ACTIVE" {
			continue
		}
		if before.Version == math.MaxInt64 {
			return domain.SafeCommandResult{}, common.ErrInternalServerError
		}
		ok, e := r.Mutation.Revoke(ctx, input.UserID, row, p.UserID(), input.Reason, now)
		if e != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(e)
		}
		if !ok {
			return domain.SafeCommandResult{}, usererrors.ErrVersionConflict
		}
		changed = append(changed, row.RoleID)
	}
	return s.finishAssignment(ctx, r, p, input, before, changed, types.AuditUserRolesRevoked, types.EventUserRolesRevoked, now)
}
