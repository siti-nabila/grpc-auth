package fixtures

import (
	"bytes"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	jwtPackage "github.com/siti-nabila/grpc-auth/pkg/jwt"
)

const AccessIssuer = "auth-test"
const AccessTokenID = "00000000-0000-4000-8000-000000000002"

func AccessKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, ed25519.SeedSize))
}

func AccessPayload() jwt.MapClaims {
	return jwt.MapClaims{"sub": "42", "sid": SessionID, "jti": AccessTokenID, "iss": AccessIssuer, "iat": ServiceTime.Add(-time.Minute).Unix(), "exp": ServiceTime.Add(14 * time.Minute).Unix(), "auth_version": int64(3)}
}

func SignAccess(t *testing.T, payload jwt.MapClaims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, payload).SignedString(AccessKey())
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func AccessClaims(t *testing.T) jwtPackage.VerifiedClaims {
	t.Helper()
	v, err := jwtPackage.NewAccessVerifier(AccessIssuer, AccessKey().Public().(ed25519.PublicKey), func() time.Time { return ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	claims, err := v.Verify(SignAccess(t, AccessPayload()))
	if err != nil {
		t.Fatal(err)
	}
	return claims
}

func SessionState() domain.SessionState {
	return domain.SessionState{UserID: 42, UserStatus: "ACTIVE", AuthVersion: 3, UserVersion: 4,
		SessionID: SessionID, SessionUserID: 42, SessionStatus: "ACTIVE", CreatedAt: ServiceTime.Add(-time.Hour), ExpiresAt: ServiceTime.Add(-time.Hour).Add(7 * 24 * time.Hour)}
}
