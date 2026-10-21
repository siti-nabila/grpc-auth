package user

import (
	"context"
	"errors"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
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

func ReadMyEffectivePermissionsSnapshot(ctx context.Context, claims jwt.VerifiedClaims, input domain.MyEffectivePermissionsInput, provider domain.ReadSnapshotProvider, now time.Time) (res domain.EffectivePermissionsResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = domain.EffectivePermissionsResult{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return res, err
	}
	if err = input.Validate(); err != nil {
		return res, err
	}
	input = input.Normalized()
	caller, err := effectivePermissionCaller(ctx, input.ApplicationCode)
	if err != nil {
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
	if err = p.AuthorizeMethod(userv1.UserService_GetMyEffectivePermissions_FullMethodName); err != nil {
		return res, err
	}
	res, err = effectivePermissionResult(ctx, tx, p.UserID(), input.ApplicationCode, caller)
	if err != nil {
		return res, err
	}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}

func ReadUserEffectivePermissionsSnapshot(ctx context.Context, claims jwt.VerifiedClaims, input domain.UserEffectivePermissionsInput, provider domain.ReadSnapshotProvider, now time.Time) (res domain.EffectivePermissionsResult, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = domain.EffectivePermissionsResult{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return res, err
	}
	if err = input.Validate(); err != nil {
		return res, err
	}
	input = input.Normalized()
	caller, err := effectivePermissionCaller(ctx, input.ApplicationCode)
	if err != nil {
		return res, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
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
	if _, err = authfeature.AuthorizeCurrentPermission(ctx, claims, userv1.UserService_GetUserEffectivePermissions_FullMethodName, types.PermissionIAMUserRead, reader.NewSessionStateReader(tx), reader.NewPermissionReader(tx), now); err != nil {
		return res, err
	}
	res, err = effectivePermissionResult(ctx, tx, input.UserID, input.ApplicationCode, caller)
	if err != nil {
		return res, err
	}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}

func effectivePermissionCaller(ctx context.Context, application string) (serviceauth.Caller, error) {
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return caller, err
	}
	if application != "" {
		if err = caller.AuthorizeNamespace(application); err != nil {
			return caller, err
		}
	}
	return caller, nil
}

func effectivePermissionResult(ctx context.Context, tx domain.Transaction, id uint64, application string, caller serviceauth.Caller) (domain.EffectivePermissionsResult, error) {
	target, err := reader.NewSessionUserReader(tx).GetByID(ctx, id)
	if errors.Is(err, auth.ErrNotFound) {
		return domain.EffectivePermissionsResult{}, auth.ErrNotFound
	}
	if err != nil {
		return domain.EffectivePermissionsResult{}, featurecommon.DependencyError(err)
	}
	if target.ID != id || target.DeletedAt.Valid {
		return domain.EffectivePermissionsResult{}, auth.ErrNotFound
	}
	if target.Version < 1 || !domain.ManagementUserStatus(target.Status) {
		return domain.EffectivePermissionsResult{}, common.ErrDependencyUnavailable
	}
	rows, err := reader.NewPermissionReader(tx).Permissions(ctx, id)
	if err != nil {
		return domain.EffectivePermissionsResult{}, featurecommon.DependencyError(err)
	}
	visible := make([]domain.PermissionContribution, 0, len(rows))
	for _, row := range rows {
		code := string(row.ApplicationCode)
		if application != "" && code != application {
			continue
		}
		if caller.AuthorizeNamespace(code) == nil {
			visible = append(visible, row)
		}
	}
	return domain.EffectivePermissionsResult{UserID: target.ID, UserStatus: target.Status, Version: target.Version, Permissions: featurecommon.ResolvePermissions(visible)}, nil
}
