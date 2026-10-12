package role

import (
	"context"
	"errors"
	"time"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func (s *managementService) CreateRole(ctx context.Context, input domain.CreateRoleInput) (*domain.SafeCommandResult, error) {
	input = input.Normalized()
	if err := input.Validate(); err != nil {
		return nil, err
	}
	presence := []string{"name", "permission_codes"}
	if input.Description != nil {
		presence = append(presence, "description")
	}
	c := roleCommand{method: rolev1.RoleService_CreateRole_FullMethodName, permission: types.PermissionIAMRoleCreate, code: input.Code, application: input.ApplicationCode, key: input.Key, payload: featurecommon.FingerprintPayload{Code: input.Code, Name: &input.Name, Description: input.Description, PermissionCodes: input.PermissionCodes, Presence: presence}}
	return s.run(ctx, c, func(ctx context.Context, r ManagementRepositories, p sessions.Principal, now time.Time) (domain.SafeCommandResult, error) {
		app, err := r.Roles.Application(ctx, input.ApplicationCode)
		if err != nil {
			if errors.Is(err, applicationerrors.ErrApplicationNotFound) {
				return domain.SafeCommandResult{}, err
			}
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
		if err = app.AllowCustomRole(); err != nil {
			return domain.SafeCommandResult{}, err
		}
		permissions, err := r.Roles.PermissionSet(ctx, app.ID, input.PermissionCodes)
		if err != nil {
			if errors.Is(err, roleerrors.ErrInvalidPermissionSet) {
				return domain.SafeCommandResult{}, err
			}
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
		role, err := r.Writer.CreateRole(ctx, domain.RoleSummary{Code: input.Code, Name: input.Name, Description: clearedDescription(input.Description), ApplicationID: app.ID, ApplicationCode: app.Code, Status: types.RoleActive, Version: 1})
		if err != nil {
			if errors.Is(err, roleerrors.ErrRoleCodeAlreadyExists) {
				return domain.SafeCommandResult{}, err
			}
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
		if err = r.Writer.ReplacePermissions(ctx, role.ID, permissionIDs(permissions)); err != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
		detail := domain.RoleDetail{Role: role, Permissions: permissions}
		return s.outcome(ctx, r, p, nil, detail, true, types.AuditRoleCreated, types.EventRoleCreated, "", now)
	})
}
