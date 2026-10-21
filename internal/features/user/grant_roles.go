package user

import (
	"context"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"math"
	"time"
)

func (s *assignmentService) GrantUserRoles(ctx context.Context, input domain.UserRoleAssignmentInput) (*domain.SafeCommandResult, error) {
	return s.runAssignment(ctx, input, assignmentCommand{method: userv1.UserService_GrantUserRoles_FullMethodName, permission: types.PermissionIAMUserRoleGrant, apply: s.grantRoles})
}

func (s *assignmentService) grantRoles(ctx context.Context, r AssignmentRepositories, p sessions.Principal, input domain.UserRoleAssignmentInput, now time.Time) (domain.SafeCommandResult, error) {
	before, rows, err := assignmentBatch(ctx, r, input)
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	if before.Status != types.UserActive {
		return domain.SafeCommandResult{}, usererrors.ErrGrantTargetInactive
	}
	// Validate the entire set before the first write, including active no-ops.
	for _, row := range rows {
		if row.RoleStatus != "ACTIVE" {
			return domain.SafeCommandResult{}, roleerrors.ErrRoleDisabled
		}
		if row.ApplicationStatus != "ACTIVE" {
			return domain.SafeCommandResult{}, applicationerrors.ErrApplicationInactive
		}
		if row.ApplicationCode != string(types.ApplicationIAM) && (row.ApplicationAccessStatus == nil || *row.ApplicationAccessStatus != "ACTIVE") {
			return domain.SafeCommandResult{}, applicationerrors.ErrApplicationAccessRequired
		}
	}
	var changed []uint64
	for _, row := range rows {
		if row.Status == "ACTIVE" {
			continue
		}
		if before.Version == math.MaxInt64 {
			return domain.SafeCommandResult{}, common.ErrInternalServerError
		}
		ok, e := r.Mutation.Activate(ctx, input.UserID, row, p.UserID(), now)
		if e != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(e)
		}
		if !ok {
			return domain.SafeCommandResult{}, usererrors.ErrVersionConflict
		}
		changed = append(changed, row.RoleID)
	}
	return s.finishAssignment(ctx, r, p, input, before, changed, types.AuditUserRolesGranted, types.EventUserRolesGranted, now)
}
