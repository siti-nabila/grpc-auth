package test_scenarios

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	authmocks "github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/features/session/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"reflect"
	"strings"
	"testing"
	"time"
)

func AdminRevocation() []testutils.Scenario {
	return []testutils.Scenario{{Name: "admin serialization authority precedes safe replay target guards and atomic mutation", Run: adminMatrix}}
}

func adminMatrix(t *testing.T) {
	for _, mode := range []string{"single", "global"} {
		method := sessionv1.SessionService_RevokeUserSession_FullMethodName
		target := "user/43/session/" + otherSession
		payload := featurecommon.FingerprintPayload{}
		if mode == "global" {
			method = sessionv1.SessionService_RevokeAllUserSessions_FullMethodName
			target = "user/43/sessions"
			payload.ExpectedVersion = 7
			payload.Presence = []string{"expected_user_version"}
		}
		scope := domain.IdempotencyScope{ActorID: 42, Method: method, Target: target, Key: "test-key"}
		hash, e := featurecommon.Fingerprint(scope, payload)
		if e != nil {
			t.Fatal(e)
		}
		lock, e := featurecommon.IdempotencyLockKey(scope)
		if e != nil {
			t.Fatal(e)
		}
		path := []string{"begin", "lock:" + lock, "lock:auth:guard:policy:global", "lock:auth:user:42", "lock:auth:user:43", "bind", "discover permissions"}
		if mode == "global" {
			path = append(path, "ids")
		}
		path = append(path, "lock:auth:role:2", "lock:auth:application:1", "lock:auth:session:"+otherSession, "lock:auth:session:"+fixtures.SessionID, "state", "permissions", "lookup", "target")
		if mode == "single" {
			path = append(path, "target session")
		} else {
			path = append(path, "counters")
		}
		path = append(path, "sessions", "refresh", "audit", "event-id", "outbox", "save", "commit")
		cases := []string{"success", "restricted", "locked restricted", "stale actor", "lost permission", "wrong permission", "foreign target", "deleted target", "missing target", "replay", "conflict", "expired", "wrong replay scope", "zero sessions", "busy", "missing key", "long key", "missing principal", "missing caller", "incomplete", "error:begin", "error:lock", "error:commit"}
		if mode == "single" {
			cases = append(cases, "terminal", "foreign session", "wrong session", "missing session", "bad session")
		} else {
			cases = append(cases, "stale version", "zero counters", "no active sessions", "invalid discovered ID", "missing version")
		}
		for _, stage := range path {
			if stage != "begin" && stage != "bind" && stage != "commit" && !strings.HasPrefix(stage, "lock:") {
				for _, failure := range []string{"error", "cancel", "panic"} {
					cases = append(cases, failure+":"+stage)
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
				key := scope.Key
				if kind == "missing key" {
					key = ""
				}
				if kind == "long key" {
					key = strings.Repeat("x", 201)
				}
				id := otherSession
				if kind == "bad session" {
					id = "bad"
				}
				expected := int64(7)
				if kind == "missing version" {
					expected = 0
				}
				var record *domain.IdempotencyRecord
				if kind == "replay" || kind == "expired" || kind == "conflict" || kind == "wrong replay scope" {
					r := domain.SafeCommandResult{UserID: 43, SessionID: otherSession, Version: 7, Changed: true}
					if mode == "global" {
						r.SessionID = ""
						r.Version = 8
					}
					if kind == "wrong replay scope" {
						r.UserID = 44
					}
					data, _ := json.Marshal(r)
					record = &domain.IdempotencyRecord{RequestHash: hash, SafeResponse: string(data), CreatedAt: fixtures.ServiceTime.Add(-time.Hour), ExpiresAt: fixtures.ServiceTime.Add(time.Hour)}
					if kind == "expired" {
						record.ExpiresAt = fixtures.ServiceTime
					}
					if kind == "conflict" {
						record.RequestHash = []byte("different-request")
					}
				}
				permissionCalls := 0
				d := session.AdminDependencies{Transactions: provider, Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }}
				d.Bind = func(handle domain.Transaction) session.AdminRepositories {
					if handle != tx {
						t.Fatal("transaction binding")
					}
					tx.Record("bind")
					if kind == "incomplete" {
						return session.AdminRepositories{}
					}
					return session.AdminRepositories{
						State: authmocks.SessionStateReader{Read: func(got context.Context, sid string) (domain.SessionState, error) {
							if got != ctx || sid != fixtures.SessionID {
								t.Fatal("actor state scope")
							}
							fresh := state
							if kind == "stale actor" {
								fresh.AuthVersion++
							}
							if kind == "locked restricted" {
								fresh.MustChangePassword = true
							}
							return fresh, step("state")
						}},
						Users: mocks.Users{Read: func(got context.Context, uid uint64) (domain.SessionUserState, error) {
							if got != ctx || uid != 43 {
								t.Fatal("target scope")
							}
							r := domain.SessionUserState{ID: 43, Status: types.UserActive, Version: 7, AuthVersion: 9}
							if kind == "foreign target" {
								r.ID = 44
							}
							if kind == "deleted target" {
								r.DeletedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
							}
							if kind == "stale version" {
								r.Version = 8
							}
							if e := step("target"); e != nil {
								return r, e
							}
							if kind == "missing target" {
								return r, auth.ErrNotFound
							}
							return r, nil
						}},
						Permissions: mocks.Permissions{Read: func(got context.Context, uid uint64) ([]domain.PermissionContribution, error) {
							if got != ctx || uid != 42 {
								t.Fatal("permission scope")
							}
							permissionCalls++
							stage := "permissions"
							if permissionCalls == 1 {
								stage = "discover permissions"
							}
							g := listGrant()
							g.PermissionCode = string(types.PermissionIAMSessionRevoke)
							if kind == "wrong permission" {
								g.PermissionCode = "iam.user.read"
							}
							if kind == "lost permission" && permissionCalls == 2 {
								return nil, step(stage)
							}
							return []domain.PermissionContribution{g, g}, step(stage)
						}},
						Targets: mocks.Targets{IDs: func(got context.Context, uid uint64) ([]string, error) {
							if got != ctx || uid != 43 {
								t.Fatal("discovery scope")
							}
							ids := []string{otherSession}
							if kind == "invalid discovered ID" {
								ids = append(ids, "bad")
							}
							return ids, step("ids")
						}, Get: func(got context.Context, sid string) (domain.SessionSummary, error) {
							if got != ctx || sid != id {
								t.Fatal("target session scope")
							}
							r := domain.SessionSummary{ID: id, UserID: 43, Status: types.SessionActive}
							if kind == "terminal" {
								r.Status = types.SessionRevoked
							}
							if kind == "foreign session" {
								r.UserID = 44
							}
							if kind == "wrong session" {
								r.ID = fixtures.SessionID
							}
							if e := step("target session"); e != nil {
								return r, e
							}
							if kind == "missing session" {
								return r, auth.ErrNotFound
							}
							return r, nil
						}},
						Counters: mocks.Counters{Write: func(got context.Context, c domain.CredentialInvalidation) (bool, error) {
							if got != ctx || c.Validate() != nil || c.UserID != 43 || c.ExpectedVersion != 7 || c.ExpectedAuthVersion != 9 {
								t.Fatal("target counter mutation")
							}
							return kind != "zero counters", step("counters")
						}},
						Sessions: authmocks.SessionRevocation{Single: func(got context.Context, r domain.SessionRevocation) (bool, error) {
							if got != ctx || r.UserID != 43 || r.ActorID != 42 || r.SessionID != id || r.Reason != types.AuditAdminSessionRevoked || r.Validate() != nil {
								t.Fatal("single session mutation")
							}
							return kind != "zero sessions", step("sessions")
						}, All: func(got context.Context, r domain.SessionRevocation) (int64, error) {
							if got != ctx || r.UserID != 43 || r.ActorID != 42 || r.SessionID != "" || r.Reason != types.AuditAdminSessionsRevoked || r.Validate() != nil {
								t.Fatal("global session mutation")
							}
							count := int64(2)
							if kind == "zero sessions" || kind == "no active sessions" {
								count = 0
							}
							return count, step("sessions")
						}},
						Refresh: authmocks.RefreshRevocation{Session: func(got context.Context, sid string, at time.Time) (int64, error) {
							if got != ctx || sid != id || !at.Equal(fixtures.ServiceTime) {
								t.Fatal("single refresh scope")
							}
							return 2, step("refresh")
						}, All: func(got context.Context, uid uint64, at time.Time) (int64, error) {
							if got != ctx || uid != 43 || !at.Equal(fixtures.ServiceTime) {
								t.Fatal("global refresh scope")
							}
							return 3, step("refresh")
						}},
						Audit: authmocks.AuditWriter{Write: func(r domain.AuditRequest) error {
							a, e := r.Record()
							if e != nil || a.ActorUserID == nil || *a.ActorUserID != 42 {
								t.Fatal("actor audit", e)
							}
							if mode == "global" {
								if r.Action != types.AuditUserSessionsRevoked || r.TargetID != "43" || r.After.Version != 8 || r.Reason != types.AuditAdminSessionsRevoked {
									t.Fatal("global audit")
								}
							} else {
								action := types.AuditSessionRevoked
								if kind == "terminal" {
									action = types.AuditNoOp
								}
								if r.Action != action || r.After.NoOp != (kind == "terminal") || r.TargetID != id || r.Reason != types.AuditAdminSessionRevoked {
									t.Fatal("single audit")
								}
							}
							return step("audit")
						}},
						Outbox: shared.OutboxWriter{Write: func(e domain.EventEnvelope) error {
							r, err := e.Record()
							if err != nil || e.ActorUserID == nil || *e.ActorUserID != 42 || e.Data.UserID != 43 || r.PublishedAt != nil {
								t.Fatal("cross-user event actor", err)
							}
							if mode == "single" && (e.AggregateID != id || e.AggregateVersion != 2) {
								t.Fatal("terminal event")
							}
							if mode == "global" && (e.AggregateID != "43" || e.AggregateVersion != 8 || e.Data.ReasonCode != "ADMIN_SESSIONS_REVOKED" || e.Data.SessionsRevoked == nil || !*e.Data.SessionsRevoked) {
								t.Fatal("global event")
							}
							return step("outbox")
						}},
						Idempotency: mocks.Idempotency{Read: func(got domain.IdempotencyScope) (*domain.IdempotencyRecord, error) {
							if got != scope {
								t.Fatal("lookup scope")
							}
							return record, step("lookup")
						}, Write: func(got domain.IdempotencyScope, h []byte, r domain.SafeCommandResult, at time.Time) error {
							if got != scope || !bytes.Equal(h, hash) || !at.Equal(fixtures.ServiceTime) || r.Validate() != nil || r.UserID != 43 || r.Changed != (kind != "terminal") {
								t.Fatal("saved safe scope/result")
							}
							if mode == "single" && (r.SessionID != id || r.Version != 7) {
								t.Fatal("single saved result")
							}
							if mode == "global" && (r.SessionID != "" || r.Version != 8) {
								t.Fatal("global saved result")
							}
							return step("save")
						}},
					}
				}
				svc, e := session.NewAdminService(d)
				if e != nil {
					t.Fatal(e)
				}
				var res *session.AdminResult
				var err error
				if mode == "single" {
					res, err = svc.RevokeUserSession(ctx, session.AdminSingleInput{UserID: 43, SessionID: id, Key: key})
				} else {
					res, err = svc.RevokeAllUserSessions(ctx, session.AdminGlobalInput{UserID: 43, ExpectedVersion: expected, Key: key})
				}
				want := append([]string{}, path...)
				if kind == "terminal" {
					for n, stage := range want {
						if stage == "sessions" {
							want = append(append([]string{}, want[:n]...), "audit", "save", "commit")
							break
						}
					}
				}
				if kind == "replay" {
					for n, stage := range want {
						if stage == "lookup" {
							want = append(want[:n+1], "commit")
							break
						}
					}
				}
				stop := map[string]string{"locked restricted": "state", "stale actor": "state", "lost permission": "permissions", "wrong permission": "permissions", "foreign target": "target", "deleted target": "target", "missing target": "target", "foreign session": "target session", "wrong session": "target session", "missing session": "target session", "conflict": "lookup", "wrong replay scope": "lookup", "zero counters": "counters", "stale version": "target", "invalid discovered ID": "ids", "incomplete": "bind", "busy": "lock:" + lock, "error:lock": "lock:" + lock, "error:begin": "begin", "error:commit": "commit"}[kind]
				if kind == "zero sessions" && mode == "single" {
					stop = "sessions"
				}
				for _, stage := range path {
					if kind == "error:"+stage || kind == "cancel:"+stage || kind == "panic:"+stage {
						stop = stage
					}
				}
				noCalls := kind == "restricted" || kind == "missing caller" || kind == "missing principal" || kind == "missing key" || kind == "long key" || kind == "bad session" || kind == "missing version"
				if noCalls {
					want = nil
				} else if stop != "" {
					for n, stage := range want {
						if stage == stop {
							want = want[:n+1]
							if stage != "begin" {
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
						t.Fatal("failure result", err)
					}
					if strings.HasPrefix(kind, "error:") && !errors.Is(err, cause) {
						t.Fatal("cause lost", err)
					}
					if strings.HasPrefix(kind, "cancel:") && !errors.Is(err, context.Canceled) {
						t.Fatal("cancel outcome", err)
					}
					if strings.HasPrefix(kind, "panic:") && !errors.Is(err, common.ErrInternalServerError) {
						t.Fatal("panic outcome", err)
					}
					if kind == "conflict" && !errors.Is(err, user.ErrIdempotencyConflict) {
						t.Fatal("conflict catalog", err)
					}
					if kind == "stale version" || kind == "zero counters" {
						if !errors.Is(err, user.ErrVersionConflict) {
							t.Fatal("version catalog", err)
						}
					}
					if kind == "busy" && !errors.Is(err, auth.ErrResourceBusy) {
						t.Fatal("busy outcome", err)
					}
					return
				}
				version := int64(7)
				if mode == "global" {
					version = 8
				}
				if err != nil || res == nil || res.Changed != (kind != "terminal") || res.Version != version {
					t.Fatal("admin result", res, err)
				}
			})
		}
	}
	if _, err := session.NewAdminService(session.AdminDependencies{}); err == nil {
		t.Fatal("incomplete constructor")
	}
}
