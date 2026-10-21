package application

import (
	"context"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
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

func (s *applicationService) GrantApplicationAccess(ctx context.Context, input domain.ApplicationAccessInput) (*domain.SafeCommandResult, error) {
	return s.run(ctx, input, command{method: applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName, permissions: []types.PermissionCode{types.PermissionIAMUserApplicationGrant, types.PermissionIAMUserRoleGrant}, validate: domain.ApplicationAccessInput.ValidateGrant, apply: s.grant})
}

func (s *applicationService) grant(ctx context.Context, r Repositories, p sessions.Principal, input domain.ApplicationAccessInput, app domain.ApplicationItem, now time.Time) (domain.SafeCommandResult, error) {
	before, access, err := mutationTarget(ctx, r, input, app)
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	if before.Status != types.UserActive {
		return domain.SafeCommandResult{}, usererrors.ErrGrantTargetInactive
	}
	if app.Status != types.ApplicationActive {
		return domain.SafeCommandResult{}, applicationerrors.ErrApplicationInactive
	}
	rows, err := r.Assignments.AssignmentRoles(ctx, input.UserID, input.RoleIDs)
	if err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if len(rows) < len(input.RoleIDs) {
		return domain.SafeCommandResult{}, roleerrors.ErrRoleNotFound
	}
	if len(rows) != len(input.RoleIDs) {
		return domain.SafeCommandResult{}, common.ErrDependencyUnavailable
	}
	for i, row := range rows {
		if !validRole(row, input.RoleIDs[i]) {
			return domain.SafeCommandResult{}, common.ErrDependencyUnavailable
		}
		if row.ApplicationID != app.ID {
			return domain.SafeCommandResult{}, common.ErrBadRequest
		}
		if row.RoleStatus != "ACTIVE" {
			return domain.SafeCommandResult{}, roleerrors.ErrRoleDisabled
		}
	}
	changed := access == nil || access.Status != "ACTIVE"
	for _, row := range rows {
		changed = changed || row.Status != "ACTIVE"
	}
	if changed && before.Version == math.MaxInt64 {
		return domain.SafeCommandResult{}, common.ErrInternalServerError
	}
	if access == nil || access.Status != "ACTIVE" {
		ok, e := r.AccessMutation.Activate(ctx, input.UserID, app.ID, access, p.UserID(), now)
		if err = writeResult(ok, e); err != nil {
			return domain.SafeCommandResult{}, err
		}
	}
	var ids []uint64
	for _, row := range rows {
		if row.Status == "ACTIVE" {
			continue
		}
		ok, e := r.Mutation.Activate(ctx, input.UserID, row, p.UserID(), now)
		if err = writeResult(ok, e); err != nil {
			return domain.SafeCommandResult{}, err
		}
		ids = append(ids, row.RoleID)
	}
	result := domain.SafeCommandResult{UserID: input.UserID, ApplicationID: app.ID, Status: "ACTIVE", Version: before.Version, Changed: changed, ChangedRoleIDs: ids}
	return s.finish(ctx, r, p, input, app, before, access, result, types.AuditApplicationAccessGranted, types.EventUserApplicationAccessGranted, now)
}
