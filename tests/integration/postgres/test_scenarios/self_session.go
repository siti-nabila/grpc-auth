//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc"
)

func SelfSessionLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "self single then global revocation preserves scopes and rejects old credentials", Run: postgresSelfSessions}, {Name: "self required writes and business guard rollback before independent audit", Run: postgresSelfFailures}, {Name: "two-backend Login versus global revoke barriers enforce committed generation", Run: postgresSelfContention}, {Name: "Login holder blocks global revocation then its committed session joins invalidation", Run: postgresLoginBeforeGlobal}}
}

func selfDependencies(f *fixtures.Schema) session.SelfDependencies {
	return session.SelfDependencies{Transactions: productionProvider(f), Bind: session.BindSelfRepositories, Now: func() time.Time { return shared.ServiceTime }, EventID: eventIDs()}
}
func selfService(t *testing.T, d session.SelfDependencies) session.SelfService {
	t.Helper()
	s, err := session.NewSelfService(d)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func selfActor(t *testing.T, ctx context.Context, f *fixtures.Schema, token, method string) context.Context {
	t.Helper()
	ctx = shared.CallerContext(t, ctx, method, method)
	claims, err := loginSecurity(t, func() time.Time { return shared.ServiceTime }).Verifier.Verify(token)
	if err != nil {
		t.Fatal("access verification", err)
	}
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	ctx, err = authfeature.AuthenticateSession(ctx, claims, reader.NewSessionStateReader(orm.NewSqlQueryAdapter(ctx, f.DB, dialect.NewPostgres(), cfg)), shared.ServiceTime)
	if err != nil {
		t.Fatal("session authentication", err)
	}
	return ctx
}

func postgresSelfSessions(t *testing.T) {
	for _, mode := range []string{"other device", "current device"} {
		t.Run(mode, func(t *testing.T) {
			ctx, f := loginFixture(t, false)
			initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			other, err := loginService(t, loginDependencies(t, ctx, f, 80)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			actor := selfActor(t, ctx, f, initial.AccessToken, sessionv1.SessionService_RevokeMySession_FullMethodName)
			svc := selfService(t, selfDependencies(f))
			target := other.SessionID
			if mode == "current device" {
				target = initial.SessionID
			}
			r, err := svc.RevokeMySession(actor, target)
			if err != nil || r == nil || !r.Changed {
				t.Fatal("single revoke", err)
			}
			if mode == "other device" {
				r, err = svc.RevokeMySession(actor, target)
				if err != nil || r == nil || r.Changed {
					t.Fatal("terminal target no-op", err)
				}
			} else {
				if r, err = svc.RevokeMySession(actor, target); r != nil || !errors.Is(err, auth.ErrInvalidSession) {
					t.Fatal("revoked self principal reused", err)
				}
			}
			var active, revoked, refresh, audits, events, v, av int
			err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE status='ACTIVE'),(SELECT count(*) FROM sessions WHERE status='REVOKED'),(SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NOT NULL),(SELECT count(*) FROM audit_logs WHERE action='SESSION_REVOKED'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.session.revoked.v1' AND aggregate_version=2),version,auth_version FROM auth WHERE id=1`).Scan(&active, &revoked, &refresh, &audits, &events, &v, &av)
			if err != nil || active != 1 || revoked != 1 || refresh != 1 || audits != 1 || events != 1 || v != 1 || av != 1 {
				t.Fatal("single scope/counts", active, revoked, refresh, audits, events, v, av, err)
			}
			remaining := initial
			if mode == "current device" {
				remaining = other
			}
			allActor := selfActor(t, ctx, f, remaining.AccessToken, sessionv1.SessionService_RevokeAllMySessions_FullMethodName)
			all, err := svc.RevokeAllMySessions(allActor)
			if err != nil || all == nil || !all.Changed || !all.LoginRequired {
				t.Fatal("global revoke", err)
			}
			if r, e := svc.RevokeAllMySessions(allActor); r != nil || !errors.Is(e, auth.ErrInvalidSession) {
				t.Fatal("old self global retry accepted", e)
			}
			err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE status='ACTIVE'),(SELECT count(*) FROM sessions WHERE status='REVOKED'),(SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NOT NULL),(SELECT count(*) FROM audit_logs WHERE action='USER_SESSIONS_REVOKED'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.user.sessions_revoked.v1' AND aggregate_version=2 AND payload->'data'->>'reason_code'='SELF_SESSIONS_REVOKED' AND payload->'data'->>'sessions_revoked'='true'),version,auth_version FROM auth WHERE id=1`).Scan(&active, &revoked, &refresh, &audits, &events, &v, &av)
			if err != nil || active != 0 || revoked != 2 || refresh != 2 || audits != 1 || events != 1 || v != 2 || av != 2 {
				t.Fatal("global scope/counts", active, revoked, refresh, audits, events, v, av, err)
			}
			refreshCtx := refreshCaller(t, ctx)
			for _, token := range []string{initial.RefreshToken, other.RefreshToken} {
				if r, e := refreshService(t, refreshDependencies(t, refreshCtx, f, 160)).Refresh(refreshCtx, token); r != nil || !errors.Is(e, auth.ErrInvalidSession) {
					t.Fatal("old refresh survived global", e)
				}
			}
			fresh, e := loginService(t, loginDependencies(t, ctx, f, 230)).Login(ctx, loginRequest())
			if e != nil {
				t.Fatal("fresh Login", e)
			}
			var generation int
			if e = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, fresh.SessionID).Scan(&generation); e != nil || generation != 2 {
				t.Fatal("fresh generation", e)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("pool leak")
			}
		})
	}
}

func postgresSelfFailures(t *testing.T) {
	for _, mode := range []string{"single", "global"} {
		for _, stage := range []string{"counters", "sessions", "zero sessions", "refresh", "audit", "outbox", "commit", "event id", "cancel", "panic"} {
			if mode == "single" && stage == "counters" {
				continue
			}
			t.Run(mode+"/"+stage, func(t *testing.T) {
				ctx, f := loginFixture(t, false)
				initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
				if err != nil {
					t.Fatal(err)
				}
				method := sessionv1.SessionService_RevokeMySession_FullMethodName
				if mode == "global" {
					method = sessionv1.SessionService_RevokeAllMySessions_FullMethodName
				}
				actor := selfActor(t, ctx, f, initial.AccessToken, method)
				actor, cancel := context.WithCancel(actor)
				defer cancel()
				d := selfDependencies(f)
				switch stage {
				case "event id":
					d.EventID = func() (string, error) { return "", errors.New("controlled entropy") }
				case "cancel":
					d.EventID = func() (string, error) { cancel(); return "", actor.Err() }
				case "panic":
					d.EventID = func() (string, error) { panic("controlled private panic") }
				default:
					table, op, condition := "sessions", "UPDATE", "TRUE"
					if stage == "counters" {
						table = "auth"
					}
					if stage == "refresh" {
						table = "refresh_tokens"
					}
					if stage == "audit" {
						table = "audit_logs"
						op = "INSERT"
						condition = "NEW.action<>'SECURITY_ATTEMPT_FAILED'"
					}
					if stage == "outbox" || stage == "commit" {
						table = "outbox_events"
						op = "INSERT"
					}
					body := `IF ` + condition + ` THEN RAISE EXCEPTION 'controlled failure' USING ERRCODE='23514'; END IF; RETURN NEW;`
					trigger := "CREATE TRIGGER reject_self AFTER " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION reject_self()"
					if stage == "zero sessions" {
						body = "RETURN NULL;"
						trigger = "CREATE TRIGGER reject_self BEFORE UPDATE ON sessions FOR EACH ROW EXECUTE FUNCTION reject_self()"
					}
					if stage == "commit" {
						trigger = "CREATE CONSTRAINT TRIGGER reject_self AFTER INSERT ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_self()"
					}
					if _, err = f.DB.ExecContext(ctx, `CREATE FUNCTION reject_self() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+body+` END $$; `+trigger); err != nil {
						t.Fatal("fault setup", err)
					}
				}
				observed := 0
				attempt, err := interceptors.NewSecurityAttemptInterceptor(func(a context.Context, r domain.AuditRequest) error {
					assertRefreshUnchanged(t, a, f)
					return featurecommon.RecordSecurityAttempt(a, productionProvider(f), r)
				}, func() { observed++ }, func() time.Time { return shared.ServiceTime })
				if err != nil {
					t.Fatal(err)
				}
				svc := selfService(t, d)
				_, err = attempt(actor, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(c context.Context, _ any) (any, error) {
					if mode == "single" {
						return svc.RevokeMySession(c, initial.SessionID)
					}
					return svc.RevokeAllMySessions(c)
				})
				want := error(common.ErrDependencyUnavailable)
				if stage == "zero sessions" {
					want = auth.ErrInvalidSession
				}
				if stage == "cancel" {
					want = context.Canceled
				}
				if stage == "panic" {
					want = common.ErrInternalServerError
				}
				if !errors.Is(err, want) || observed != 0 {
					t.Fatal("failure/audit outcome", err, observed)
				}
				var audits, events, v, av int
				if err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND actor_service='rest-orc'),(SELECT count(*) FROM outbox_events),version,auth_version FROM auth WHERE id=1`).Scan(&audits, &events, &v, &av); err != nil || audits != 1 || events != 0 || v != 1 || av != 1 {
					t.Fatal("rollback before failure audit", audits, events, v, av, err)
				}
				if f.DB.Stats().InUse != 0 {
					t.Fatal("failure pool leak")
				}
			})
		}
	}
}

type selfAllOutcome struct {
	result *session.RevokeAllResult
	err    error
}

func postgresSelfContention(t *testing.T) {
	for _, outcome := range []string{"commit", "failure", "panic", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, f := loginFixture(t, false)
			initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			actor := selfActor(t, ctx, f, initial.AccessToken, sessionv1.SessionService_RevokeAllMySessions_FullMethodName)
			request, cancel := context.WithCancel(actor)
			defer cancel()
			d := selfDependencies(f)
			first := &observedProvider{inner: d.Transactions}
			d.Transactions = first
			ready, release := make(chan struct{}), make(chan struct{})
			done := make(chan selfAllOutcome, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			finished := false
			defer func() {
				unblock()
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("worker cleanup")
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
			svc := selfService(t, d)
			go func() { r, e := svc.RevokeAllMySessions(request); done <- selfAllOutcome{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("failed before barrier", r.err)
			case <-ctx.Done():
				t.Fatal("barrier timeout")
			}
			assertRefreshUnchanged(t, ctx, f)
			login := loginDependencies(t, ctx, f, 80)
			second := &observedProvider{inner: login.Transactions}
			login.Transactions = second
			if r, e := loginService(t, login).Login(ctx, loginRequest()); r != nil || !errors.Is(e, auth.ErrResourceBusy) || first.pid == 0 || second.pid == 0 || first.pid == second.pid {
				t.Fatal("two-backend Login contention", e)
			}
			t.Logf("global holder backend=%d Login contender backend=%d", first.pid, second.pid)
			if outcome == "cancel" {
				cancel()
			} else {
				unblock()
			}
			var winner selfAllOutcome
			select {
			case winner = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("worker timeout")
			}
			if outcome == "commit" {
				if winner.err != nil || winner.result == nil || !winner.result.LoginRequired {
					t.Fatal("winner", winner.err)
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
					t.Fatal("rollback outcome", winner.err)
				}
				assertRefreshUnchanged(t, ctx, f)
			}
			fresh, e := loginService(t, loginDependencies(t, ctx, f, 160)).Login(ctx, loginRequest())
			if e != nil {
				t.Fatal("post-global Login", e)
			}
			var generation int
			if e = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, fresh.SessionID).Scan(&generation); e != nil || generation != map[bool]int{true: 2, false: 1}[outcome == "commit"] {
				t.Fatal("post-global generation", generation, e)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("contention pool leak")
			}
		})
	}
}

func postgresLoginBeforeGlobal(t *testing.T) {
	ctx, f := loginFixture(t, false)
	initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	actor := selfActor(t, ctx, f, initial.AccessToken, sessionv1.SessionService_RevokeAllMySessions_FullMethodName)
	d := loginDependencies(t, ctx, f, 80)
	first := &observedProvider{inner: d.Transactions}
	d.Transactions = first
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan loginResult, 1)
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	finished := false
	defer func() {
		unblock()
		if !finished {
			waitLogin(t, ctx, done)
		}
	}()
	issue := d.Issue
	d.Issue = func(r jwt.AccessIssue) (jwt.IssuedAccess, error) {
		close(ready)
		select {
		case <-release:
		case <-ctx.Done():
			return jwt.IssuedAccess{}, ctx.Err()
		}
		return issue(r)
	}
	svc := loginService(t, d)
	go func() { r, e := svc.Login(ctx, loginRequest()); done <- loginResult{r, e} }()
	select {
	case <-ready:
	case r := <-done:
		finished = true
		t.Fatal("Login failed before barrier", r.err)
	case <-ctx.Done():
		t.Fatal("barrier timeout")
	}
	all := selfDependencies(f)
	second := &observedProvider{inner: all.Transactions}
	all.Transactions = second
	if r, e := selfService(t, all).RevokeAllMySessions(actor); r != nil || !errors.Is(e, auth.ErrResourceBusy) || first.pid == second.pid || first.pid == 0 || second.pid == 0 {
		t.Fatal("global contender missing busy/two backends", e)
	}
	t.Logf("Login holder backend=%d global contender backend=%d", first.pid, second.pid)
	unblock()
	winner := waitLogin(t, ctx, done)
	finished = true
	if winner.err != nil || winner.response == nil {
		t.Fatal("Login winner", winner.err)
	}
	if r, e := selfService(t, selfDependencies(f)).RevokeAllMySessions(actor); e != nil || r == nil || !r.Changed {
		t.Fatal("post-Login global invalidation", e)
	}
	var revoked, refresh, v, av int
	if err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE status='REVOKED'),(SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NOT NULL),version,auth_version FROM auth WHERE id=1`).Scan(&revoked, &refresh, &v, &av); err != nil || revoked != 2 || refresh != 2 || v != 2 || av != 2 {
		t.Fatal("Login winner escaped global invalidation", revoked, refresh, v, av, err)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("Login/global pool leak")
	}
}
