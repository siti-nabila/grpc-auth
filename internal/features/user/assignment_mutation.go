package user

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"math"
	"strconv"
	"time"
)

func assignmentTarget(ctx context.Context, r AssignmentRepositories, id uint64) (domain.SessionUserState, error) {
	before, err := r.Users.GetByID(ctx, id)
	if errors.Is(err, auth.ErrNotFound) {
		return before, auth.ErrNotFound
	}
	if err != nil {
		return before, featurecommon.DependencyError(err)
	}
	if before.ID != id || before.DeletedAt.Valid {
		return before, auth.ErrNotFound
	}
	if before.Version < 1 || !domain.ManagementUserStatus(before.Status) {
		return before, common.ErrDependencyUnavailable
	}
	return before, nil
}

func assignmentBatch(ctx context.Context, r AssignmentRepositories, input domain.UserRoleAssignmentInput) (domain.SessionUserState, []domain.UserRoleAssignmentItem, error) {
	before, err := assignmentTarget(ctx, r, input.UserID)
	if err != nil {
		return before, nil, err
	}
	if before.Version != input.ExpectedVersion {
		return before, nil, usererrors.ErrVersionConflict
	}
	rows, err := r.Assignments.AssignmentRoles(ctx, input.UserID, input.RoleIDs)
	if err != nil {
		return before, nil, featurecommon.DependencyError(err)
	}
	if len(rows) < len(input.RoleIDs) {
		return before, nil, roleerrors.ErrRoleNotFound
	}
	if len(rows) != len(input.RoleIDs) {
		return before, nil, common.ErrDependencyUnavailable
	}
	for i, row := range rows {
		if !validAssignmentRole(row, input.RoleIDs[i]) {
			return before, nil, common.ErrDependencyUnavailable
		}
	}
	return before, rows, nil
}

func (s *assignmentService) finishAssignment(ctx context.Context, r AssignmentRepositories, p sessions.Principal, input domain.UserRoleAssignmentInput, before domain.SessionUserState, changed []uint64, action types.AuditAction, event types.EventType, now time.Time) (domain.SafeCommandResult, error) {
	result := domain.SafeCommandResult{UserID: input.UserID, Version: before.Version, Changed: len(changed) > 0, ChangedRoleIDs: changed}
	if result.Changed {
		if err := advanceAssignment(ctx, r, input.UserID, before.Version, now); err != nil {
			return domain.SafeCommandResult{}, err
		}
		result.Version++
	} else {
		action = types.AuditNoOp
	}
	return s.assignmentEvidence(ctx, r, p, input, before, result, action, event, now)
}

func advanceAssignment(ctx context.Context, r AssignmentRepositories, userID uint64, version int64, now time.Time) error {
	if version == math.MaxInt64 {
		return common.ErrInternalServerError
	}
	ok, err := r.Mutation.AdvanceVersion(ctx, userID, version, now)
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if !ok {
		return usererrors.ErrVersionConflict
	}
	return nil
}

func (s *assignmentService) assignmentEvidence(ctx context.Context, r AssignmentRepositories, p sessions.Principal, input domain.UserRoleAssignmentInput, before domain.SessionUserState, result domain.SafeCommandResult, action types.AuditAction, event types.EventType, now time.Time) (domain.SafeCommandResult, error) {
	changed := result.ChangedRoleIDs
	actor, err := domain.ActorForVerifiedUser(p.UserID())
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(input.UserID, 10), Before: &domain.SafeSnapshot{Version: before.Version}, After: &domain.SafeSnapshot{Version: result.Version, RoleIDs: changed, NoOp: !result.Changed}, Reason: types.AuditReason(input.Reason), OccurredAt: now}); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if result.Changed {
		id, e := s.deps.EventID()
		if e != nil {
			if canceled := ctx.Err(); canceled != nil {
				return domain.SafeCommandResult{}, canceled
			}
			return domain.SafeCommandResult{}, errors.Join(common.ErrInternalServerError, e)
		}
		actorID := p.UserID()
		envelope := featurecommon.UserEvent(ctx, id, event, input.UserID, result.Version, &actorID, "", now)
		envelope.Data.ChangedRoleIDs = changed
		if err = r.Outbox.Create(envelope); err != nil {
			return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
		}
	}
	return result, nil
}

func validAssignmentRole(row domain.UserRoleAssignmentItem, roleID uint64) bool {
	if row.RoleID != roleID || domain.ValidateManagementTarget(row.ApplicationID) != nil || row.Code == "" {
		return false
	}
	if row.ID == 0 {
		return row.Status == ""
	}
	if domain.ValidateManagementTarget(row.ID) != nil {
		return false
	}
	return row.Status == "ACTIVE" || row.Status == "REVOKED"
}
