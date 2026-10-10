package test_scenarios

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
	"google.golang.org/grpc/codes"
	"reflect"
	"testing"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "Register has one ordered request-bound transaction and no credential on failure", Run: register}, {Name: "Login uses canonical email and preserves credential semantics", Run: login}}
}

func register(t *testing.T) {
	for _, failureAt := range []string{"", "invalid", "hash", "hash panic", "canceled begin", "begin", "busy", "lock", "unavailable", "deadline", "canceled lock", "lookup", "duplicate", "role", "auth", "profile", "assignment", "token", "nil token", "commit", "panic", "cancel"} {
		t.Run(failureAt, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx := &shared.Transaction{Context: ctx}
			provider := &shared.TransactionProvider{Tx: tx}
			failure := errors.New("dependency failed")
			tokenValue := "issued-credential"
			if failureAt == "begin" {
				provider.Error = failure
			}
			if failureAt == "busy" {
				tx.Busy = true
			}
			if failureAt == "lock" {
				tx.LockError = failure
			}
			if failureAt == "unavailable" {
				tx.LockError = &normalizeerr.DBError{Kind: normalizeerr.KindUnknown, Raw: failure}
			}
			if failureAt == "deadline" {
				tx.LockError = &normalizeerr.DBError{Kind: normalizeerr.KindUnknown, Raw: context.DeadlineExceeded}
			}
			if failureAt == "canceled lock" {
				tx.LockError = &normalizeerr.DBError{Kind: normalizeerr.KindUnknown, Raw: context.Canceled}
			}
			if failureAt == "commit" {
				tx.CommitError = failure
			}
			step := func(name string) error {
				tx.Record(name)
				if failureAt == name {
					return failure
				}
				return nil
			}
			deps := authfeature.RegisterDependencies{
				Transactions: provider,
				HashPassword: func(password string) (string, error) {
					if password != "secret12" {
						t.Fatal("password changed")
					}
					if err := step("hash"); err != nil {
						return "", err
					}
					if failureAt == "hash panic" {
						panic("private hash payload")
					}
					return "encoded-hash", nil
				},
				GenerateToken: func(id uint64) (*string, error) {
					if id != 42 {
						t.Fatal("wrong token owner")
					}
					if err := step("token"); err != nil {
						return nil, err
					}
					if failureAt == "nil token" {
						return nil, nil
					}
					return &tokenValue, nil
				},
				Bind: func(handle domain.Transaction) authfeature.RegisterRepositories {
					if handle != tx {
						t.Fatal("bound handle changed")
					}
					tx.Record("bind")
					return authfeature.RegisterRepositories{
						AuthReader: mocks.AuthReader{Get: func(email string) (domain.Auth, error) {
							if email != "user@example.test" {
								t.Fatal("email not canonical")
							}
							if err := step("lookup"); err != nil {
								return domain.Auth{}, err
							}
							if failureAt == "duplicate" {
								return domain.Auth{Id: 42}, nil
							}
							return domain.Auth{}, auth.ErrNotFound
						}},
						RoleReader: mocks.RoleReader{Get: func() (domain.Role, error) { return domain.Role{Id: 7}, step("role") }},
						AuthWriter: mocks.AuthWriter{Write: func(req *domain.AuthRequest) error {
							if req.Email != "user@example.test" || req.Password != "encoded-hash" {
								t.Fatal("unsafe auth write")
							}
							req.Id = 42
							return step("auth")
						}},
						ProfileWriter: mocks.ProfileWriter{Write: func(req *domain.ProfileRequest) error {
							if req.UserId != 42 {
								t.Fatal("wrong profile owner")
							}
							if failureAt == "panic" {
								tx.Record("profile")
								panic("private payload")
							}
							if failureAt == "cancel" {
								cancel()
								tx.Record("profile")
								return ctx.Err()
							}
							return step("profile")
						}},
						UserRoleWriter: mocks.UserRoleWriter{Write: func(req *domain.UserRoleRequest) error {
							if req.UserId != 42 || req.RoleId != 7 {
								t.Fatal("wrong assignment")
							}
							return step("assignment")
						}},
					}
				},
			}
			request := domain.AuthRequest{Email: " USER@example.test ", Password: "secret12"}
			if failureAt == "invalid" {
				request.Email = "invalid"
			}
			service := authfeature.NewRegisterServiceWithDependencies(ctx, deps)
			if failureAt == "canceled begin" {
				cancel()
			}
			token, err := service.Register(request)
			keys := featurecommon.RegisterLockKeys(request.Email)
			full := []string{"hash", "begin", "lock:" + keys[0], "lock:" + keys[1], "bind", "lookup", "role", "auth", "profile", "assignment", "token", "commit"}
			want := full
			stop := map[string]int{"invalid": 0, "hash": 1, "hash panic": 1, "canceled begin": 0, "begin": 2, "busy": 3, "lock": 3, "unavailable": 3, "deadline": 3, "canceled lock": 3, "lookup": 6, "duplicate": 6, "role": 7, "auth": 8, "profile": 9, "panic": 9, "cancel": 9, "assignment": 10, "token": 11, "nil token": 11, "commit": 12}
			if n, ok := stop[failureAt]; ok {
				want = append([]string{}, full[:n]...)
				if n > 2 {
					want = append(want, "rollback")
				}
			}
			if !reflect.DeepEqual(tx.Calls, want) && !(len(tx.Calls) == 0 && len(want) == 0) {
				t.Fatalf("calls %v want %v", tx.Calls, want)
			}
			if failureAt == "" {
				if err != nil || token == nil || *token != tokenValue {
					t.Fatalf("success token=%v err=%v", token, err)
				}
			} else {
				if err == nil || token != nil {
					t.Fatal("failure returned credential/success")
				}
				if failureAt == "busy" && !errors.Is(err, auth.ErrResourceBusy) {
					t.Fatal("wrong busy error")
				}
				if failureAt == "duplicate" && !errors.Is(err, auth.ErrDataExists) {
					t.Fatal("wrong duplicate error")
				}
				if (failureAt == "panic" || failureAt == "hash panic" || failureAt == "nil token") && !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("panic leaked")
				}
				if (failureAt == "cancel" || failureAt == "canceled begin") && !errors.Is(err, context.Canceled) {
					t.Fatal("cancel cause lost")
				}
				if expected, ok := (map[string]codes.Code{"busy": codes.Aborted, "unavailable": codes.Unavailable, "deadline": codes.DeadlineExceeded, "canceled lock": codes.Canceled})[failureAt]; ok {
					decoded, ok := grpcerror.Decode(helpers.HandleErrorContext(locale.NewContext(ctx, "id"), err))
					if !ok || decoded.Code != expected {
						t.Fatalf("lock boundary code %+v want %v", decoded, expected)
					}
				}
				if failureAt != "invalid" && failureAt != "busy" && failureAt != "duplicate" && failureAt != "panic" && failureAt != "cancel" && failureAt != "deadline" && failureAt != "canceled lock" && failureAt != "hash panic" && failureAt != "canceled begin" && failureAt != "nil token" && !errors.Is(err, failure) {
					t.Fatal("dependency cause lost")
				}
			}
		})
	}
}
