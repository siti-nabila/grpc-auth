package sessions

import (
	"context"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

const (
	restrictedChangePasswordMethod = authv1.AuthService_ChangePassword_FullMethodName
	restrictedLogoutMethod         = authv1.AuthService_Logout_FullMethodName
)

type principalKey struct{}

type Principal struct {
	claims      jwt.VerifiedClaims
	restricted  bool
	userVersion int64
	evaluatedAt time.Time
}

func (p Principal) UserID() uint64         { return p.claims.UserID() }
func (p Principal) SessionID() string      { return p.claims.SessionID() }
func (p Principal) TokenID() string        { return p.claims.TokenID() }
func (p Principal) AuthVersion() int64     { return p.claims.AuthVersion() }
func (p Principal) Restricted() bool       { return p.restricted }
func (p Principal) UserVersion() int64     { return p.userVersion }
func (p Principal) EvaluatedAt() time.Time { return p.evaluatedAt }

// Claims returns an immutable value for a fresh feature snapshot or locked
// recheck. It does not turn the earlier principal into a lasting grant.
func (p Principal) Claims() jwt.VerifiedClaims { return p.claims }

// PrincipalForState is evaluated again on the feature's read snapshot or after
// mutation locks; an earlier principal is not a permanent authorization grant.
func PrincipalForState(claims jwt.VerifiedClaims, state domain.SessionState, now time.Time) (Principal, error) {
	if claims.UserID() == 0 || claims.AuthVersion() < 1 || state.UserVersion < 1 {
		return Principal{}, auth.ErrInvalidSession
	}
	if state.UserID != claims.UserID() || state.SessionUserID != claims.UserID() || state.SessionID != claims.SessionID() {
		return Principal{}, auth.ErrInvalidSession
	}
	if state.UserStatus != types.UserActive || state.SessionStatus != types.SessionActive || state.DeletedAt.Valid || state.RevokedAt.Valid || state.AuthVersion != claims.AuthVersion() {
		return Principal{}, auth.ErrInvalidSession
	}
	if state.CreatedAt.IsZero() || state.CreatedAt.After(now) || !state.ExpiresAt.After(state.CreatedAt) || state.ExpiresAt.Sub(state.CreatedAt) > jwt.MaxSessionLifetime {
		return Principal{}, auth.ErrInvalidSession
	}
	if !now.Before(state.ExpiresAt) || claims.ExpiresAt().After(state.ExpiresAt) {
		return Principal{}, auth.ErrInvalidSession
	}
	if !now.Before(claims.ExpiresAt().Add(jwt.AccessClockSkew)) || claims.IssuedAt().After(now.Add(jwt.AccessClockSkew)) {
		return Principal{}, auth.ErrInvalidSession
	}
	return Principal{claims: claims, restricted: state.MustChangePassword, userVersion: state.UserVersion, evaluatedAt: now}, nil
}

func (p Principal) AuthorizeMethod(method string) error {
	if p.UserID() == 0 {
		return auth.ErrInvalidSession
	}
	if p.restricted && method != userv1.UserService_Me_FullMethodName && method != restrictedChangePasswordMethod && method != restrictedLogoutMethod {
		return common.ErrForbidden
	}
	return nil
}

func (p Principal) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFromContext(ctx context.Context) (Principal, error) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	if !ok || p.UserID() == 0 {
		return Principal{}, auth.ErrInvalidSession
	}
	return p, nil
}
