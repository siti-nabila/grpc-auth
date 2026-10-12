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
func AuthorizeCurrentPermission(ctx context.Context, claims jwt.VerifiedClaims, method string, permission types.PermissionCode, state domain.SessionStateReader, permissions domain.PermissionReader, now time.Time) error {
	verified, err := AuthenticateSession(ctx, claims, state, now)
	if err != nil {
		return err
	}
	p, err := sessions.PrincipalFromContext(verified)
	if err != nil {
		return err
	}
	if err = p.AuthorizeMethod(method); err != nil {
		return err
	}
	rows, err := permissions.Permissions(ctx, p.UserID())
	if err != nil {
		return featurecommon.DependencyError(err)
	}
	if !featurecommon.HasPermission(featurecommon.ResolvePermissions(rows), string(permission)) {
		return common.ErrForbidden
	}
	return nil
}
