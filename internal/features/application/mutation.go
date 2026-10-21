package application

import (
	"context"
	"errors"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"strconv"
	"time"
)

func lookupApplication(ctx context.Context, r Repositories, code string) (domain.ApplicationItem, error) {
	app, err := r.Applications.Application(ctx, code)
	if errors.Is(err, applicationerrors.ErrApplicationNotFound) {
		return app, err
	}
	if err != nil {
		return app, featurecommon.DependencyError(err)
	}
	if domain.ValidateManagementTarget(app.ID) != nil || string(app.Code) != code || app.Name == "" || app.Status != types.ApplicationActive && app.Status != types.ApplicationDisabled {
		return app, common.ErrDependencyUnavailable
	}
	return app, nil
}

func mutationTarget(ctx context.Context, r Repositories, input domain.ApplicationAccessInput, app domain.ApplicationItem) (domain.SessionUserState, *domain.ApplicationAccessItem, error) {
	before, err := target(ctx, r, input.UserID)
	if err != nil {
		return before, nil, err
	}
	if before.Version != input.ExpectedVersion {
		return before, nil, usererrors.ErrVersionConflict
	}
	if app.IsSystem || app.Code == types.ApplicationIAM {
		return before, nil, applicationerrors.ErrApplicationAccessSystemProtected
	}
	access, err := r.Access.Access(ctx, input.UserID, app.ID)
	if err != nil {
		return before, nil, featurecommon.DependencyError(err)
	}
	if access != nil && (access.UserID != input.UserID || access.ApplicationID != app.ID || access.Status != "ACTIVE" && access.Status != "REVOKED") {
		return before, nil, common.ErrDependencyUnavailable
	}
	return before, access, nil
}

func validRole(row domain.UserRoleAssignmentItem, id uint64) bool {
	if domain.ValidateManagementTarget(id) != nil || row.RoleID != id || domain.ValidateManagementTarget(row.ApplicationID) != nil || row.Code == "" || row.RoleStatus != "ACTIVE" && row.RoleStatus != "DISABLED" {
		return false
	}
	if row.ID == 0 {
		return row.Status == ""
	}
	return domain.ValidateManagementTarget(row.ID) == nil && (row.Status == "ACTIVE" || row.Status == "REVOKED")
}

func writeResult(ok bool, err error) error {
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if !ok {
		return usererrors.ErrVersionConflict
	}
	return nil
}

func (s *applicationService) finish(ctx context.Context, r Repositories, p sessions.Principal, input domain.ApplicationAccessInput, app domain.ApplicationItem, before domain.SessionUserState, access *domain.ApplicationAccessItem, result domain.SafeCommandResult, action types.AuditAction, event types.EventType, now time.Time) (domain.SafeCommandResult, error) {
	if result.Changed {
		ok, err := r.Mutation.AdvanceVersion(ctx, input.UserID, before.Version, now)
		if err = writeResult(ok, err); err != nil {
			return domain.SafeCommandResult{}, err
		}
		result.Version++
	} else {
		action = types.AuditNoOp
	}
	actor, err := domain.ActorForVerifiedUser(p.UserID())
	if err != nil {
		return domain.SafeCommandResult{}, err
	}
	beforeStatus := ""
	if access != nil {
		beforeStatus = access.Status
	}
	if err = r.Audit.Create(domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetUser, TargetID: strconv.FormatUint(input.UserID, 10), Before: &domain.SafeSnapshot{Version: before.Version, ApplicationID: app.ID, Status: beforeStatus}, After: &domain.SafeSnapshot{Version: result.Version, ApplicationID: app.ID, Status: result.Status, RoleIDs: result.ChangedRoleIDs, NoOp: !result.Changed}, Reason: types.AuditReason(input.Reason), OccurredAt: now}); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	if !result.Changed {
		return result, nil
	}
	id, err := s.deps.EventID()
	if err != nil {
		if canceled := ctx.Err(); canceled != nil {
			return domain.SafeCommandResult{}, canceled
		}
		return domain.SafeCommandResult{}, errors.Join(common.ErrInternalServerError, err)
	}
	actorID := p.UserID()
	envelope := featurecommon.UserEvent(ctx, id, event, input.UserID, result.Version, &actorID, "", now)
	envelope.Data.ApplicationID = app.ID
	envelope.Data.ApplicationCode = app.Code
	envelope.Data.ChangedRoleIDs = result.ChangedRoleIDs
	envelope.Data.Status = result.Status
	if envelope.Data.Status == "" {
		envelope.Data.Status = "REVOKED"
	}
	if err = r.Outbox.Create(envelope); err != nil {
		return domain.SafeCommandResult{}, featurecommon.DependencyError(err)
	}
	return result, nil
}
