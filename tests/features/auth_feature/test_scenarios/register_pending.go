package test_scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func PendingRegistration() []testutils.Scenario {
	return []testutils.Scenario{{Name: "pending registration commits safe nonadmin membership audit and outbox", Run: pendingRegistration}}
}

func pendingRegistration(t *testing.T) {
	for _, name := range []string{"success", "caller", "privilege", "validation", "hash", "begin", "busy", "lock", "duplicate", "lookup", "role", "zero role", "auth", "duplicate write", "profile", "member", "audit", "event-id", "outbox", "commit", "panic", "cancel"} {
		t.Run(name, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := fixtures.CallerContext(t, base, userv1.UserService_Register_FullMethodName)
			if name == "caller" {
				ctx = base
			}
			tx := &shared.Transaction{Context: ctx}
			provider := &shared.TransactionProvider{Tx: tx}
			cause := errors.New("controlled registration dependency")
			if name == "begin" {
				provider.Error = cause
			}
			if name == "busy" {
				tx.Busy = true
			}
			if name == "lock" {
				tx.LockError = cause
			}
			if name == "commit" {
				tx.CommitError = cause
			}
			step := func(op string) error {
				tx.Record(op)
				if name == op {
					return cause
				}
				return nil
			}
			var audits []domain.AuditRequest
			var events []domain.EventEnvelope
			deps := authfeature.PendingRegistrationDependencies{Transactions: provider, Now: func() time.Time { return fixtures.ServiceTime }, HashPassword: func(password string) (string, error) {
				if password != "secret12" {
					t.Fatal("password changed")
				}
				if err := step("hash"); err != nil {
					return "", err
				}
				return "safe-hash", nil
			}, EventID: func() (string, error) {
				if err := step("event-id"); err != nil {
					return "", err
				}
				return fixtures.AccessTokenID, nil
			}, Bind: func(handle domain.Transaction) authfeature.PendingRegistrationRepositories {
				if handle != tx {
					t.Fatal("handle changed")
				}
				tx.Record("bind")
				return authfeature.PendingRegistrationRepositories{
					Account: mocks.AuthReader{Get: func(email string) (domain.Auth, error) {
						if email != "new@example.test" {
							t.Fatal("email not normalized")
						}
						if err := step("lookup"); err != nil {
							return domain.Auth{}, err
						}
						if name == "duplicate" {
							return domain.Auth{Id: 42}, nil
						}
						return domain.Auth{}, auth.ErrNotFound
					}},
					Role: mocks.RegistrationRole{Read: func() (uint64, error) {
						err := step("role")
						if name == "zero role" {
							return 0, nil
						}
						return 1, err
					}},
					Auth: mocks.RegistrationWriter{Write: func(req *domain.PendingRegistration) error {
						if req.ID != 0 || req.Email != "new@example.test" || req.Password != "safe-hash" || req.Status != types.UserPending || req.AuthVersion != 1 || req.Version != 1 || req.MustChangePassword {
							t.Fatal("unsafe pending account")
						}
						err := step("auth")
						if name == "duplicate write" {
							return auth.ErrDataExists
						}
						if name == "panic" {
							panic("private payload")
						}
						if name == "cancel" {
							cancel()
							return ctx.Err()
						}
						req.ID = 42
						return err
					}},
					Profile: mocks.ProfileWriter{Write: func(req *domain.ProfileRequest) error {
						if *req != (domain.ProfileRequest{UserId: 42}) {
							t.Fatal("initial profile changed")
						}
						return step("profile")
					}},
					Member: mocks.RegistrationMember{Write: func(userID, roleID uint64) error {
						if userID != 42 || roleID != 1 {
							t.Fatal("wrong member grant")
						}
						return step("member")
					}},
					Audit: shared.AuditWriter{Write: func(req domain.AuditRequest) error { audits = append(audits, req); return step("audit") }}, Outbox: shared.OutboxWriter{Write: func(req domain.EventEnvelope) error { events = append(events, req); return step("outbox") }},
				}
			}}
			s, err := authfeature.NewPendingRegistrationService(deps)
			if err != nil {
				t.Fatal(err)
			}
			req := domain.AuthRequest{Email: " NEW@example.test ", Password: "secret12"}
			if name == "privilege" {
				req.Id = 999
			}
			if name == "validation" {
				req.Email = "invalid"
			}
			res, err := s.Register(ctx, req)
			keys := featurecommon.RegisterLockKeys("new@example.test")
			full := []string{"hash", "begin", "lock:" + keys[0], "lock:" + keys[1], "bind", "lookup", "role", "auth", "profile", "member", "audit", "event-id", "outbox", "commit"}
			want := full
			stops := map[string]int{"caller": 0, "privilege": 0, "validation": 0, "hash": 1, "begin": 2, "busy": 3, "lock": 3, "duplicate": 6, "lookup": 6, "role": 7, "zero role": 7, "auth": 8, "duplicate write": 8, "panic": 8, "cancel": 8, "profile": 9, "member": 10, "audit": 11, "event-id": 12, "outbox": 13, "commit": 14}
			if n, ok := stops[name]; ok {
				want = append([]string(nil), full[:n]...)
				if n > 2 {
					want = append(want, "rollback")
				}
			}
			if !reflect.DeepEqual(tx.Calls, want) {
				t.Fatalf("calls %v want %v", tx.Calls, want)
			}
			if name == "success" {
				if err != nil || res == nil || res.ID != 42 || res.Status != types.UserPending || len(audits) != 1 || len(events) != 1 {
					t.Fatal("registration", res, err)
				}
				record, err := audits[0].Record()
				if err != nil || record.ActorUserID != nil || record.ActorService == nil || *record.ActorService != "rest-orc" || audits[0].Action != types.AuditUserRegistered {
					t.Fatal("service actor", err)
				}
				if events[0].ActorUserID != nil || events[0].AggregateVersion != 1 || events[0].Data.Status != "PENDING" || events[0].EventType != types.EventUserCreated {
					t.Fatal("registration event")
				}
				if _, err := events[0].Record(); err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal([]any{record, events[0]})
				if strings.Contains(string(data), "safe-hash") || strings.Contains(string(data), "secret12") || strings.Contains(string(data), "new@example.test") {
					t.Fatal("sensitive registration payload")
				}
			} else {
				if err == nil || res != nil {
					t.Fatal("partial registration success", res, err)
				}
				if name == "duplicate" || name == "duplicate write" {
					if !errors.Is(err, auth.ErrDataExists) || errors.Is(err, common.ErrDependencyUnavailable) {
						t.Fatal("duplicate business mapping", err)
					}
				}
				if name == "busy" && !errors.Is(err, auth.ErrResourceBusy) || name == "panic" && !errors.Is(err, common.ErrInternalServerError) || name == "cancel" && !errors.Is(err, context.Canceled) {
					t.Fatal("failure mapping", err)
				}
				for _, dep := range []string{"hash", "begin", "lock", "lookup", "role", "auth", "profile", "member", "audit", "event-id", "outbox", "commit"} {
					if name == dep && (!errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable)) {
						t.Fatal("dependency cause lost", err)
					}
				}
			}
		})
	}
}
