package test_scenarios

import (
	"context"
	"errors"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func SessionLogin() []testutils.Scenario {
	return []testutils.Scenario{{Name: "session Login handler validates maps exact published fields and preserves failures", Run: sessionLoginHandler}}
}

func sessionLoginHandler(t *testing.T) {
	for _, kind := range []string{"success", "nil", "invalid", "failure", "nil result", "empty result"} {
		ctx := context.WithValue(context.Background(), struct{}{}, "handler-context")
		cause := errors.New("controlled producer error")
		calls := 0
		result := &authfeature.LoginResult{AccessToken: "test-access", RefreshToken: "test-refresh", SessionID: fixtures.SessionID, AccessExpiresAt: fixtures.ServiceTime.Add(15 * time.Minute), SessionExpiresAt: fixtures.ServiceTime.Add(7 * 24 * time.Hour), MustChangePassword: true}
		h, err := handler.NewSessionLoginHandler(mocks.SessionLogin{Call: func(got context.Context, req domain.AuthRequest) (*authfeature.LoginResult, error) {
			calls++
			if got != ctx || req.Email != "member@example.com" || req.Password != "secret12" {
				t.Fatal("request/context")
			}
			switch kind {
			case "failure":
				return result, cause
			case "nil result":
				return nil, nil
			case "empty result":
				return &authfeature.LoginResult{}, nil
			}
			return result, nil
		}})
		if err != nil {
			t.Fatal(err)
		}
		req := &userv1.AuthRequest{Email: " MEMBER@example.com ", Password: "secret12"}
		if kind == "nil" {
			req = nil
		}
		if kind == "invalid" {
			req.Email = "invalid"
		}
		res, err := h.Login(ctx, req)
		if kind == "success" {
			if err != nil || res.Token != result.AccessToken || res.RefreshToken != result.RefreshToken || res.SessionId != result.SessionID || !res.AccessExpiresAt.AsTime().Equal(result.AccessExpiresAt) || !res.SessionExpiresAt.AsTime().Equal(result.SessionExpiresAt) || !res.MustChangePassword || res.RegistrationId != 0 || res.RegistrationStatus != "" {
				t.Fatal("published response mapping", err)
			}
		} else {
			if err == nil || res != nil {
				t.Fatal("failed handler credential leak")
			}
			if kind == "failure" && !errors.Is(err, cause) {
				t.Fatal("cause lost")
			}
			if kind == "nil result" || kind == "empty result" {
				if !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("malformed result accepted")
				}
			}
		}
		want := 1
		if kind == "nil" || kind == "invalid" {
			want = 0
		}
		if calls != want {
			t.Fatal("feature call count", kind)
		}
	}
	if _, err := handler.NewSessionLoginHandler(nil); err == nil {
		t.Fatal("missing producer accepted")
	}
}
