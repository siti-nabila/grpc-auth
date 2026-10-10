package test_scenarios

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/tests/features/profile/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
	"google.golang.org/grpc/codes"
	"reflect"
	"testing"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "profile mutations use principal lock bound owner and committed response", Run: update}}
}
func update(t *testing.T) {
	for _, name := range []string{"", "missing principal", "missing profile", "begin", "busy", "lock", "unavailable", "deadline", "canceled lock", "read", "owner", "update", "commit", "panic", "cancel"} {
		t.Run(name, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := context.WithValue(base, sessions.UserSessionKey, &sessions.UserSession{UserId: 42})
			if name == "missing principal" {
				ctx = base
			}
			tx := &shared.Transaction{Context: ctx}
			provider := &shared.TransactionProvider{Tx: tx}
			failure := errors.New("dependency failure")
			if name == "begin" {
				provider.Error = failure
			}
			if name == "busy" {
				tx.Busy = true
			}
			if name == "lock" {
				tx.LockError = failure
			}
			if name == "unavailable" {
				tx.LockError = &normalizeerr.DBError{Kind: normalizeerr.KindUnknown, Raw: failure}
			}
			if name == "deadline" {
				tx.LockError = &normalizeerr.DBError{Kind: normalizeerr.KindUnknown, Raw: context.DeadlineExceeded}
			}
			if name == "canceled lock" {
				tx.LockError = &normalizeerr.DBError{Kind: normalizeerr.KindUnknown, Raw: context.Canceled}
			}
			if name == "commit" {
				tx.CommitError = failure
			}
			deps := profile.Dependencies{Transactions: provider, Bind: func(handle domain.Transaction) profile.Repositories {
				if handle != tx {
					t.Fatal("handle changed")
				}
				tx.Record("bind")
				return profile.Repositories{
					Reader: mocks.Reader{Get: func(id uint64) (domain.Profile, error) {
						tx.Record("read")
						if id != 42 {
							t.Fatal("wrong lookup owner")
						}
						if name == "read" {
							return domain.Profile{}, failure
						}
						if name == "missing profile" {
							return domain.Profile{}, auth.ErrNotFound
						}
						owner := uint64(42)
						if name == "owner" {
							owner = 99
						}
						return domain.Profile{Id: 7, UserId: owner, Name: "old", Address: "old", Phone: "old"}, nil
					}},
					Writer: mocks.Writer{Write: func(req *domain.UpdateProfileRequest) error {
						tx.Record("update")
						if req.Id != 7 || req.Name != "new" || req.Address != "" || req.Phone != "" {
							t.Fatal("full replacement or identity changed")
						}
						if name == "panic" {
							panic("private payload")
						}
						if name == "cancel" {
							cancel()
							return ctx.Err()
						}
						if name == "update" {
							return failure
						}
						return nil
					}},
				}
			}}
			service := profile.NewProfileServiceWithDependencies(ctx, deps)
			result, err := service.UpdateProfile(domain.UpdateProfileRequest{Id: 999, Name: "new"})
			full := []string{"begin", "lock:auth:user:42", "bind", "read", "update", "commit"}
			want := full
			stops := map[string]int{"missing principal": 0, "missing profile": 4, "begin": 1, "busy": 2, "lock": 2, "unavailable": 2, "deadline": 2, "canceled lock": 2, "read": 4, "owner": 4, "update": 5, "panic": 5, "cancel": 5, "commit": 6}
			if n, ok := stops[name]; ok {
				want = append([]string{}, full[:n]...)
				if n > 1 {
					want = append(want, "rollback")
				}
			}
			if !reflect.DeepEqual(tx.Calls, want) && !(len(tx.Calls) == 0 && len(want) == 0) {
				t.Fatalf("calls %v want %v", tx.Calls, want)
			}
			if name == "" {
				if err != nil || result != (domain.Profile{Id: 7, UserId: 42, Name: "new"}) {
					t.Fatalf("result %+v err %v", result, err)
				}
			} else {
				if err == nil || result != (domain.Profile{}) {
					t.Fatal("failure returned success profile")
				}
				if name == "busy" && !errors.Is(err, user.ErrResourceBusy) {
					t.Fatal("wrong busy error")
				}
				if name == "missing profile" && !errors.Is(err, auth.ErrNotFound) {
					t.Fatal("missing profile error lost")
				}
				if name == "owner" && !errors.Is(err, common.ErrForbidden) {
					t.Fatal("cross owner permitted")
				}
				if name == "panic" && !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("panic leaked")
				}
				if name == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal("cancel cause lost")
				}
				if name == "begin" || name == "lock" || name == "read" || name == "update" || name == "commit" {
					if !errors.Is(err, failure) {
						t.Fatal("dependency cause lost")
					}
				}
				if expected, ok := (map[string]codes.Code{"busy": codes.Aborted, "unavailable": codes.Unavailable, "deadline": codes.DeadlineExceeded, "canceled lock": codes.Canceled})[name]; ok {
					decoded, ok := grpcerror.Decode(helpers.HandleErrorContext(locale.NewContext(ctx, "id"), err))
					if !ok || decoded.Code != expected {
						t.Fatalf("lock boundary code %+v want %v", decoded, expected)
					}
				}
			}
		})
	}
}
