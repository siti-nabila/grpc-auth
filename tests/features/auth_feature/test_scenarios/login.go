package test_scenarios

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"reflect"
	"testing"
)

func login(t *testing.T) {
	for _, name := range []string{"success", "invalid", "missing", "verify", "mismatch", "token"} {
		t.Run(name, func(t *testing.T) {
			calls := []string{}
			failure := errors.New("dependency failure")
			value := "credential"
			deps := authfeature.LoginDependencies{
				Reader: mocks.AuthReader{Get: func(email string) (domain.Auth, error) {
					calls = append(calls, "read")
					if email != "user@example.test" {
						t.Fatal("email not normalized")
					}
					if name == "missing" {
						return domain.Auth{}, auth.ErrNotFound
					}
					return domain.Auth{Id: 42, Password: "encoded"}, nil
				}},
				VerifyPassword: func(saved, request string) (bool, error) {
					calls = append(calls, "verify")
					if saved != "encoded" || request != "secret12" {
						t.Fatal("password altered")
					}
					if name == "verify" {
						return false, failure
					}
					return name != "mismatch", nil
				},
				GenerateToken: func(id uint64) (*string, error) {
					calls = append(calls, "token")
					if id != 42 {
						t.Fatal("wrong token owner")
					}
					if name == "token" {
						return nil, failure
					}
					return &value, nil
				},
			}
			request := domain.AuthRequest{Email: " USER@example.test ", Password: "secret12"}
			if name == "invalid" {
				request.Email = "invalid"
			}
			token, err := authfeature.NewLoginServiceWithDependencies(context.Background(), deps).Login(request)
			wantCalls := []string{"read", "verify", "token"}
			if name == "invalid" {
				wantCalls = []string{}
			}
			if name == "missing" {
				wantCalls = wantCalls[:1]
			}
			if name == "verify" || name == "mismatch" {
				wantCalls = wantCalls[:2]
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatal(calls)
			}
			if name == "success" {
				if err != nil || token == nil || *token != value {
					t.Fatal("login changed")
				}
				return
			}
			if err == nil || token != nil {
				t.Fatal("failed login returned credential")
			}
			if name == "missing" && !errors.Is(err, auth.ErrNotFound) {
				t.Fatal("not found changed")
			}
			if name == "mismatch" && !errors.Is(err, auth.ErrPasswordMismatch) {
				t.Fatal("mismatch changed")
			}
			if (name == "verify" || name == "token") && !errors.Is(err, failure) {
				t.Fatal("dependency cause lost")
			}
		})
	}
}
