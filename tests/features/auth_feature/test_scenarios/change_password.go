package test_scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	sessionmocks "github.com/siti-nabila/grpc-auth/tests/features/session/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func ChangePassword() []testutils.Scenario {
	return []testutils.Scenario{{Name: "password proof preparation locked recheck and atomic global invalidation", Run: passwordMatrix}, {Name: "password bounds and input JSON exclude both credentials", Run: passwordValidation}}
}

func passwordMatrix(t *testing.T) {
	other := "00000000-0000-0000-0000-000000000001"
	path := []string{"candidate", "verify", "hash", "begin", "lock:auth:user:42", "bind", "ids", "lock:auth:session:" + other, "lock:auth:session:" + fixtures.SessionID, "state", "account", "password", "sessions", "refresh", "password audit", "global audit", "event-id", "outbox", "commit"}
	cases := []string{"success", "restricted", "wrong password", "foreign candidate", "missing candidate", "inactive candidate", "stale candidate", "stale account", "foreign account", "missing account", "stale session", "zero password", "zero sessions", "invalid IDs", "incomplete", "overflow", "empty hash", "long hash", "missing principal", "missing caller", "busy", "error:lock", "error:begin", "error:commit"}
	for _, stage := range path {
		if stage != "begin" && stage != "bind" && stage != "commit" && !strings.HasPrefix(stage, "lock:") {
			for _, fail := range []string{"error", "panic", "cancel"} {
				cases = append(cases, fail+":"+stage)
			}
		}
	}
	for _, kind := range cases {
		t.Run(kind, func(t *testing.T) {
			ctx := fixtures.CallerContext(t, context.Background(), authv1.AuthService_ChangePassword_FullMethodName, authv1.AuthService_ChangePassword_FullMethodName)
			if kind == "missing caller" {
				ctx = context.Background()
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			state := fixtures.SessionState()
			if kind == "restricted" {
				state.MustChangePassword = true
			}
			if kind == "overflow" {
				state.UserVersion = math.MaxInt64
			}
			if kind != "missing principal" {
				p, e := sessions.PrincipalForState(fixtures.AccessClaims(t), state, fixtures.ServiceTime)
				if e != nil {
					t.Fatal(e)
				}
				ctx = p.Context(ctx)
			}
			tx := &shared.Transaction{Context: ctx}
			provider := &shared.TransactionProvider{Tx: tx}
			cause := errors.New("controlled private failure")
			step := func(stage string) error {
				tx.Record(stage)
				switch kind {
				case "error:" + stage:
					return cause
				case "cancel:" + stage:
					cancel()
					return context.Canceled
				case "panic:" + stage:
					panic("private panic")
				}
				return nil
			}
			if kind == "busy" {
				tx.Busy = true
			}
			if kind == "error:lock" {
				tx.LockError = cause
			}
			if kind == "error:begin" {
				provider.Error = cause
			}
			if kind == "error:commit" {
				tx.CommitError = cause
			}
			a := domain.LoginAccount{ID: 42, Email: "fixture@example.test", Password: "saved-private-hash", Status: types.UserActive, AuthVersion: 3, Version: state.UserVersion, MustChangePassword: state.MustChangePassword}
			account := func(stage string) mocks.LoginAccountReader {
				return mocks.LoginAccountReader{ID: func(got context.Context, id uint64) (domain.LoginAccount, error) {
					if got != ctx || id != 42 {
						t.Fatal("account scope")
					}
					if e := step(stage); e != nil {
						return a, e
					}
					r := a
					if stage == "candidate" {
						switch kind {
						case "foreign candidate":
							r.ID = 43
						case "missing candidate":
							return r, auth.ErrNotFound
						case "inactive candidate":
							r.Status = types.UserSuspended
						case "stale candidate":
							r.AuthVersion++
						}
					}
					if stage == "account" {
						switch kind {
						case "foreign account":
							r.ID = 43
						case "missing account":
							return r, auth.ErrNotFound
						case "stale account":
							r.Password = "different private hash"
						}
					}
					return r, nil
				}}
			}
			d := authfeature.PasswordDependencies{Transactions: provider, Candidate: account("candidate"), Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }, VerifyPassword: func(saved, raw string) (bool, error) {
				if saved != a.Password || raw != "secret12" {
					t.Fatal("password proof arguments")
				}
				return kind != "wrong password", step("verify")
			}, HashPassword: func(raw string) (string, error) {
				if raw != "new-secret12" {
					t.Fatal("new password argument")
				}
				hash := "new-private-hash"
				if kind == "empty hash" {
					hash = ""
				}
				if kind == "long hash" {
					hash = strings.Repeat("x", 151)
				}
				return hash, step("hash")
			}}
			d.Bind = func(handle domain.Transaction) authfeature.PasswordRepositories {
				if handle != tx {
					t.Fatal("handle")
				}
				tx.Record("bind")
				if kind == "incomplete" {
					return authfeature.PasswordRepositories{}
				}
				return authfeature.PasswordRepositories{
					Account: account("account"), State: mocks.SessionStateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
						if got != ctx || id != fixtures.SessionID {
							t.Fatal("state scope")
						}
						fresh := state
						if kind == "stale session" {
							fresh.AuthVersion++
						}
						return fresh, step("state")
					}},
					Targets: sessionmocks.Targets{IDs: func(got context.Context, id uint64) ([]string, error) {
						if got != ctx || id != 42 {
							t.Fatal("ID scope")
						}
						ids := []string{fixtures.SessionID, other, fixtures.SessionID}
						if kind == "invalid IDs" {
							ids = append(ids, "bad")
						}
						return ids, step("ids")
					}},
					Credentials: mocks.CredentialInvalidation{Password: func(got context.Context, c domain.PasswordChange) (bool, error) {
						if got != ctx || c.Validate() != nil || c.UserID != 42 || c.ExpectedVersion != state.UserVersion || c.ExpectedAuthVersion != 3 || c.PasswordHash != "new-private-hash" || !c.At.Equal(fixtures.ServiceTime) {
							t.Fatal("password mutation scope")
						}
						return kind != "zero password", step("password")
					}},
					Sessions: mocks.SessionRevocation{All: func(got context.Context, r domain.SessionRevocation) (int64, error) {
						if got != ctx || r.Validate() != nil || r.UserID != 42 || r.ActorID != 42 || r.SessionID != "" || r.Reason != types.AuditPasswordChange {
							t.Fatal("global session scope")
						}
						n := int64(2)
						if kind == "zero sessions" {
							n = 0
						}
						return n, step("sessions")
					}},
					Refresh: mocks.RefreshRevocation{All: func(got context.Context, id uint64, at time.Time) (int64, error) {
						if got != ctx || id != 42 || !at.Equal(fixtures.ServiceTime) {
							t.Fatal("global refresh scope")
						}
						return 2, step("refresh")
					}},
					Audit: mocks.AuditWriter{Write: func(r domain.AuditRequest) error {
						record, e := r.Record()
						if e != nil || record.ActorUserID == nil || *record.ActorUserID != 42 || r.TargetID != "42" || r.Reason != types.AuditPasswordChange || r.Before.Version != state.UserVersion || r.After.Version != state.UserVersion+1 {
							t.Fatal("safe audit", e)
						}
						data, _ := json.Marshal(record)
						if strings.Contains(string(data), "private-hash") {
							t.Fatal("hash leaked")
						}
						stage := "password audit"
						if r.Action == types.AuditUserSessionsRevoked {
							stage = "global audit"
						} else if r.Action != types.AuditPasswordChanged {
							t.Fatal("unexpected audit")
						}
						return step(stage)
					}},
					Outbox: shared.OutboxWriter{Write: func(e domain.EventEnvelope) error {
						r, er := e.Record()
						if er != nil || e.EventType != types.EventUserSessionsRevoked || e.AggregateID != "42" || e.AggregateVersion != state.UserVersion+1 || e.Data.ReasonCode != "PASSWORD_CHANGED" || e.Data.SessionsRevoked == nil || !*e.Data.SessionsRevoked || r.PublishedAt != nil {
							t.Fatal("safe global event", er)
						}
						if strings.Contains(r.Payload, "private-hash") {
							t.Fatal("hash leaked")
						}
						return step("outbox")
					}},
				}
			}
			svc, e := authfeature.NewChangePasswordService(d)
			if e != nil {
				t.Fatal(e)
			}
			res, err := svc.ChangePassword(ctx, domain.ChangePasswordInput{CurrentPassword: "secret12", NewPassword: "new-secret12"})
			stop := map[string]string{"wrong password": "verify", "foreign candidate": "candidate", "missing candidate": "candidate", "inactive candidate": "candidate", "stale candidate": "candidate", "stale account": "account", "foreign account": "account", "missing account": "account", "stale session": "state", "zero password": "password", "zero sessions": "sessions", "invalid IDs": "ids", "incomplete": "bind", "overflow": "account", "empty hash": "hash", "long hash": "hash", "busy": "lock:auth:user:42", "error:lock": "lock:auth:user:42", "error:begin": "begin", "error:commit": "commit"}[kind]
			for _, stage := range path {
				if kind == "error:"+stage || kind == "panic:"+stage || kind == "cancel:"+stage {
					stop = stage
				}
			}
			want := append([]string{}, path...)
			noCalls := kind == "missing principal" || kind == "missing caller"
			if noCalls {
				want = nil
			} else if stop != "" {
				for n, stage := range want {
					if stage == stop {
						want = want[:n+1]
						if n > indexOf(path, "begin") {
							want = append(want, "rollback")
						}
						break
					}
				}
			}
			if !reflect.DeepEqual(tx.Calls, want) {
				t.Fatalf("calls %v want %v", tx.Calls, want)
			}
			if noCalls || stop != "" {
				if res != nil || err == nil {
					t.Fatal("failure outcome", err)
				}
				if strings.HasPrefix(kind, "error:") && !errors.Is(err, cause) {
					t.Fatal("cause lost", err)
				}
				if strings.HasPrefix(kind, "panic:") && !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("panic outcome", err)
				}
				if strings.HasPrefix(kind, "cancel:") && !errors.Is(err, context.Canceled) {
					t.Fatal("cancel outcome", err)
				}
				if kind == "wrong password" && !errors.Is(err, auth.ErrPasswordMismatch) {
					t.Fatal("wrong password mapping", err)
				}
				if kind == "zero password" && !errors.Is(err, user.ErrVersionConflict) {
					t.Fatal("conditional update", err)
				}
				return
			}
			if err != nil || res == nil || !res.Changed || !res.LoginRequired {
				t.Fatal("committed result", err)
			}
		})
	}
	if _, err := authfeature.NewChangePasswordService(authfeature.PasswordDependencies{}); err == nil {
		t.Fatal("incomplete dependencies")
	}
}

func passwordValidation(t *testing.T) {
	for _, field := range []string{"current_password", "new_password"} {
		for _, n := range []int{0, 5, 6, 50, 51} {
			t.Run(field+"/"+strings.Repeat("x", n), func(t *testing.T) {
				r := domain.ChangePasswordInput{CurrentPassword: "secret12", NewPassword: "new-secret12"}
				if field == "current_password" {
					r.CurrentPassword = strings.Repeat("x", n)
				} else {
					r.NewPassword = strings.Repeat("x", n)
				}
				err := r.Validate()
				if n >= 6 && n <= 50 {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				var fields dictionary.FieldErrors
				if !errors.As(err, &fields) || len(fields) != 1 || fields[field] == nil {
					t.Fatal("field validation", err)
				}
			})
		}
	}
	data, err := json.Marshal(domain.ChangePasswordInput{CurrentPassword: "private-current", NewPassword: "private-new"})
	if err != nil || string(data) != "{}" {
		t.Fatal("credentials serialized")
	}
}
