package authfeature

import (
	"context"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

// ReadUserDataSnapshot rechecks the principal and reads Me in one primary
// RepeatableRead snapshot for the default foundation server.
func ReadUserDataSnapshot(ctx context.Context, claims jwt.VerifiedClaims, provider domain.ReadSnapshotProvider, now time.Time) (res *userv1.UserData, err error) {
	defer func() {
		if recover() != nil {
			err = common.ErrInternalServerError
		}
		if err != nil {
			res = nil
		}
	}()
	if err = ctx.Err(); err != nil {
		return res, err
	}
	if _, err = serviceauth.CallerFromContext(ctx); err != nil {
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
	verified, err := AuthenticateSession(ctx, claims, reader.NewSessionStateReader(tx), now)
	if err != nil {
		return res, err
	}
	principal, err := sessions.PrincipalFromContext(verified)
	if err != nil {
		return res, err
	}
	if err = principal.AuthorizeMethod(userv1.UserService_Me_FullMethodName); err != nil {
		return res, err
	}
	self := reader.NewUserIdentityReader(tx)
	identity, err := self.Identity(ctx, principal.UserID())
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	profile, err := reader.NewProfileReaderInTransaction(tx).GetByUserId(principal.UserID())
	if err != nil {
		return res, featurecommon.DependencyError(err)
	}
	res = &userv1.UserData{Id: identity.ID, Email: identity.Email, Fullname: profile.Name}
	res.Address, res.Phone = profile.Address, profile.Phone
	res.Status, res.Version, res.MustChangePassword = string(identity.Status), uint64(identity.Version), principal.Restricted()
	if !principal.Restricted() {
		roles, readErr := self.ActiveRoles(ctx, principal.UserID())
		err = readErr
		if err != nil {
			return res, featurecommon.DependencyError(err)
		}
		for _, role := range roles {
			res.RoleCodes = append(res.RoleCodes, int32(role.Code))
			res.RoleNames = append(res.RoleNames, role.Name)
		}
	}
	if err = owner.Commit(); err != nil {
		return res, featurecommon.DependencyError(err)
	}
	return res, nil
}
