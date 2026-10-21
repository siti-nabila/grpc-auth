package test_scenarios

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
)

func LogoutCredential() []testutils.Scenario {
	return []testutils.Scenario{{Name: "exactly one credential mode never installs a normal principal", Run: logoutCredential}}
}
func logoutCredential(t *testing.T) {
	for _, kind := range []string{"access", "refresh", "missing", "empty refresh", "both", "empty authorization with refresh", "duplicate", "malformed", "expired", "nil verifier", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			verifier, err := jwt.NewAccessVerifier(fixtures.AccessIssuer, fixtures.AccessKey().Public().(ed25519.PublicKey), func() time.Time { return fixtures.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			var refresh *string
			md := metadata.MD{}
			if kind == "access" || kind == "duplicate" || kind == "nil verifier" || kind == "cancel" {
				md.Set("authorization", "Bearer "+fixtures.SignAccess(t, fixtures.AccessPayload()))
			}
			if kind == "refresh" || kind == "both" || kind == "empty authorization with refresh" {
				r := "test-only-refresh"
				refresh = &r
			}
			if kind == "empty refresh" {
				r := ""
				refresh = &r
			}
			if kind == "both" {
				md.Set("authorization", "Bearer test")
			}
			if kind == "empty authorization with refresh" {
				md.Set("authorization", "")
			}
			if kind == "duplicate" {
				md.Append("authorization", md.Get("authorization")[0])
			}
			if kind == "malformed" {
				md.Set("authorization", "bearer token")
			}
			if kind == "expired" {
				p := fixtures.AccessPayload()
				p["iat"] = fixtures.ServiceTime.Add(-16 * time.Minute).Unix()
				p["exp"] = fixtures.ServiceTime.Add(-time.Minute).Unix()
				md.Set("authorization", "Bearer "+fixtures.SignAccess(t, p))
			}
			if kind == "nil verifier" {
				verifier = nil
			}
			ctx = metadata.NewIncomingContext(ctx, md)
			if kind == "cancel" {
				cancel()
			}
			proof, err := sessions.VerifyLogoutCredential(ctx, refresh, verifier)
			if kind == "access" {
				claims, ok := proof.Access()
				if err != nil || !ok || claims.UserID() != 42 || claims.SessionID() != fixtures.SessionID {
					t.Fatal("access proof", err)
				}
				if _, ok = proof.RefreshToken(); ok {
					t.Fatal("ambiguous mode")
				}
			} else if kind == "refresh" {
				raw, ok := proof.RefreshToken()
				if err != nil || !ok || raw != *refresh {
					t.Fatal("refresh proof", err)
				}
				if _, ok = proof.Access(); ok {
					t.Fatal("ambiguous mode")
				}
			} else {
				want := error(auth.ErrInvalidSession)
				if kind == "cancel" {
					want = context.Canceled
				}
				if !errors.Is(err, want) {
					t.Fatal("invalid mode admitted", err)
				}
			}
			if _, err := sessions.PrincipalFromContext(ctx); !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("terminal proof became protected principal")
			}
			if data, err := json.Marshal(proof); err != nil || string(data) != "{}" {
				t.Fatal("credential serialized")
			}
		})
	}
}
