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

// ReadUserDetailSnapshot keeps authority and target data in one primary snapshot.
func ReadUserDetailSnapshot(ctx context.Context, claims jwt.VerifiedClaims, id uint64, provider domain.ReadSnapshotProvider, now time.Time) (res domain.UserDetail, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = domain.UserDetail{}
		}
	}()
	if err = ctx.Err(); err != nil {
		return res, err
	}
	caller, err := serviceauth.CallerFromContext(ctx)
	if err != nil {
		return res, err
	}
	if err = caller.AuthorizeNamespace(string(types.ApplicationIAM)); err != nil {
		return res, err
	}
	if err = domain.ValidateManagementTarget(id); err != nil {
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
	if err = p.AuthorizeMethod(userv1.UserService_GetUser_FullMethodName); err != nil {
		return res, err
	}
	rows, err := reader.NewPermissionReader(tx).Permissions(ctx, p.UserID())
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	if !featurecommon.HasPermission(featurecommon.ResolvePermissions(rows), string(types.PermissionIAMUserRead)) {
		return res, common.ErrForbidden
	}
	res, err = reader.NewUserDetailReader(tx).Detail(ctx, id)
	if errors.Is(err, auth.ErrNotFound) {
		return res, auth.ErrNotFound
	}
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}
