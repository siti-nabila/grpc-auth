package test_scenarios

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
)

func LogoutHandler() []testutils.Scenario {
	return []testutils.Scenario{{Name: "Logout optional credential presence and safe changed result map without identity payload", Run: logoutHandler}}
}
func logoutHandler(t *testing.T) {
	v, err := jwt.NewAccessVerifier(fixtures.AccessIssuer, fixtures.AccessKey().Public().(ed25519.PublicKey), func() time.Time { return fixtures.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"access", "refresh", "noop", "nil", "missing", "empty present", "both", "failure", "nil result"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			req := &authv1.LogoutRequest{}
			calls := 0
			cause := errors.New("controlled command failure")
			if kind != "missing" && kind != "refresh" && kind != "empty present" {
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+fixtures.SignAccess(t, fixtures.AccessPayload())))
			}
			if kind == "refresh" || kind == "both" {
				r := "test-proof"
				req.RefreshToken = &r
			}
			if kind == "empty present" {
				r := ""
				req.RefreshToken = &r
			}
			if kind == "nil" {
				req = nil
			}
			h, err := handler.NewLogoutHandler(mocks.Logout{Call: func(got context.Context, c sessions.LogoutCredential) (*authfeature.LogoutResult, error) {
				calls++
				if got != ctx {
					t.Fatal("context replaced")
				}
				if kind == "refresh" {
					if r, ok := c.RefreshToken(); !ok || r != "test-proof" {
						t.Fatal("refresh presence lost")
					}
				} else {
					if claims, ok := c.Access(); !ok || claims.UserID() != 42 {
						t.Fatal("access proof lost")
					}
				}
				if kind == "failure" {
					return nil, cause
				}
				if kind == "nil result" {
					return nil, nil
				}
				return &authfeature.LogoutResult{SessionID: fixtures.SessionID, Changed: kind != "noop"}, nil
			}}, v)
			if err != nil {
				t.Fatal(err)
			}
			res, err := h.Logout(ctx, req)
			if kind == "access" || kind == "refresh" || kind == "noop" {
				if err != nil || res.Changed != (kind != "noop") {
					t.Fatal("changed mapping", err)
				}
			} else {
				if err == nil || res != nil {
					t.Fatal("invalid outcome")
				}
				if kind == "failure" && !errors.Is(err, cause) {
					t.Fatal("cause lost")
				}
				if kind == "nil result" && !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("malformed result")
				}
			}
			want := 1
			if kind == "nil" || kind == "missing" || kind == "empty present" || kind == "both" {
				want = 0
			}
			if calls != want {
				t.Fatal("feature calls", calls, want)
			}
		})
	}
	if _, err := handler.NewLogoutHandler(nil, v); err == nil {
		t.Fatal("nil service accepted")
	}
	if _, err := handler.NewLogoutHandler(mocks.Logout{}, nil); err == nil {
		t.Fatal("nil verifier accepted")
	}
}
