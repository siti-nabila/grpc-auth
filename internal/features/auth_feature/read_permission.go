package authfeature

import (
	"context"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// AuthorizeCurrentPermission evaluates current user/session and exact permission
// using readers bound by the owning feature to its primary snapshot or locked transaction.
func AuthorizeCurrentPermission(ctx context.Context, claims jwt.VerifiedClaims, method string, permission types.PermissionCode, state domain.SessionStateReader, permissions domain.PermissionReader, now time.Time) (sessions.Principal, error) {
	return AuthorizeCurrentPermissions(ctx, claims, method, []types.PermissionCode{permission}, state, permissions, now)
}

// AuthorizeCurrentPermissions requires every exact permission from one current read.
func AuthorizeCurrentPermissions(ctx context.Context, claims jwt.VerifiedClaims, method string, required []types.PermissionCode, state domain.SessionStateReader, permissions domain.PermissionReader, now time.Time) (sessions.Principal, error) {
	if len(required) == 0 {
		return sessions.Principal{}, common.ErrInternalServerError
	}
	verified, err := AuthenticateSession(ctx, claims, state, now)
	if err != nil {
		return sessions.Principal{}, err
	}
	p, err := sessions.PrincipalFromContext(verified)
	if err != nil {
		return sessions.Principal{}, err
	}
	if err = p.AuthorizeMethod(method); err != nil {
		return sessions.Principal{}, err
	}
	rows, err := permissions.Permissions(ctx, p.UserID())
	if err != nil {
		return sessions.Principal{}, featurecommon.DependencyError(err)
	}
	resolved := featurecommon.ResolvePermissions(rows)
	for _, permission := range required {
		if !featurecommon.HasPermission(resolved, string(permission)) {
			return sessions.Principal{}, common.ErrForbidden
		}
	}
	return p, nil
}
