package test_scenarios

import (
	"context"
	"errors"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func RefreshHandler() []testutils.Scenario {
	return []testutils.Scenario{{Name: "refresh handler preserves context credentials mapping and localized safe errors", Run: refreshHandler}}
}
func refreshHandler(t *testing.T) {
	for _, kind := range []string{"success", "nil", "empty proof", "whitespace proof", "denial", "reuse", "dependency", "cancel", "nil result", "empty result", "restricted result"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "refresh-context")
			calls := 0
			r := &authfeature.LoginResult{AccessToken: "test-access", RefreshToken: "test-refresh", SessionID: fixtures.SessionID, AccessExpiresAt: fixtures.ServiceTime.Add(15 * time.Minute), SessionExpiresAt: fixtures.ServiceTime.Add(7 * 24 * time.Hour)}
			errExpected := map[string]error{"denial": auth.ErrInvalidSession, "reuse": auth.ErrRefreshTokenReused, "dependency": common.ErrDependencyUnavailable, "cancel": context.Canceled, "empty proof": auth.ErrInvalidSession, "whitespace proof": auth.ErrInvalidSession}[kind]
			req := &authv1.RefreshSessionRequest{RefreshToken: "unchanged-proof"}
			if kind == "nil" {
				req = nil
			}
			if kind == "empty proof" {
				req.RefreshToken = ""
			}
			if kind == "whitespace proof" {
				req.RefreshToken = " proof "
			}
			h, err := handler.NewRefreshHandler(mocks.Refresh{Call: func(got context.Context, raw string) (*authfeature.LoginResult, error) {
				calls++
				if got != ctx || raw != req.RefreshToken {
					t.Fatal("request mutated or context replaced")
				}
				if errExpected != nil {
					return r, errExpected
				}
				if kind == "nil result" {
					return nil, nil
				}
				if kind == "empty result" {
					return &authfeature.LoginResult{}, nil
				}
				if kind == "restricted result" {
					r.MustChangePassword = true
				}
				return r, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			res, err := h.RefreshSession(ctx, req)
			if kind == "success" {
				if err != nil || res.Token != r.AccessToken || res.RefreshToken != r.RefreshToken || res.SessionId != r.SessionID || !res.AccessExpiresAt.AsTime().Equal(r.AccessExpiresAt) || !res.SessionExpiresAt.AsTime().Equal(r.SessionExpiresAt) || res.MustChangePassword || res.RegistrationId != 0 || res.RegistrationStatus != "" {
					t.Fatal("published mapping", err)
				}
			} else {
				if err == nil || res != nil {
					t.Fatal("failure credential leak")
				}
				if errExpected != nil && !errors.Is(err, errExpected) {
					t.Fatal("error identity lost", err)
				}
			}
			want := 1
			if kind == "nil" {
				want = 0
			}
			if calls != want {
				t.Fatal("calls", calls, want)
			}
		})
	}
	encoder := grpcerror.NewEncoder(append(grpcmapping.CommonCodeMappings(), grpcmapping.AuthCodeMappings()...)...)
	for _, language := range []locale.Language{locale.English, locale.Indonesian} {
		ctx := locale.NewContext(context.Background(), string(language))
		_, err := interceptors.NewErrorInterceptor(encoder)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: authv1.AuthService_RefreshSession_FullMethodName}, func(context.Context, any) (any, error) { return nil, auth.ErrRefreshTokenReused })
		decoded, ok := grpcerror.Decode(err)
		if !ok || decoded.Code != codes.Unauthenticated || decoded.Reason != auth.ErrRefreshTokenReused.Key() || decoded.Message != auth.ErrRefreshTokenReused.Message(language) {
			t.Fatal("localized refresh error", err)
		}
	}
	if _, err := handler.NewRefreshHandler(nil); err == nil {
		t.Fatal("nil service accepted")
	}
}
