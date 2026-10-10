package test_scenarios

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func SessionLogin() []testutils.Scenario {
	return []testutils.Scenario{{Name: "persistent Login orders proof lock recheck issuance and rollback-before-audit", Run: loginSessionMatrix}, {Name: "repeated Login produces distinct sessions without account mutation", Run: loginSessionRepeated}}
}

func loginSessionMatrix(t *testing.T) {
	for _, kind := range []string{"success", "restricted", "invalid", "unverified", "cancelled", "candidate", "unknown", "PENDING", "SUSPENDED", "DEACTIVATED", "deleted", "verify", "mismatch", "entropy", "begin", "busy", "lock", "locked", "disappeared", "changed hash", "changed status", "changed version", "changed auth version", "changed restricted", "session", "refresh", "audit", "sign", "empty token", "commit", "panic session", "cancel session", "audit failure", "audit panic"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(callerContext(t))
			defer cancel()
			if kind == "unverified" {
				ctx = context.Background()
			}
			tx := &shared.Transaction{Context: ctx}
			provider := &shared.TransactionProvider{Tx: tx}
			cause := errors.New("controlled private dependency detail")
			step := func(name string) error {
				tx.Record(name)
				if kind == name {
					return cause
				}
				return nil
			}
			a := domain.LoginAccount{ID: 42, Email: "member@example.com", Password: "test-only-hash", Status: "ACTIVE", AuthVersion: 3, Version: 4, MustChangePassword: kind == "restricted"}
			if kind == "PENDING" || kind == "SUSPENDED" || kind == "DEACTIVATED" {
				a.Status = types.UserStatus(kind)
			}
			if kind == "deleted" {
				a.DeletedAt = sql.NullTime{Time: fixtures.ServiceTime, Valid: true}
			}
			if kind == "begin" {
				provider.Error = cause
			}
			if kind == "busy" {
				tx.Busy = true
			}
			if kind == "lock" {
				tx.LockError = cause
			}
			if kind == "commit" {
				tx.CommitError = cause
			}
			data := make([]byte, 96)
			for n := range data {
				data[n] = byte(n)
			}
			c, err := authfeature.GenerateInitialCredentials(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			i, err := jwt.NewAccessIssuer(fixtures.AccessIssuer, fixtures.AccessKey())
			if err != nil {
				t.Fatal(err)
			}
			deps := authfeature.SessionLoginDependencies{Transactions: provider, Now: func() time.Time { return fixtures.ServiceTime },
				Candidate: mocks.LoginAccountReader{Email: func(got context.Context, email string) (domain.LoginAccount, error) {
					if got != ctx || email != a.Email {
						t.Fatal("candidate context/argument")
					}
					if err := step("candidate"); err != nil {
						return a, err
					}
					if kind == "unknown" {
						return a, auth.ErrNotFound
					}
					return a, nil
				}},
				VerifyPassword: func(hash, password string) (bool, error) {
					if password != "secret12" || hash != a.Password {
						t.Fatal("password arguments")
					}
					return kind != "mismatch" && kind != "audit failure" && kind != "audit panic", step("verify")
				},
				Credentials: func() (authfeature.InitialCredentials, error) { return c, step("entropy") },
				Issue: func(req jwt.AccessIssue) (jwt.IssuedAccess, error) {
					if req.UserID != 42 || req.AuthVersion != 3 || req.SessionID != c.SessionID || req.TokenID != c.TokenID {
						t.Fatal("claims")
					}
					if err := step("sign"); err != nil {
						return jwt.IssuedAccess{}, err
					}
					if kind == "empty token" {
						return jwt.IssuedAccess{}, nil
					}
					return i.Issue(req)
				},
				RecordFailure: func(got context.Context, r domain.AuditRequest) error {
					tx.Record("failure-audit")
					record, err := r.Record()
					if got != ctx || err != nil || record.ActorUserID != nil || record.ActorService == nil || *record.ActorService != "rest-orc" || record.TargetID != "/user.v1.UserService/Login" || record.Action != "LOGIN_FAILED" || record.SafeBefore != nil || record.SafeAfter != nil {
						t.Fatal("unsafe failure audit", err)
					}
					if kind == "audit failure" {
						return cause
					}
					if kind == "audit panic" {
						panic("private recorder detail")
					}
					return nil
				}, ObserveAuditFailure: func() { tx.Record("audit-metric") },
			}
			deps.Bind = func(handle domain.Transaction) authfeature.LoginRepositories {
				if handle != tx {
					t.Fatal("wrong handle")
				}
				tx.Record("bind")
				return authfeature.LoginRepositories{
					Account: mocks.LoginAccountReader{ID: func(got context.Context, id uint64) (domain.LoginAccount, error) {
						if got != ctx || id != 42 {
							t.Fatal("locked read arguments")
						}
						if err := step("locked"); err != nil {
							return a, err
						}
						b := a
						switch kind {
						case "disappeared":
							return b, auth.ErrNotFound
						case "changed hash":
							b.Password = "new-hash"
						case "changed status":
							b.Status = "SUSPENDED"
						case "changed version":
							b.Version++
						case "changed auth version":
							b.AuthVersion++
						case "changed restricted":
							b.MustChangePassword = !b.MustChangePassword
						}
						return b, nil
					}},
					Session: mocks.SessionWriter{Write: func(s domain.Session) error {
						if s.Validate() != nil || s.ID != c.SessionID || s.UserID != 42 || !s.ExpiresAt.Equal(fixtures.ServiceTime.Add(7*24*time.Hour)) {
							t.Fatal("session state")
						}
						if kind == "panic session" {
							tx.Record("session")
							panic("private session detail")
						}
						if kind == "cancel session" {
							tx.Record("session")
							cancel()
							return ctx.Err()
						}
						return step("session")
					}},
					Refresh: mocks.RefreshWriter{Write: func(r domain.InitialRefresh) error {
						if r.Validate() != nil || r.ID != c.RefreshID || r.FamilyID != c.FamilyID || r.SessionID != c.SessionID || !bytes.Equal(r.TokenHash, c.RefreshDigest) {
							t.Fatal("refresh state")
						}
						return step("refresh")
					}},
					Audit: mocks.AuditWriter{Write: func(r domain.AuditRequest) error {
						record, err := r.Record()
						if err != nil || record.ActorUserID == nil || *record.ActorUserID != 42 || record.Action != "SESSION_CREATED" || record.TargetID != c.SessionID || record.SafeAfter == nil || strings.Contains(*record.SafeAfter, "password") || strings.Contains(*record.SafeAfter, c.RefreshToken) {
							t.Fatal("unsafe success audit", err)
						}
						return step("audit")
					}},
				}
			}
			s, err := authfeature.NewSessionLoginService(deps)
			if err != nil {
				t.Fatal(err)
			}
			req := domain.AuthRequest{Email: " MEMBER@example.com ", Password: "secret12"}
			if kind == "invalid" {
				req.Email = "bad"
			}
			if kind == "cancelled" {
				cancel()
			}
			res, err := s.Login(ctx, req)
			full := []string{"candidate", "verify", "entropy", "begin", "lock:auth:user:42", "lock:auth:session:" + c.SessionID, "bind", "locked", "session", "refresh", "audit", "sign", "commit"}
			stops := map[string]int{"invalid": 0, "unverified": 0, "cancelled": 0, "candidate": 1, "unknown": 1, "PENDING": 1, "SUSPENDED": 1, "DEACTIVATED": 1, "deleted": 1, "verify": 2, "mismatch": 2, "audit failure": 2, "audit panic": 2, "entropy": 3, "begin": 4, "busy": 5, "lock": 5, "locked": 8, "disappeared": 8, "changed hash": 8, "changed status": 8, "changed version": 8, "changed auth version": 8, "changed restricted": 8, "session": 9, "panic session": 9, "cancel session": 9, "refresh": 10, "audit": 11, "sign": 12, "empty token": 12, "commit": 13}
			want := append([]string{}, full...)
			if n, ok := stops[kind]; ok {
				want = append([]string{}, full[:n]...)
				if n > 4 {
					want = append(want, "rollback")
				}
			}
			denied := kind == "unknown" || kind == "PENDING" || kind == "SUSPENDED" || kind == "DEACTIVATED" || kind == "deleted" || kind == "mismatch" || kind == "disappeared" || strings.HasPrefix(kind, "changed ") || kind == "audit failure" || kind == "audit panic"
			if denied {
				want = append(want, "failure-audit")
				if kind == "audit failure" || kind == "audit panic" {
					want = append(want, "audit-metric")
				}
			}
			if !reflect.DeepEqual(tx.Calls, want) && !(len(tx.Calls) == 0 && len(want) == 0) {
				t.Fatalf("calls %v want %v", tx.Calls, want)
			}
			if kind == "success" || kind == "restricted" {
				if err != nil || res == nil || res.AccessToken == "" || res.RefreshToken != c.RefreshToken || res.MustChangePassword != (kind == "restricted") || !res.AccessExpiresAt.Equal(fixtures.ServiceTime.Add(15*time.Minute)) {
					t.Fatal("success result", err)
				}
			} else {
				if err == nil || res != nil {
					t.Fatal("failure exposed credential")
				}
				if denied && !errors.Is(err, auth.ErrLoginFailed) {
					t.Fatal("generic denial lost", err)
				}
				if kind == "busy" && !errors.Is(err, auth.ErrResourceBusy) {
					t.Fatal("busy lost")
				}
				if kind == "cancel session" || kind == "cancelled" {
					if !errors.Is(err, context.Canceled) {
						t.Fatal("cancel lost")
					}
				}
			}
		})
	}
	if _, err := authfeature.NewSessionLoginService(authfeature.SessionLoginDependencies{}); err == nil {
		t.Fatal("missing dependencies accepted")
	}
}

