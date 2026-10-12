package role

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func editableRole(ctx context.Context, r ManagementRepositories, id uint64, version int64) (domain.RoleSummary, error) {
	role, err := r.Roles.Role(ctx, id)
	if err != nil {
		if errors.Is(err, roleerrors.ErrRoleNotFound) {
			return role, err
		}
		return domain.RoleSummary{}, featurecommon.DependencyError(err)
	}
	if err = role.Protect(); err != nil {
		return domain.RoleSummary{}, err
	}
	app, err := r.Roles.Application(ctx, string(role.ApplicationCode))
	if err != nil {
		return domain.RoleSummary{}, featurecommon.DependencyError(err)
	}
	if app.ID != role.ApplicationID {
		return domain.RoleSummary{}, common.ErrDependencyUnavailable
	}
	if app.IsSystem || app.Code == types.ApplicationIAM {
		return domain.RoleSummary{}, applicationerrors.ErrSystemApplicationProtected
	}
	if role.Version != version {
		return domain.RoleSummary{}, roleerrors.ErrVersionConflict
	}
	return role, nil
}

func advanceRole(ctx context.Context, r ManagementRepositories, role *domain.RoleSummary, expected int64, now time.Time) error {
	if expected == math.MaxInt64 {
		return common.ErrInternalServerError
	}
	role.Version = expected + 1
	role.UpdatedAt = &now
	ok, err := r.Writer.UpdateRole(ctx, *role, expected)
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if !ok {
		return roleerrors.ErrVersionConflict
	}
	return nil
}

func clearedDescription(value *string) *string {
	if value == nil || *value == "" {
		return nil
	}
	text := *value
	return &text
}
func sameDescription(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
func permissionIDs(rows []domain.PermissionItem) []uint64 {
	ids := make([]uint64, 0, len(rows))
	for _, p := range rows {
		ids = append(ids, p.ID)
	}
	return ids
}
func permissionCodes(rows []domain.PermissionItem) []string {
	codes := make([]string, 0, len(rows))
	for _, p := range rows {
		codes = append(codes, p.Code)
	}
	return domain.NormalizePermissionCodes(codes)
}

func roleSnapshot(detail domain.RoleDetail) *domain.SafeSnapshot {
	return &domain.SafeSnapshot{Version: detail.Role.Version, Status: string(detail.Role.Status), Name: &detail.Role.Name, Description: detail.Role.Description, ApplicationID: detail.Role.ApplicationID, PermissionCodes: permissionCodes(detail.Permissions)}
}

func (s *managementService) outcome(ctx context.Context, r ManagementRepositories, p sessions.Principal, before *domain.RoleDetail, after domain.RoleDetail, changed bool, action types.AuditAction, event types.EventType, reason string, now time.Time) (domain.SafeCommandResult, error) {
	result := domain.RoleCommandResult(after, changed)
	if result.Validate() != nil {
		return domain.SafeCommandResult{}, common.ErrInternalServerError
	}
	actor, err := domain.ActorForVerifiedUser(p.UserID())
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	var previous *domain.SafeSnapshot
	if before != nil {
		previous = roleSnapshot(*before)
	}
	next := roleSnapshot(after)
	next.NoOp = !changed
	if !changed {
		action = types.AuditNoOp
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetRole, TargetID: strconv.FormatUint(after.Role.ID, 10), Before: previous, After: next, Reason: types.AuditReason(reason), OccurredAt: now}); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if changed {
		id, err := s.deps.EventID()
		if err != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
		envelope := featurecommon.RoleEvent(ctx, id, event, after.Role, p.UserID(), permissionCodes(after.Permissions), now)
		if err = r.Outbox.Create(envelope); err != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
	}
	return result, nil
}
