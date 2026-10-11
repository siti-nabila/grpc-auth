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
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Refresh() []testutils.Scenario {
	return []testutils.Scenario{{Name: "rotation and reuse preserve exact proof lock write commit and rollback ordering", Run: refreshMatrix}}
}

func refreshMatrix(t *testing.T) {
	rotation := []string{"candidate", "candidate-state", "admit", "begin", "lock:auth:user:42", "lock:auth:session:" + fixtures.SessionID, "bind", "locked-token", "locked-state", "entropy", "insert", "consume", "rotation-audit", "sign", "commit"}
	reuse := append(append([]string{}, rotation[:9]...), "revoke-session", "revoke-family", "reuse-audit", "terminal-audit", "event-id", "outbox", "commit")
	for _, mode := range []string{"rotation", "reuse"} {
		cases := []string{"success", "busy", "zero consume", "terminal", "near expiry", "restricted", "unknown", "expired", "revoked", "deleted", "owner", "changed owner", "changed family", "empty credential", "malformed", "empty issued", "unverified", "canceled entry", "PENDING", "SUSPENDED", "DEACTIVATED", "expired session", "long session", "future session", "expired token", "extended token", "zero version", "zero auth version", "missing session", "wrong digest", "busy session", "unknown generation", "changed generation"}
		path := rotation
		if mode == "reuse" {
			path = reuse
		}
		for _, stage := range path {
			if stage != "bind" && !strings.HasPrefix(stage, "lock:") && stage != "begin" && stage != "commit" {
				for _, failure := range []string{"error", "panic", "cancel"} {
					cases = append(cases, failure+":"+stage)
				}
			}
		}
		cases = append(cases, "error:begin", "error:lock", "error:commit", "panic:commit", "cancel:commit")
		for _, kind := range cases {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				ctx, cancel := context.WithCancel(fixtures.CallerContext(t, context.Background(), authv1.AuthService_RefreshSession_FullMethodName, authv1.AuthService_RefreshSession_FullMethodName))
				defer cancel()
				if kind == "unverified" {
					ctx = context.Background()
				}
				cause := errors.New("controlled private failure")
				tx := &shared.Transaction{Context: ctx}
				provider := &shared.TransactionProvider{Tx: tx}
				step := func(name string) error {
					tx.Record(name)
					if kind == "panic:"+name {
						panic("private failure")
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
				if kind == "busy session" {
					tx.Hook = func(call string) {
						if call == "lock:auth:session:"+fixtures.SessionID {
							tx.Busy = true
						}
					}
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
				if kind == "panic:commit" || kind == "cancel:commit" {
					tx.Hook = func(call string) {
						if call == "commit" {
							if kind == "panic:commit" {
								panic("private commit")
							}
							cancel()
						}
					}
				}
				oldC, err := authfeature.GenerateInitialCredentials(bytes.NewReader(bytes.Repeat([]byte{1}, 96)))
				if err != nil {
					t.Fatal(err)
				}
				newC, err := authfeature.GenerateRotatedCredentials(bytes.NewReader(bytes.Repeat([]byte{2}, 64)))
				if err != nil {
					t.Fatal(err)
				}
				state := fixtures.SessionState()
				old := domain.InitialRefresh{ID: oldC.RefreshID, SessionID: fixtures.SessionID, TokenHash: oldC.RefreshDigest, FamilyID: oldC.FamilyID, IssuedAt: state.CreatedAt, ExpiresAt: state.ExpiresAt}
				if mode == "reuse" {
					old.ConsumedAt = sql.NullTime{Time: fixtures.ServiceTime.Add(-time.Minute), Valid: true}
					old.ReplacedBy = sql.NullString{String: newC.RefreshID, Valid: true}
				}
				switch kind {
				case "PENDING", "SUSPENDED", "DEACTIVATED":
					state.UserStatus = types.UserStatus(kind)
				case "expired session":
					state.ExpiresAt = fixtures.ServiceTime
				case "long session":
					state.ExpiresAt = state.CreatedAt.Add(8 * 24 * time.Hour)
				case "future session":
					state.CreatedAt = fixtures.ServiceTime.Add(time.Second)
				case "expired token":
					old.ExpiresAt = fixtures.ServiceTime
				case "extended token":
					old.ExpiresAt = state.ExpiresAt.Add(time.Second)
				case "unknown generation":
					state.SessionAuthVersion = sql.NullInt64{}
				case "changed generation":
					state.AuthVersion++
				case "zero version":
					state.UserVersion = 0
				case "zero auth version":
					state.AuthVersion = 0
				case "wrong digest":
					old.TokenHash = bytes.Repeat([]byte{0}, 32)
				case "near expiry":
					state.ExpiresAt = fixtures.ServiceTime.Add(time.Minute)
					old.ExpiresAt = state.ExpiresAt
				case "restricted":
					state.MustChangePassword = true
				case "expired":
					old.ExpiresAt = fixtures.ServiceTime
				case "revoked":
					old.RevokedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
				case "deleted":
					state.DeletedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
				case "owner":
					state.SessionUserID = 43
				case "terminal":
					state.SessionStatus = types.SessionRevoked
					state.RevokedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
				}
				readToken := func(name string) mocks.RefreshReader {
					return mocks.RefreshReader{Read: func(got context.Context, d []byte) (domain.InitialRefresh, error) {
						if got != ctx || !bytes.Equal(d, oldC.RefreshDigest) {
							t.Fatal("refresh identity/context")
						}
						if err := step(name); err != nil {
							return old, err
						}
						if kind == "unknown" {
							return old, auth.ErrNotFound
						}
						r := old
						if name == "locked-token" && kind == "changed family" {
							r.FamilyID = newC.TokenID
						}
						return r, nil
					}}
				}
				readState := func(name string) mocks.SessionStateReader {
					return mocks.SessionStateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
						if got != ctx || id != fixtures.SessionID {
							t.Fatal("state args")
						}
						if err := step(name); err != nil {
							return state, err
						}
						if kind == "missing session" {
							return state, auth.ErrNotFound
						}
						r := state
						if name == "locked-state" && kind == "changed owner" {
							r.UserID = 43
							r.SessionUserID = 43
						}
						return r, nil
					}}
				}
				issuer, err := jwt.NewAccessIssuer(fixtures.AccessIssuer, fixtures.AccessKey())
				if err != nil {
					t.Fatal(err)
				}
				deps := authfeature.RefreshDependencies{Transactions: provider, Tokens: readToken("candidate"), State: readState("candidate-state"), Now: func() time.Time { return fixtures.ServiceTime }, Admit: func(got context.Context, id string) error {
					if got != ctx || id != fixtures.SessionID {
						t.Fatal("unproven admission")
					}
					return step("admit")
				}, Credentials: func() (authfeature.RotatedCredentials, error) { return newC, step("entropy") }, EventID: func() (string, error) { return fixtures.AccessTokenID, step("event-id") }, Issue: func(req jwt.AccessIssue) (jwt.IssuedAccess, error) {
					if req.UserID != 42 || req.SessionID != fixtures.SessionID || req.AuthVersion != 3 || req.TokenID != newC.TokenID || !req.SessionExpiresAt.Equal(state.ExpiresAt) {
						t.Fatal("issue claims")
					}
					if err := step("sign"); err != nil {
						return jwt.IssuedAccess{}, err
					}
					if kind == "empty issued" {
						return jwt.IssuedAccess{}, nil
					}
					return issuer.Issue(req)
				}}
				deps.Bind = func(handle domain.Transaction) authfeature.RefreshRepositories {
					if handle != tx {
						t.Fatal("wrong tx")
					}
					tx.Record("bind")
					return authfeature.RefreshRepositories{Tokens: readToken("locked-token"), State: readState("locked-state"), Rotation: mocks.RefreshRotation{Insert: func(r domain.InitialRefresh) error {
						if r.Validate() != nil || r.ID != newC.RefreshID || r.SessionID != old.SessionID || r.FamilyID != old.FamilyID || !r.ExpiresAt.Equal(old.ExpiresAt) || !bytes.Equal(r.TokenHash, newC.RefreshDigest) {
							t.Fatal("successor mutation")
						}
						return step("insert")
					}, ConsumeToken: func(got context.Context, r domain.RefreshConsumption) (bool, error) {
						if got != ctx || r.ID != old.ID || r.SuccessorID != newC.RefreshID || !r.At.Equal(fixtures.ServiceTime) {
							t.Fatal("consume args")
						}
						return kind != "zero consume", step("consume")
					}},
						Sessions: mocks.SessionRevocation{Single: func(got context.Context, r domain.SessionRevocation) (bool, error) {
							if got != ctx || r.Validate() != nil || r.UserID != 42 || r.ActorID != 42 || r.SessionID != fixtures.SessionID || r.Reason != types.AuditRefreshReuse {
								t.Fatal("revoke session args")
							}
							return kind != "terminal", step("revoke-session")
						}},
						Family: mocks.RefreshRevocation{Family: func(got context.Context, r domain.RefreshFamilyRevocation) (int64, error) {
							if got != ctx || r.Validate() != nil || r.SessionID != fixtures.SessionID || r.FamilyID != old.FamilyID {
								t.Fatal("family scope")
							}
							return 2, step("revoke-family")
						}},
						Audit: mocks.AuditWriter{Write: func(r domain.AuditRequest) error {
							record, err := r.Record()
							if err != nil || record.ActorUserID == nil || *record.ActorUserID != 42 || record.TargetID != fixtures.SessionID {
								t.Fatal("safe audit", err)
							}
							names := map[types.AuditAction]string{types.AuditSessionRefreshRotated: "rotation-audit", types.AuditRefreshReuseDetected: "reuse-audit", types.AuditSessionRevoked: "terminal-audit"}
							name, ok := names[r.Action]
							if !ok {
								t.Fatal("unexpected audit")
							}
							return step(name)
						}},
						Outbox: shared.OutboxWriter{Write: func(e domain.EventEnvelope) error {
							record, err := e.Record()
							if err != nil || e.AggregateID != fixtures.SessionID || e.AggregateVersion != 2 || e.EventType != types.EventSessionRevoked || record.PublishedAt != nil || strings.Contains(record.Payload, oldC.RefreshToken) || strings.Contains(record.Payload, "token_hash") {
								t.Fatal("unsafe event", err)
							}
							return step("outbox")
						}}}
				}
				svc, err := authfeature.NewRefreshService(deps)
				if err != nil {
					t.Fatal(err)
				}
				raw := oldC.RefreshToken
				if kind == "empty credential" {
					raw = ""
				}
				if kind == "malformed" {
					raw = strings.Repeat("!", 43)
				}
				if kind == "canceled entry" {
					cancel()
				}
				result, err := svc.Refresh(ctx, raw)
				want := append([]string{}, path...)
				if kind == "terminal" && mode == "reuse" {
					want = append(append([]string{}, reuse[:12]...), "commit")
				}
				stops := map[string]int{"empty credential": 0, "malformed": 0, "unverified": 0, "canceled entry": 0, "unknown": 1, "expired": 2, "deleted": 2, "owner": 2, "changed owner": 9, "changed family": 9, "busy": 5, "error:lock": 5, "error:begin": 4}
				if mode == "rotation" {
					stops["restricted"] = 2
					stops["revoked"] = 2
					stops["terminal"] = 2
					stops["zero consume"] = 12
					stops["empty issued"] = 14
				}
				for _, invalid := range []string{"PENDING", "SUSPENDED", "DEACTIVATED", "expired session", "long session", "future session", "expired token", "extended token", "zero version", "zero auth version", "missing session", "unknown generation", "changed generation"} {
					stops[invalid] = 2
				}
				stops["wrong digest"] = 1
				stops["busy session"] = 6
				n, stopped := stops[kind]
				if !stopped {
					for i, name := range path {
						if kind == "error:"+name || kind == "panic:"+name || kind == "cancel:"+name {
							n = i + 1
							stopped = true
							break
						}
					}
				}
				if stopped {
					want = append([]string{}, path[:n]...)
					if n > 4 {
						want = append(want, "rollback")
					}
				}
				// A driver Commit that returns success owns the outcome even if cancellation follows.
				if kind == "cancel:commit" {
					want = append([]string{}, path...)
				}
				if !reflect.DeepEqual(tx.Calls, want) && !(len(tx.Calls) == 0 && len(want) == 0) {
					t.Fatalf("calls %v want %v", tx.Calls, want)
				}
				if stopped && kind != "cancel:commit" {
					if result != nil || err == nil {
						t.Fatal("failure exposed credentials")
					}
					if strings.HasPrefix(kind, "error:") && kind != "error:lock" && !errors.Is(err, cause) {
						t.Fatal("cause lost", err)
					}
					if strings.HasPrefix(kind, "panic:") && !errors.Is(err, common.ErrInternalServerError) {
						t.Fatal("panic lost", err)
					}
					if strings.HasPrefix(kind, "cancel:") || kind == "canceled entry" {
						if !errors.Is(err, context.Canceled) {
							t.Fatal("cancellation lost", err)
						}
					}
					if (kind == "busy" || kind == "busy session") && !errors.Is(err, auth.ErrResourceBusy) {
						t.Fatal("busy lost")
					}
					return
				}
				if mode == "reuse" {
					if result != nil || !errors.Is(err, auth.ErrRefreshTokenReused) {
						t.Fatal("reuse commit denial", err)
					}
					return
				}
				if err != nil || result == nil || result.RefreshToken != newC.RefreshToken || result.SessionID != fixtures.SessionID || !result.SessionExpiresAt.Equal(state.ExpiresAt) {
					t.Fatal("rotation result", err)
				}
				verify, e := jwt.NewAccessVerifier(fixtures.AccessIssuer, fixtures.AccessKey().Public().(ed25519.PublicKey), func() time.Time { return fixtures.ServiceTime })
				if e != nil {
					t.Fatal(e)
				}
				claims, e := verify.Verify(result.AccessToken)
				if e != nil || claims.AuthVersion() != 3 || claims.UserID() != 42 || result.AccessExpiresAt.After(state.ExpiresAt) || result.AccessExpiresAt.Sub(fixtures.ServiceTime) > 15*time.Minute {
					t.Fatal("issued bounds", e)
				}
			})
		}
	}
	if _, err := authfeature.NewRefreshService(authfeature.RefreshDependencies{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
}