func loginSessionRepeated(t *testing.T) {
	ctx := callerContext(t)
	tx := &shared.Transaction{Context: ctx}
	provider := &shared.TransactionProvider{Tx: tx}
	data := make([]byte, 192)
	for n := range data {
		data[n] = byte(n)
	}
	entropy := bytes.NewReader(data)
	a := domain.LoginAccount{ID: 42, Email: "member@example.com", Password: "test-only-hash", Status: "ACTIVE", AuthVersion: 3, Version: 4}
	r := mocks.LoginAccountReader{Email: func(context.Context, string) (domain.LoginAccount, error) { return a, nil }, ID: func(context.Context, uint64) (domain.LoginAccount, error) { return a, nil }}
	var records []domain.Session
	var refreshes []domain.InitialRefresh
	audits := 0
	i, err := jwt.NewAccessIssuer(fixtures.AccessIssuer, fixtures.AccessKey())
	if err != nil {
		t.Fatal(err)
	}
	s, err := authfeature.NewSessionLoginService(authfeature.SessionLoginDependencies{Transactions: provider, Candidate: r, Now: func() time.Time { return fixtures.ServiceTime }, Credentials: func() (authfeature.InitialCredentials, error) { return authfeature.GenerateInitialCredentials(entropy) }, VerifyPassword: func(string, string) (bool, error) { return true, nil }, Issue: i.Issue, RecordFailure: func(context.Context, domain.AuditRequest) error { t.Fatal("unexpected denial"); return nil }, ObserveAuditFailure: func() { t.Fatal("unexpected audit failure") }, Bind: func(domain.Transaction) authfeature.LoginRepositories {
		return authfeature.LoginRepositories{Account: r, Session: mocks.SessionWriter{Write: func(s domain.Session) error { records = append(records, s); return nil }}, Refresh: mocks.RefreshWriter{Write: func(r domain.InitialRefresh) error { refreshes = append(refreshes, r); return nil }}, Audit: mocks.AuditWriter{Write: func(domain.AuditRequest) error { audits++; return nil }}}
	}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Login(ctx, domain.AuthRequest{Email: a.Email, Password: "secret12"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Login(ctx, domain.AuthRequest{Email: a.Email, Password: "secret12"})
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID == second.SessionID || first.RefreshToken == second.RefreshToken || first.AccessToken == second.AccessToken || len(records) != 2 || len(refreshes) != 2 || refreshes[0].FamilyID == refreshes[1].FamilyID || audits != 2 || a.Version != 4 || a.AuthVersion != 3 {
		t.Fatal("repeated Login replayed or mutated account")
	}
}
