package sessions

import (
	"context"
	"strings"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"google.golang.org/grpc/metadata"
)

// LogoutCredential proves only the selected credential mode. It is deliberately
// separate from Principal: terminal ownership never grants protected access.
type LogoutCredential struct {
	access             jwt.VerifiedClaims
	refresh            string
	refreshMode, valid bool
}

func (c LogoutCredential) Access() (jwt.VerifiedClaims, bool) {
	return c.access, c.valid && !c.refreshMode
}
func (c LogoutCredential) RefreshToken() (string, bool) { return c.refresh, c.valid && c.refreshMode }

func VerifyLogoutCredential(ctx context.Context, refresh *string, verifier *jwt.AccessVerifier) (LogoutCredential, error) {
	if err := ctx.Err(); err != nil {
		return LogoutCredential{}, err
	}
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if refresh != nil {
		if *refresh == "" || len(values) != 0 {
			return LogoutCredential{}, auth.ErrInvalidSession
		}
		return LogoutCredential{refresh: *refresh, refreshMode: true, valid: true}, nil
	}
	if verifier == nil || len(values) != 1 {
		return LogoutCredential{}, auth.ErrInvalidSession
	}
	parts := strings.Split(values[0], " ")
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return LogoutCredential{}, auth.ErrInvalidSession
	}
	claims, err := verifier.Verify(parts[1])
	if err != nil {
		return LogoutCredential{}, err
	}
	return LogoutCredential{access: claims, valid: true}, nil
}
