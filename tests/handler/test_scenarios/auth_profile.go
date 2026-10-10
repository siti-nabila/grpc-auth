package test_scenarios

import (
	"context"
	"errors"
	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	profilefeature "github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/handler/mocks"
	"testing"
)

func canonicalAuthHandlers(t *testing.T) {
	for _, method := range []string{"register", "login"} {
		for _, name := range []string{"success", "duplicate", "dependency", "invalid", "nil"} {
			t.Run(method+"/"+name, func(t *testing.T) {
				ctx := context.WithValue(context.Background(), struct{}{}, "handler")
				calls := 0
				factories := 0
				failure := errors.New("dependency failed")
				h := &handler.UserHandler{NewAuthService: func(got context.Context) authfeature.AuthService {
					factories++
					if got != ctx {
						t.Fatal("context replaced")
					}
					return mocks.AuthService{Call: func(got string, req domain.AuthRequest) (*string, error) {
						calls++
						if got != method || req.Email != "user@example.test" || req.Password != "secret12" {
							t.Fatal("mapping changed")
						}
						if name == "duplicate" {
							return nil, auth.ErrDataExists
						}
						if name == "dependency" {
							return nil, failure
						}
						token := "credential"
						return &token, nil
					}}
				}}
				request := &userv1.AuthRequest{Email: " USER@example.test ", Password: "secret12"}
				if name == "invalid" {
					request.Email = "bad"
				}
				if name == "nil" {
					request = nil
				}
				invoke := h.Register
				if method == "login" {
					invoke = h.Login
				}
				result, err := invoke(ctx, request)
				if name == "invalid" || name == "nil" {
					if err == nil || result != nil || factories != 0 || calls != 0 {
						t.Fatal("invalid request reached feature")
					}
					return
				}
				if factories != 1 || calls != 1 {
					t.Fatal("wrong call count")
				}
				if name == "success" {
					if err != nil || result.GetToken() != "credential" {
						t.Fatal("success mapping failed")
					}
					return
				}
				want := failure
				if name == "duplicate" {
					want = auth.ErrDataExists
				}
				if !errors.Is(err, want) || result != nil {
					t.Fatal("feature failure swallowed")
				}
			})
		}
	}
}

func committedProfileHandler(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "profile")
	for _, name := range []string{"success", "failure", "nil"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			factories := 0
			failure := errors.New("commit failed")
			h := &handler.ProfileHandler{NewProfileService: func(got context.Context) profilefeature.ProfileService {
				factories++
				if got != ctx {
					t.Fatal("context replaced")
				}
				return mocks.ProfileService{Call: func(req domain.UpdateProfileRequest) (domain.Profile, error) {
					calls++
					if req.Id != 0 || req.Name != "new" || req.Address != "" || req.Phone != "" {
						t.Fatal("input mapping changed")
					}
					if name == "failure" {
						return domain.Profile{}, failure
					}
					return domain.Profile{Id: 7, UserId: 42, Name: "committed"}, nil
				}}
			}}
			request := &profilev1.ProfileRequest{Name: "new"}
			if name == "nil" {
				request = nil
			}
			result, err := h.UpdateProfile(ctx, request)
			if name == "nil" {
				if !errors.Is(err, common.ErrBadRequest) || result != nil || factories != 0 {
					t.Fatal("nil request reached feature")
				}
				return
			}
			if factories != 1 || calls != 1 {
				t.Fatal("unexpected call count")
			}
			if name == "failure" {
				if !errors.Is(err, failure) || result != nil {
					t.Fatal("commit failure swallowed")
				}
				return
			}
			if err != nil || result.GetProfile().GetId() != 7 || result.GetProfile().GetUserId() != 42 || result.GetProfile().GetName() != "committed" {
				t.Fatal("response uses input instead of committed profile")
			}
		})
	}
}
