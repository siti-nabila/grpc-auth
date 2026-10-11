package test_scenarios

import (
	"context"
	"errors"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	grpcmapping "github.com/siti-nabila/api-contracts/pkg/grpcerror/mapping"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"testing"
)

func ChangePasswordHandler() []testutils.Scenario {
	return []testutils.Scenario{{Name: "password handler maps proof and safe results with localized catalog errors", Run: changePasswordHandler}}
}
func changePasswordHandler(t *testing.T) {
	for _, kind := range []string{"success", "nil request", "nil result", "wrong password", "dependency", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			want := map[string]error{"wrong password": auth.ErrPasswordMismatch, "dependency": common.ErrDependencyUnavailable, "cancel": context.Canceled}[kind]
			featureErr := want
			h, err := handler.NewChangePasswordHandler(mocks.ChangePassword{Call: func(got context.Context, r domain.ChangePasswordInput) (*authfeature.ChangePasswordResult, error) {
				calls++
				if got != ctx || r.CurrentPassword != " old-password " || r.NewPassword != " new-password " {
					t.Fatal("context/proof mapping")
				}
				if featureErr != nil {
					return nil, featureErr
				}
				if kind == "nil result" {
					return nil, nil
				}
				return &authfeature.ChangePasswordResult{Changed: true, LoginRequired: true}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			req := &authv1.ChangePasswordRequest{CurrentPassword: " old-password ", NewPassword: " new-password "}
			if kind == "nil request" {
				req = nil
				want = common.ErrBadRequest
			}
			if kind == "nil result" {
				want = common.ErrInternalServerError
			}
			r, err := h.ChangePassword(ctx, req)
			if kind == "success" {
				if err != nil || r == nil || !r.Changed || !r.LoginRequired {
					t.Fatal("response mapping", err)
				}
			} else if r != nil || !errors.Is(err, want) {
				t.Fatal("error identity", err)
			}
			n := 1
			if kind == "nil request" {
				n = 0
			}
			if calls != n {
				t.Fatal("feature call count", calls)
			}
		})
	}
	encoder := grpcerror.NewEncoder(append(grpcmapping.CommonCodeMappings(), grpcmapping.AuthCodeMappings()...)...)
	for _, language := range []locale.Language{locale.English, locale.Indonesian} {
		ctx := locale.NewContext(context.Background(), string(language))
		_, err := interceptors.NewErrorInterceptor(encoder)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: authv1.AuthService_ChangePassword_FullMethodName}, func(context.Context, any) (any, error) { return nil, auth.ErrPasswordMismatch })
		decoded, ok := grpcerror.Decode(err)
		if !ok || decoded.Reason != auth.ErrPasswordMismatch.Key() || decoded.Message != auth.ErrPasswordMismatch.Message(language) {
			t.Fatal("localized mismatch", err)
		}
	}
	if _, err := handler.NewChangePasswordHandler(nil); err == nil {
		t.Fatal("nil dependency accepted")
	}
}
