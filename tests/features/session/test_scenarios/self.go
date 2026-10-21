package test_scenarios

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	authmocks "github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/features/session/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

const otherSession = "00000000-0000-0000-0000-000000000001"

func SelfRevocation() []testutils.Scenario {
	return []testutils.Scenario{{Name: "self revoke uses locked authority and atomic writes with complete rollback", Run: selfMatrix}}
}

func selfMatrix(t *testing.T) {
	for _, mode := range []string{"single", "global"} {
		path := []string{"begin", "lock:auth:user:42", "lock:auth:session:" + otherSession, "lock:auth:session:" + fixtures.SessionID, "bind", "state", "target", "sessions", "refresh", "audit", "event-id", "outbox", "commit"}
		method := sessionv1.SessionService_RevokeMySession_FullMethodName
		if mode == "global" {
			path = []string{"begin", "lock:auth:user:42", "bind", "ids", "lock:auth:session:" + otherSession, "lock:auth:session:" + fixtures.SessionID, "state", "counters", "sessions", "refresh", "audit", "event-id", "outbox", "commit"}
			method = sessionv1.SessionService_RevokeAllMySessions_FullMethodName
		}
		cases := []string{"success", "restricted", "locked restricted", "stale principal", "missing principal", "missing caller", "busy", "incomplete", "zero sessions", "overflow", "bad id", "error:begin", "error:lock", "error:commit"}
		if mode == "single" {
			cases = append(cases, "current", "terminal", "foreign", "wrong target")
		} else {
			cases = append(cases, "zero counters", "invalid discovered id")
		}
		for _, stage := range path {
			if stage != "begin" && stage != "bind" && stage != "commit" && !strings.HasPrefix(stage, "lock:") {
				for _, fail := range []string{"error", "cancel", "panic"} {
					cases = append(cases, fail+":"+stage)
				}
			}
		}
		for _, kind := range cases {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx := fixtures.CallerContext(t, context.Background(), method, method)
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
					p, err := sessions.PrincipalForState(fixtures.AccessClaims(t), state, fixtures.ServiceTime)
					if err != nil {
						t.Fatal(err)
					}
					ctx = p.Context(ctx)
				}
				tx := &shared.Transaction{Context: ctx}
				provider := &shared.TransactionProvider{Tx: tx}
				cause := errors.New("controlled private dependency error")
				step := func(name string) error {
					tx.Record(name)
					switch kind {
					case "error:" + name:
						return cause
					case "cancel:" + name:
						cancel()
						return context.Canceled
					case "panic:" + name:
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
				id := otherSession
				if kind == "current" {
					id = fixtures.SessionID
				}
				if kind == "bad id" {
					id = "invalid"
				}
				d := session.SelfDependencies{Transactions: provider, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }}
				d.Bind = func(handle domain.Transaction) session.SelfRepositories {
					if handle != tx {
						t.Fatal("transaction binding")
					}
					tx.Record("bind")
					if kind == "incomplete" {
						return session.SelfRepositories{}
					}
					return session.SelfRepositories{
						State: authmocks.SessionStateReader{Read: func(got context.Context, sid string) (domain.SessionState, error) {
							if got != ctx || sid != fixtures.SessionID {
								t.Fatal("state scope/context")
							}
							fresh := state
							if kind == "stale principal" {
								fresh.AuthVersion++
							}
							if kind == "locked restricted" {
								fresh.MustChangePassword = true
							}
							return fresh, step("state")
						}},
						Targets: mocks.Targets{Get: func(got context.Context, sid string) (domain.SessionSummary, error) {
							if got != ctx || sid != id {
								t.Fatal("target scope")
							}
							target := domain.SessionSummary{ID: id, UserID: 42, Status: types.SessionActive}
							if kind == "foreign" {
								target.UserID = 43
							}
							if kind == "wrong target" {
								target.ID = fixtures.SessionID
							}
							if kind == "terminal" {
								target.Status = types.SessionRevoked
							}
							return target, step("target")
						}, IDs: func(got context.Context, uid uint64) ([]string, error) {
							if got != ctx || uid != 42 {
								t.Fatal("IDs scope")
							}
							ids := []string{fixtures.SessionID, otherSession, fixtures.SessionID}
							if kind == "invalid discovered id" {
								ids = append(ids, "invalid")
							}
							return ids, step("ids")
						}},
						Counters: mocks.Counters{Write: func(got context.Context, c domain.CredentialInvalidation) (bool, error) {
							if got != ctx || c.UserID != 42 || c.ExpectedVersion != state.UserVersion || c.ExpectedAuthVersion != 3 || !c.At.Equal(fixtures.ServiceTime) {
								t.Fatal("counter arguments")
							}
							return kind != "zero counters", step("counters")
						}},
						Sessions: authmocks.SessionRevocation{Single: func(got context.Context, r domain.SessionRevocation) (bool, error) {
							if got != ctx || r.Validate() != nil || r.UserID != 42 || r.ActorID != 42 || r.SessionID != id || r.Reason != types.AuditSelfSessionRevoked {
								t.Fatal("single scope")
							}
							return kind != "zero sessions", step("sessions")
						}, All: func(got context.Context, r domain.SessionRevocation) (int64, error) {
							if got != ctx || r.Validate() != nil || r.UserID != 42 || r.ActorID != 42 || r.SessionID != "" || r.Reason != types.AuditSelfSessionsRevoked {
								t.Fatal("all scope")
							}
							n := int64(2)
							if kind == "zero sessions" {
								n = 0
							}
							return n, step("sessions")
						}},
						Refresh: authmocks.RefreshRevocation{Session: func(got context.Context, sid string, at time.Time) (int64, error) {
							if got != ctx || sid != id || !at.Equal(fixtures.ServiceTime) {
								t.Fatal("refresh session scope")
							}
							return 2, step("refresh")
						}, All: func(got context.Context, uid uint64, at time.Time) (int64, error) {
							if got != ctx || uid != 42 || !at.Equal(fixtures.ServiceTime) {
								t.Fatal("refresh global scope")
							}
							return 3, step("refresh")
						}},
						Audit: authmocks.AuditWriter{Write: func(r domain.AuditRequest) error {
							record, err := r.Record()
							if err != nil || record.ActorUserID == nil || *record.ActorUserID != 42 {
								t.Fatal("audit", err)
							}
							if mode == "single" && (r.TargetID != id || r.Action != types.AuditSessionRevoked || r.Reason != types.AuditSelfSessionRevoked) {
								t.Fatal("single audit")
							}
							if mode == "global" && (r.TargetID != "42" || r.Action != types.AuditUserSessionsRevoked || r.After.Version != state.UserVersion+1 || r.Reason != types.AuditSelfSessionsRevoked) {
								t.Fatal("global audit")
							}
							return step("audit")
						}},
						Outbox: shared.OutboxWriter{Write: func(e domain.EventEnvelope) error {
							r, err := e.Record()
							if err != nil || r.PublishedAt != nil || e.ActorUserID == nil || *e.ActorUserID != 42 {
								t.Fatal("event", err)
							}
							if mode == "single" && (e.AggregateID != id || e.AggregateVersion != 2 || e.EventType != types.EventSessionRevoked) {
								t.Fatal("single event")
							}
							if mode == "global" && (e.AggregateID != "42" || e.AggregateVersion != state.UserVersion+1 || e.EventType != types.EventUserSessionsRevoked || e.Data.SessionsRevoked == nil || !*e.Data.SessionsRevoked) {
								t.Fatal("global event")
							}
							return step("outbox")
						}},
					}
				}
				svc, err := session.NewSelfService(d)
				if err != nil {
					t.Fatal(err)
				}
				var changed, login bool
				if mode == "single" {
					r, e := svc.RevokeMySession(ctx, id)
					err = e
					if r != nil {
						changed = r.Changed
					}
				} else {
					r, e := svc.RevokeAllMySessions(ctx)
					err = e
					if r != nil {
						changed = r.Changed
						login = r.LoginRequired
					}
				}
				want := append([]string{}, path...)
				if kind == "current" {
					want = append(want[:2], want[3:]...)
				}
				if kind == "terminal" {
					want = append(want[:7], "commit")
				}
				stop := map[string]string{"busy": "lock:auth:user:42", "error:lock": "lock:auth:user:42", "error:begin": "begin", "error:commit": "commit", "incomplete": "bind", "stale principal": "state", "locked restricted": "state", "foreign": "target", "wrong target": "target", "zero counters": "counters", "zero sessions": "sessions", "invalid discovered id": "ids"}[kind]
				if kind == "overflow" && mode == "global" {
					stop = "state"
				}
				for _, stage := range path {
					if kind == "error:"+stage || kind == "panic:"+stage || kind == "cancel:"+stage {
						stop = stage
					}
				}
				noCalls := kind == "restricted" || kind == "missing principal" || kind == "missing caller" || kind == "bad id" && mode == "single"
				if noCalls {
					want = nil
				} else if stop != "" {
					for n, name := range want {
						if name == stop {
							want = want[:n+1]
							if name != "begin" {
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
					if err == nil || changed || login {
						t.Fatal("error outcome", changed, login, err)
					}
					if strings.HasPrefix(kind, "error:") && !errors.Is(err, cause) {
						t.Fatal("lost cause", err)
					}
					if strings.HasPrefix(kind, "panic:") && !errors.Is(err, common.ErrInternalServerError) {
						t.Fatal("panic outcome", err)
					}
					if strings.HasPrefix(kind, "cancel:") && !errors.Is(err, context.Canceled) {
						t.Fatal("cancel outcome", err)
					}
					if kind == "busy" && !errors.Is(err, auth.ErrResourceBusy) {
						t.Fatal("busy outcome", err)
					}
					if kind == "zero counters" && !errors.Is(err, user.ErrVersionConflict) {
						t.Fatal("version conflict", err)
					}
					if (kind == "foreign" || kind == "wrong target" || kind == "restricted" || kind == "locked restricted") && !errors.Is(err, common.ErrForbidden) {
						t.Fatal("ownership/restriction", err)
					}
					return
				}
				if err != nil || changed != (kind != "terminal") || login != (mode == "global") {
					t.Fatal("result", changed, login, err)
				}
			})
		}
	}
	if _, err := session.NewSelfService(session.SelfDependencies{}); err == nil {
		t.Fatal("incomplete dependencies accepted")
	}
}
