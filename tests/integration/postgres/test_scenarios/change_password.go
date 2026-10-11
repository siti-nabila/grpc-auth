//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc"
)

func PasswordLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "restricted password change invalidates every device and requires fresh password Login", Run: postgresPassword}, {Name: "password required writes business guard and deferred commit rollback before failure audit", Run: postgresPasswordFailures}, {Name: "two-backend password holder serializes Login refresh and global invalidation", Run: postgresPasswordContention}}
}

func passwordDependencies(ctx context.Context, f *fixtures.Schema) authfeature.PasswordDependencies {
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	return authfeature.PasswordDependencies{Transactions: productionProvider(f), Candidate: reader.NewLoginAccountReader(orm.NewSqlQueryAdapter(ctx, f.DB, dialect.NewPostgres(), cfg)), Bind: authfeature.BindPasswordRepositories, VerifyPassword: authfeature.VerifyLoginPassword, HashPassword: func(raw string) (string, error) { return shared.LoginPasswordHashFor(raw), nil }, Now: func() time.Time { return shared.ServiceTime }, EventID: eventIDs()}
}
func passwordService(t *testing.T, d authfeature.PasswordDependencies) authfeature.ChangePasswordService {
	t.Helper()
	s, e := authfeature.NewChangePasswordService(d)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func passwordRequest() domain.ChangePasswordInput {
	return domain.ChangePasswordInput{CurrentPassword: "secret12", NewPassword: "new-secret12"}
}

func postgresPassword(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "restricted"}[restricted], func(t *testing.T) {
			ctx, f := loginFixture(t, restricted)
			initial, e := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if e != nil {
				t.Fatal(e)
			}
			other, e := loginService(t, loginDependencies(t, ctx, f, 80)).Login(ctx, loginRequest())
			if e != nil {
				t.Fatal(e)
			}
			actor := selfActor(t, ctx, f, initial.AccessToken, authv1.AuthService_ChangePassword_FullMethodName)
			svc := passwordService(t, passwordDependencies(actor, f))
			wrong := passwordRequest()
			wrong.CurrentPassword = "incorrect"
			if r, e := svc.ChangePassword(actor, wrong); r != nil || !errors.Is(e, auth.ErrPasswordMismatch) {
				t.Fatal("wrong current password", e)
			}
			r, e := svc.ChangePassword(actor, passwordRequest())
			if e != nil || r == nil || !r.Changed || !r.LoginRequired {
				t.Fatal("password change", e)
			}
			if r, e = svc.ChangePassword(actor, passwordRequest()); r != nil || !errors.Is(e, auth.ErrInvalidSession) {
				t.Fatal("old password self retry", e)
			}
			var v, av, revoked, tokens, passwordAudits, globalAudits, events int
			var safe bool
			e = f.DB.QueryRowContext(ctx, `SELECT version,auth_version,(SELECT count(*) FROM sessions WHERE status='REVOKED'),(SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NOT NULL),(SELECT count(*) FROM audit_logs WHERE action='PASSWORD_CHANGED'),(SELECT count(*) FROM audit_logs WHERE action='USER_SESSIONS_REVOKED'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.user.sessions_revoked.v1' AND aggregate_version=2 AND payload->'data'->>'reason_code'='PASSWORD_CHANGED' AND payload->'data'->>'sessions_revoked'='true' AND published_at IS NULL),NOT must_change_password AND password_changed_at=$1 AND password=$2 AND status='ACTIVE' FROM auth WHERE id=1`, shared.ServiceTime, shared.LoginPasswordHashFor("new-secret12")).Scan(&v, &av, &revoked, &tokens, &passwordAudits, &globalAudits, &events, &safe)
			if e != nil || v != 2 || av != 2 || revoked != 2 || tokens != 2 || passwordAudits != 1 || globalAudits != 1 || events != 1 || !safe {
				t.Fatal("password atomic counts", v, av, revoked, tokens, passwordAudits, globalAudits, events, safe, e)
			}
			refreshCtx := refreshCaller(t, ctx)
			for _, token := range []string{initial.RefreshToken, other.RefreshToken} {
				if r, e := refreshService(t, refreshDependencies(t, refreshCtx, f, 160)).Refresh(refreshCtx, token); r != nil || !errors.Is(e, auth.ErrInvalidSession) {
					t.Fatal("old refresh survived password", e)
				}
			}
			security := loginSecurity(t, func() time.Time { return shared.ServiceTime })
			cfg := database.GetORMConfig()
			cfg.EnableDebug = false
			for _, token := range []string{initial.AccessToken, other.AccessToken} {
				claims, e := security.Verifier.Verify(token)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = authfeature.AuthenticateSession(actor, claims, reader.NewSessionStateReader(orm.NewSqlQueryAdapter(actor, f.DB, dialect.NewPostgres(), cfg)), shared.ServiceTime); !errors.Is(e, auth.ErrInvalidSession) {
					t.Fatal("old access survived password", e)
				}
			}
			if r, e := loginService(t, loginDependencies(t, ctx, f, 160)).Login(ctx, loginRequest()); r != nil || !errors.Is(e, auth.ErrLoginFailed) {
				t.Fatal("old password accepted", e)
			}
			freshReq := loginRequest()
			freshReq.Password = "new-secret12"
			fresh, e := loginService(t, loginDependencies(t, ctx, f, 230)).Login(ctx, freshReq)
			if e != nil || fresh == nil || fresh.MustChangePassword {
				t.Fatal("new password Login", e)
			}
			var generation int
			if e = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, fresh.SessionID).Scan(&generation); e != nil || generation != 2 {
				t.Fatal("fresh password generation", e)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("password pool leak")
			}
		})
	}
}

