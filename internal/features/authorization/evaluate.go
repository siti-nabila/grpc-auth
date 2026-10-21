package authorization

import (
	"context"
	"errors"
	"time"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func EvaluateAccessSnapshot(ctx context.Context, claims jwt.VerifiedClaims, input domain.EvaluateAccessInput, provider domain.ReadSnapshotProvider, now time.Time) (res domain.AccessDecision, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = domain.AccessDecision{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return res, err
	}
	if err = input.Validate(); err != nil {
		return res, err
	}
	input = input.Normalized()
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return res, err
	}
	if err = caller.AuthorizeNamespace(input.ApplicationCode); err != nil {
		return res, err
	}
	if provider == nil {
		return res, common.ErrDependencyUnavailable
	}
	tx, err := provider.BeginSnapshot(ctx)
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	verified, err := authfeature.AuthenticateSession(ctx, claims, reader.NewSessionStateReader(tx), now)
	if err != nil {
		return res, err
	}
	p, err := sessions.PrincipalFromContext(verified)
	if err != nil {
		return res, err
	}
	if err = p.AuthorizeMethod(authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName); err != nil {
		return res, err
	}
	reason, err := evaluatePermission(ctx, tx, p.UserID(), input)
	if err != nil {
		return res, err
	}
	res = domain.AccessDecision{Allowed: reason == types.DecisionAllowed, Reason: reason, UserID: p.UserID(), SessionID: p.SessionID(), Version: p.UserVersion(), EvaluatedAt: p.EvaluatedAt().UTC()}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}

func evaluatePermission(ctx context.Context, tx domain.Transaction, userID uint64, input domain.EvaluateAccessInput) (types.AccessDecisionReason, error) {
	app, err := reader.NewRoleManagementReader(tx).Application(ctx, input.ApplicationCode)
	if errors.Is(err, applicationerrors.ErrApplicationNotFound) {
		return types.DecisionPermissionMissing, nil
	}
	if err != nil {
		return types.DecisionUnspecified, featurecommon.DependencyError(err)
	}
	if string(app.Code) != input.ApplicationCode || app.Name == "" || (app.Status != "ACTIVE" && app.Status != "DISABLED") {
		return types.DecisionUnspecified, common.ErrDependencyUnavailable
	}
	if app.Status == "DISABLED" {
		return types.DecisionApplicationInactive, nil
	}
	if input.ApplicationCode != string(types.ApplicationIAM) {
		access, err := reader.NewApplicationAccessReader(tx).Access(ctx, userID, app.ID)
		if err != nil {
			return types.DecisionUnspecified, featurecommon.DependencyError(err)
		}
		if access != nil && (access.ApplicationCode != string(app.Code) || access.ApplicationStatus != string(app.Status)) {
			return types.DecisionUnspecified, common.ErrDependencyUnavailable
		}
		if access == nil || access.Status != "ACTIVE" {
			return types.DecisionApplicationAccessRequired, nil
		}
	}
	rows, err := reader.NewPermissionReader(tx).Permissions(ctx, userID)
	if err != nil {
		return types.DecisionUnspecified, featurecommon.DependencyError(err)
	}
	visible := make([]domain.PermissionContribution, 0, len(rows))
	for _, row := range rows {
		if row.RoleApplicationID == app.ID && row.PermissionApplicationID == app.ID && string(row.ApplicationCode) == input.ApplicationCode {
			visible = append(visible, row)
		}
	}
	if featurecommon.HasPermission(featurecommon.ResolvePermissions(visible), input.PermissionCode) {
		return types.DecisionAllowed, nil
	}
	return types.DecisionPermissionMissing, nil
}
