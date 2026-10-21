package authfeature

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

// AuthenticateSession validates primary state. Read/mutation consumers repeat
// this check inside their snapshot or after locks; it is not a lasting grant.
func AuthenticateSession(ctx context.Context, claims jwt.VerifiedClaims, reader domain.SessionStateReader, now time.Time) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return ctx, err
	}
	if _, err := serviceauth.CallerFromContext(ctx); err != nil {
		return ctx, err
	}
	if reader == nil {
		return ctx, common.ErrDependencyUnavailable
	}
	state, err := reader.GetSessionState(ctx, claims.SessionID())
	if err != nil {
		if errors.Is(err, auth.ErrNotFound) {
			return ctx, auth.ErrInvalidSession
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ctx, err
		}
		return ctx, fmt.Errorf("%w: %w", common.ErrDependencyUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return ctx, err
	}
	principal, err := sessions.PrincipalForState(claims, state, now)
	if err != nil {
		return ctx, err
	}
	return principal.Context(ctx), nil
}