func postgresPasswordFailures(t *testing.T) {
	for _, stage := range []string{"password", "zero password", "sessions", "zero sessions", "refresh", "password audit", "global audit", "outbox", "commit", "event id", "cancel", "panic"} {
		t.Run(stage, func(t *testing.T) {
			ctx, f := loginFixture(t, true)
			initial, e := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if e != nil {
				t.Fatal(e)
			}
			actor := selfActor(t, ctx, f, initial.AccessToken, authv1.AuthService_ChangePassword_FullMethodName)
			actor, cancel := context.WithCancel(actor)
			defer cancel()
			d := passwordDependencies(actor, f)
			switch stage {
			case "event id":
				d.EventID = func() (string, error) { return "", errors.New("controlled entropy") }
			case "cancel":
				d.EventID = func() (string, error) { cancel(); return "", actor.Err() }
			case "panic":
				d.EventID = func() (string, error) { panic("private panic") }
			default:
				table, op, condition := "auth", "UPDATE", "TRUE"
				if stage == "sessions" || stage == "zero sessions" {
					table = "sessions"
				}
				if stage == "refresh" {
					table = "refresh_tokens"
				}
				if stage == "password audit" || stage == "global audit" {
					table = "audit_logs"
					op = "INSERT"
					action := "PASSWORD_CHANGED"
					if stage == "global audit" {
						action = "USER_SESSIONS_REVOKED"
					}
					condition = "NEW.action='" + action + "'"
				}
				if stage == "outbox" || stage == "commit" {
					table = "outbox_events"
					op = "INSERT"
				}
				body := `IF ` + condition + ` THEN RAISE EXCEPTION 'controlled failure' USING ERRCODE='23514'; END IF; RETURN NEW;`
				trigger := "CREATE TRIGGER reject_password AFTER " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION reject_password()"
				if stage == "zero password" || stage == "zero sessions" {
					body = "RETURN NULL;"
					trigger = "CREATE TRIGGER reject_password BEFORE UPDATE ON " + table + " FOR EACH ROW EXECUTE FUNCTION reject_password()"
				}
				if stage == "commit" {
					trigger = "CREATE CONSTRAINT TRIGGER reject_password AFTER INSERT ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_password()"
				}
				if _, e = f.DB.ExecContext(ctx, `CREATE FUNCTION reject_password() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+body+` END $$; `+trigger); e != nil {
					t.Fatal("password fault setup", e)
				}
			}
			observed := 0
			attempt, e := interceptors.NewSecurityAttemptInterceptor(func(c context.Context, r domain.AuditRequest) error {
				assertRefreshUnchanged(t, c, f)
				var original bool
				if e := f.DB.QueryRowContext(c, `SELECT password=$1 AND must_change_password AND password_changed_at IS NULL FROM auth WHERE id=1`, shared.LoginPasswordHash()).Scan(&original); e != nil || !original {
					t.Fatal("partial password effects before failure audit", e)
				}
				return featurecommon.RecordSecurityAttempt(c, productionProvider(f), r)
			}, func() { observed++ }, func() time.Time { return shared.ServiceTime })
			if e != nil {
				t.Fatal(e)
			}
			svc := passwordService(t, d)
			_, e = attempt(actor, nil, &grpc.UnaryServerInfo{FullMethod: authv1.AuthService_ChangePassword_FullMethodName}, func(c context.Context, _ any) (any, error) { return svc.ChangePassword(c, passwordRequest()) })
			want := error(common.ErrDependencyUnavailable)
			if stage == "zero password" {
				want = user.ErrVersionConflict
			}
			if stage == "zero sessions" {
				want = auth.ErrInvalidSession
			}
			if stage == "cancel" {
				want = context.Canceled
			}
			if stage == "panic" {
				want = common.ErrInternalServerError
			}
			if !errors.Is(e, want) {
				t.Fatal("password fault outcome", e)
			}
			if observed != 0 {
				t.Fatal("audit failed")
			}
			var attempts, effects int
			if e = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND actor_service='rest-orc'),(SELECT count(*) FROM outbox_events)+(SELECT count(*) FROM audit_logs WHERE action IN ('PASSWORD_CHANGED','USER_SESSIONS_REVOKED'))`).Scan(&attempts, &effects); e != nil || attempts != 1 || effects != 0 {
				t.Fatal("failure atomic evidence", attempts, effects, e)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("password rollback pool leak")
			}
		})
	}
}

type passwordOutcome struct {
	result *authfeature.ChangePasswordResult
	err    error
}

func postgresPasswordContention(t *testing.T) {
	for _, competitor := range []string{"Login", "refresh", "global"} {
		for _, outcome := range []string{"commit", "failure", "panic", "cancel"} {
			t.Run(competitor+"/"+outcome, func(t *testing.T) {
				ctx, f := loginFixture(t, false)
				initial, e := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
				if e != nil {
					t.Fatal(e)
				}
				actor := selfActor(t, ctx, f, initial.AccessToken, authv1.AuthService_ChangePassword_FullMethodName)
				request, cancel := context.WithCancel(actor)
				defer cancel()
				d := passwordDependencies(request, f)
				first := &observedProvider{inner: d.Transactions}
				d.Transactions = first
				ready, release := make(chan struct{}), make(chan struct{})
				done := make(chan passwordOutcome, 1)
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				finished := false
				defer func() {
					unblock()
					if !finished {
						select {
						case <-done:
						case <-ctx.Done():
							t.Error("password worker cleanup")
						}
					}
				}()
				eventID := d.EventID
				d.EventID = func() (string, error) {
					close(ready)
					select {
					case <-release:
					case <-request.Done():
						return "", request.Err()
					}
					if outcome == "failure" {
						return "", errors.New("controlled event failure")
					}
					if outcome == "panic" {
						panic("private panic")
					}
					return eventID()
				}
				svc := passwordService(t, d)
				go func() { r, e := svc.ChangePassword(request, passwordRequest()); done <- passwordOutcome{r, e} }()
				select {
				case <-ready:
				case r := <-done:
					finished = true
					t.Fatal("password failed before barrier", r.err)
				case <-ctx.Done():
					t.Fatal("password barrier timeout")
				}
				assertRefreshUnchanged(t, ctx, f)
				second := &observedProvider{inner: productionProvider(f)}
				switch competitor {
				case "Login":
					c := loginDependencies(t, ctx, f, 80)
					c.Transactions = second
					r, err := loginService(t, c).Login(ctx, loginRequest())
					if r != nil || !errors.Is(err, auth.ErrResourceBusy) {
						t.Fatal("Login contention", err)
					}
				case "refresh":
					cctx := refreshCaller(t, ctx)
					c := refreshDependencies(t, cctx, f, 80)
					c.Transactions = second
					r, err := refreshService(t, c).Refresh(cctx, initial.RefreshToken)
					if r != nil || !errors.Is(err, auth.ErrResourceBusy) {
						t.Fatal("refresh contention", err)
					}
				case "global":
					cctx := selfActor(t, ctx, f, initial.AccessToken, sessionv1.SessionService_RevokeAllMySessions_FullMethodName)
					c := selfDependencies(f)
					c.Transactions = second
					r, err := selfService(t, c).RevokeAllMySessions(cctx)
					if r != nil || !errors.Is(err, auth.ErrResourceBusy) {
						t.Fatal("global contention", err)
					}
				}
				if first.pid == 0 || second.pid == 0 || first.pid == second.pid {
					t.Fatal("password contention missing distinct backends")
				}
				t.Logf("password holder backend=%d %s contender backend=%d", first.pid, competitor, second.pid)
				if outcome == "cancel" {
					cancel()
				} else {
					unblock()
				}
				var winner passwordOutcome
				select {
				case winner = <-done:
					finished = true
				case <-ctx.Done():
					t.Fatal("password worker timeout")
				}
				if outcome == "commit" {
					if winner.err != nil || winner.result == nil || !winner.result.LoginRequired {
						t.Fatal("password winner", winner.err)
					}
				} else {
					want := error(common.ErrDependencyUnavailable)
					if outcome == "panic" {
						want = common.ErrInternalServerError
					}
					if outcome == "cancel" {
						want = context.Canceled
					}
					if winner.result != nil || !errors.Is(winner.err, want) {
						t.Fatal("password rollback outcome", winner.err)
					}
					assertRefreshUnchanged(t, ctx, f)
				}
				req := loginRequest()
				expected := 1
				if outcome == "commit" {
					req.Password = "new-secret12"
					expected = 2
				}
				fresh, e := loginService(t, loginDependencies(t, ctx, f, 160)).Login(ctx, req)
				if e != nil || fresh == nil {
					t.Fatal("post-password Login", e)
				}
				var generation int
				if e = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, fresh.SessionID).Scan(&generation); e != nil || generation != expected {
					t.Fatal("post-password generation", generation, e)
				}
				if f.DB.Stats().InUse != 0 {
					t.Fatal("password contention pool leak")
				}
			})
		}
	}
}
