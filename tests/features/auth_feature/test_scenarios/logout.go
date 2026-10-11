package test_scenarios

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/metadata"
)

func Logout() []testutils.Scenario {
	return []testutils.Scenario{{Name: "Logout access and latest refresh prove owner generation then commit one terminal transition", Run: logoutMatrix}}
}
func logoutMatrix(t *testing.T) {
	for _, mode := range []string{"access", "refresh"} {
		path := []string{"candidate-state", "begin", "lock:auth:user:42", "lock:auth:session:" + fixtures.SessionID, "bind", "locked-state", "revoke-session", "revoke-refresh", "audit", "event-id", "outbox", "commit"}
		if mode == "refresh" {
			path = append([]string{"candidate-token"}, path...)
			path = append(append(append([]string{}, path[:6]...), "locked-token"), path[6:]...)
		}
		cases := []string{"success", "restricted", "terminal", "consumed", "revoked refresh active session", "stale generation", "unknown generation", "owner", "expired", "zero transition", "unknown", "missing state", "busy", "changed generation", "changed owner"}
		for _, stage := range path {
			if stage != "bind" && stage != "begin" && stage != "commit" && !strings.HasPrefix(stage, "lock:") {
				for _, failure := range []string{"error", "panic", "cancel"} {
					cases = append(cases, failure+":"+stage)
				}
			}
		}
		cases = append(cases, "error:begin", "error:lock", "error:commit")
		for _, kind := range cases {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx, cancel := context.WithCancel(fixtures.CallerContext(t, context.Background(), authv1.AuthService_Logout_FullMethodName, authv1.AuthService_Logout_FullMethodName))
				defer cancel()
				c, err := authfeature.GenerateInitialCredentials(bytes.NewReader(bytes.Repeat([]byte{1}, 96)))
				if err != nil {
					t.Fatal(err)
				}
				v, err := jwt.NewAccessVerifier(fixtures.AccessIssuer, fixtures.AccessKey().Public().(ed25519.PublicKey), func() time.Time { return fixtures.ServiceTime })
				if err != nil {
					t.Fatal(err)
				}
				var raw *string
				if mode == "refresh" {
					raw = &c.RefreshToken
				} else {
					md, _ := metadata.FromIncomingContext(ctx)
					md = md.Copy()
					md.Set("authorization", "Bearer "+fixtures.SignAccess(t, fixtures.AccessPayload()))
					ctx = metadata.NewIncomingContext(ctx, md)
				}
				proof, err := sessions.VerifyLogoutCredential(ctx, raw, v)
				if err != nil {
					t.Fatal(err)
				}
				tx := &shared.Transaction{Context: ctx}
				provider := &shared.TransactionProvider{Tx: tx}
				cause := errors.New("controlled private failure")
				step := func(name string) error {
					tx.Record(name)
					if kind == "panic:"+name {
						panic("private panic")
					}
					if kind == "cancel:"+name {
						cancel()
						return context.Canceled
					}
					if kind == "error:"+name {
						return cause
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
				state := fixtures.SessionState()
				token := domain.InitialRefresh{ID: c.RefreshID, SessionID: fixtures.SessionID, TokenHash: c.RefreshDigest, FamilyID: c.FamilyID, IssuedAt: state.CreatedAt, ExpiresAt: state.ExpiresAt}
				switch kind {
				case "restricted":
					state.MustChangePassword = true
				case "terminal":
					state.SessionStatus = types.SessionRevoked
					state.RevokedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
					token.RevokedAt = state.RevokedAt
				case "consumed":
					token.ConsumedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
				case "revoked refresh active session":
					token.RevokedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
				case "stale generation":
					state.AuthVersion++
				case "unknown generation":
					state.SessionAuthVersion = sql.NullInt64{}
				case "owner":
					state.SessionUserID = 43
				case "expired":
					state.ExpiresAt = fixtures.ServiceTime
				}
				readState := func(name string) mocks.SessionStateReader {
					return mocks.SessionStateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
						if got != ctx || id != fixtures.SessionID {
							t.Fatal("state scope/context")
						}
						if err := step(name); err != nil {
							return state, err
						}
						if kind == "missing state" {
							return state, auth.ErrNotFound
						}
						s := state
						if name == "locked-state" && kind == "changed generation" {
							s.AuthVersion++
						}
						if name == "locked-state" && kind == "changed owner" {
							s.UserID = 43
							s.SessionUserID = 43
						}
						return s, nil
					}}
				}
				readToken := func(name string) mocks.RefreshReader {
					return mocks.RefreshReader{Read: func(got context.Context, digest []byte) (domain.InitialRefresh, error) {
						if got != ctx || !bytes.Equal(digest, c.RefreshDigest) {
							t.Fatal("token scope/context")
						}
						if err := step(name); err != nil {
							return token, err
						}
						if kind == "unknown" {
							return token, auth.ErrNotFound
						}
						return token, nil
					}}
				}
				d := authfeature.LogoutDependencies{Transactions: provider, Tokens: readToken("candidate-token"), State: readState("candidate-state"), Now: func() time.Time { return fixtures.ServiceTime }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }}
				d.Bind = func(handle domain.Transaction) authfeature.LogoutRepositories {
					if handle != tx {
						t.Fatal("handle")
					}
					tx.Record("bind")
					return authfeature.LogoutRepositories{Tokens: readToken("locked-token"), State: readState("locked-state"), Sessions: mocks.SessionRevocation{Single: func(got context.Context, r domain.SessionRevocation) (bool, error) {
						if got != ctx || r.Validate() != nil || r.UserID != 42 || r.ActorID != 42 || r.SessionID != fixtures.SessionID || r.Reason != types.AuditLogout {
							t.Fatal("transition scope")
						}
						return kind != "terminal" && kind != "zero transition", step("revoke-session")
					}}, Refresh: mocks.RefreshRevocation{Session: func(got context.Context, id string, at time.Time) (int64, error) {
						if got != ctx || id != fixtures.SessionID || !at.Equal(fixtures.ServiceTime) {
							t.Fatal("family scope")
						}
						return 2, step("revoke-refresh")
					}}, Audit: mocks.AuditWriter{Write: func(r domain.AuditRequest) error {
						record, err := r.Record()
						if err != nil || record.ActorUserID == nil || *record.ActorUserID != 42 || record.Action != "SESSION_REVOKED" || record.Reason == nil || *record.Reason != "LOGOUT" {
							t.Fatal("safe audit", err)
						}
						return step("audit")
					}}, Outbox: shared.OutboxWriter{Write: func(e domain.EventEnvelope) error {
						record, err := e.Record()
						if err != nil || e.AggregateID != fixtures.SessionID || e.AggregateVersion != 2 || e.Data.ReasonCode != "LOGOUT" || record.PublishedAt != nil {
							t.Fatal("safe event", err)
						}
						return step("outbox")
					}}}
				}
				svc, err := authfeature.NewLogoutService(d)
				if err != nil {
					t.Fatal(err)
				}
				res, err := svc.Logout(ctx, proof)
				want := append([]string{}, path...)
				if kind == "terminal" {
					for n, name := range path {
						if name == "revoke-session" {
							want = append(append([]string{}, path[:n+1]...), "commit")
							break
						}
					}
				}
				stops := map[string]string{"stale generation": "candidate-state", "unknown generation": "candidate-state", "owner": "candidate-state", "expired": "candidate-state", "missing state": "candidate-state", "zero transition": "revoke-session", "changed generation": "locked-state", "changed owner": "locked-state", "busy": "lock:auth:user:42", "error:lock": "lock:auth:user:42", "error:begin": "begin"}
				if mode == "refresh" {
					stops["unknown"] = "candidate-token"
					stops["consumed"] = "candidate-state"
					stops["revoked refresh active session"] = "candidate-state"
				}
				stop, stopped := stops[kind]
				if !stopped {
					for _, name := range path {
						if kind == "error:"+name || kind == "panic:"+name || kind == "cancel:"+name {
							stop = name
							stopped = true
							break
						}
					}
				}
				if stopped {
					for n, name := range path {
						if name == stop {
							want = append([]string{}, path[:n+1]...)
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
				if stopped {
					if res != nil || err == nil {
						t.Fatal("failure outcome")
					}
					if strings.HasPrefix(kind, "error:") && !errors.Is(err, cause) {
						t.Fatal("cause lost", err)
					}
					if strings.HasPrefix(kind, "panic:") && !errors.Is(err, common.ErrInternalServerError) {
						t.Fatal("panic lost", err)
					}
					if strings.HasPrefix(kind, "cancel:") && !errors.Is(err, context.Canceled) {
						t.Fatal("cancel lost", err)
					}
					return
				}
				if err != nil || res == nil || res.SessionID != fixtures.SessionID || res.Changed != (kind != "terminal") {
					t.Fatal("Logout result", res, err)
				}
				if kind == "terminal" {
					if _, err := sessions.PrincipalForState(fixtures.AccessClaims(t), state, fixtures.ServiceTime); !errors.Is(err, auth.ErrInvalidSession) {
						t.Fatal("terminal Logout proof bypassed normal principal")
					}
				}
			})
		}
	}
	if _, err := authfeature.NewLogoutService(authfeature.LogoutDependencies{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
}
func indexOf(values []string, want string) int {
	for n, value := range values {
		if value == want {
			return n
		}
	}
	return -1
}
