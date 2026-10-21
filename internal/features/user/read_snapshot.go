package user

import (
	"context"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/orm/orm"
)

// ReadUsersSnapshot prepares the protected consumer without activating an
// incompatible JWT pipeline before the real Login producer is ready.
func ReadUsersSnapshot(ctx context.Context, claims jwt.VerifiedClaims, req domain.UserListRequest, provider domain.ReadSnapshotProvider, now time.Time) (res orm.PageData[domain.UserSearchRow], err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = orm.PageData[domain.UserSearchRow]{}
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
	if provider == nil {
		return res, common.ErrDependencyUnavailable
	}
	tx, err := provider.BeginSnapshot(ctx)
	if err != nil {
		return res, err
	}
	owner := featurecommon.NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if _, err = authfeature.AuthorizeCurrentPermission(ctx, claims, userv1.UserService_ListUsers_FullMethodName, types.PermissionIAMUserRead, reader.NewSessionStateReader(tx), reader.NewPermissionReader(tx), now); err != nil {
		return res, err
	}
	service := &userService{ctx: ctx, userReader: reader.NewUserReaderInSnapshot(ctx, tx)}
	res, err = service.SearchUsers(req)
	if err != nil {
		return res, err
	}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}
