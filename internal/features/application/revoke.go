package application

import (
	"context"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"math"
	"time"
)

func (s *applicationService) RevokeApplicationAccess(ctx context.Context, input domain.ApplicationAccessInput) (*domain.SafeCommandResult, error) {
	return s.run(ctx, input, command{method: applicationv1.ApplicationService_RevokeApplicationAccess_FullMethodName, permissions: []types.PermissionCode{types.PermissionIAMUserApplicationRevoke, types.PermissionIAMUserRoleRevoke}, validate: domain.ApplicationAccessInput.ValidateRevoke, apply: s.revoke})
}

func (s *applicationService) revoke(ctx context.Context, r Repositories, p sessions.Principal, input domain.ApplicationAccessInput, app domain.ApplicationItem, now time.Time) (domain.SafeCommandResult, error) {
	before, access, err := mutationTarget(ctx, r, input, app)
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	rows, err := r.Access.ApplicationAssignments(ctx, input.UserID, app.ID)
	if err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	for i, row := range rows {
		if !validRole(row, row.RoleID) || row.ID == 0 || row.ApplicationID != app.ID || row.Status != "ACTIVE" || i > 0 && row.RoleID <= rows[i-1].RoleID {
			return domain.SafeCommandResult{}, common.ErrDependencyUnavailable
		}
	}
	changed := len(rows) > 0 || access != nil && access.Status == "ACTIVE"
	if changed && before.Version == math.MaxInt64 {
		return domain.SafeCommandResult{}, common.ErrInternalServerError
	}
	status := ""
	if access != nil {
		status = "REVOKED"
		if access.Status == "ACTIVE" {
			ok, e := r.AccessMutation.Revoke(ctx, input.UserID, *access, p.UserID(), input.Reason, now)
			if err = writeResult(ok, e); err != nil {
				return domain.SafeCommandResult{}, err
			}
		}
	}
	var ids []uint64
	for _, row := range rows {
		ok, e := r.Mutation.Revoke(ctx, input.UserID, row, p.UserID(), input.Reason, now)
		if err = writeResult(ok, e); err != nil {
			return domain.SafeCommandResult{}, err
		}
		ids = append(ids, row.RoleID)
	}
	result := domain.SafeCommandResult{UserID: input.UserID, ApplicationID: app.ID, Status: status, Version: before.Version, Changed: changed, ChangedRoleIDs: ids}
	return s.finish(ctx, r, p, input, app, before, access, result, types.AuditApplicationAccessRevoked, types.EventUserApplicationAccessRevoked, now)
}
