//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/session"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"sync"
	"testing"
	"time"
)

func AdminSessionLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "admin single safe replay no-op different target and expiry replacement", Run: postgresAdminSingle}, {Name: "admin global replay precedes expected version and new key invalidates empty target", Run: postgresAdminGlobal}, {Name: "admin self-target result cannot bypass revoked actor", Run: postgresAdminSelf}, {Name: "admin required writes and deferred commit faults rollback mutation audit event and result", Run: postgresAdminFailures}, {Name: "two-backend admin idem contention and current policy guard prevent stale replay", Run: postgresAdminContention}}
}

func adminFixture(t *testing.T, method string) (context.Context, context.Context, *fixtures.Schema, *authfeature.LoginResult, *authfeature.LoginResult) {
	t.Helper()
	ctx, f := loginFixture(t, false)
	if _, e := f.DB.ExecContext(ctx, `INSERT INTO auth(email,password,status) VALUES('target@example.test',$1,'ACTIVE')`, shared.LoginPasswordHash()); e != nil {
		t.Fatal("target account setup", e)
	}
	if _, e := f.DB.ExecContext(ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.session.revoke','fixture'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Session Admin',2,1,'iam.session_admin'); INSERT INTO role_permissions VALUES(2,1); INSERT INTO user_role(user_id,role_id) VALUES(1,2)`); e != nil {
		t.Fatal("admin grant setup", e)
	}

	initial, e := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
	if e != nil {
		t.Fatal(e)
	}
	req := loginRequest()
	req.Email = "target@example.test"
	target, e := loginService(t, loginDependencies(t, ctx, f, 80)).Login(ctx, req)
	if e != nil {
		t.Fatal(e)
	}
	actor := selfActor(t, ctx, f, initial.AccessToken, method)
	return ctx, actor, f, initial, target
}
func adminDependencies(f *fixtures.Schema) session.AdminDependencies {
	return session.AdminDependencies{Transactions: productionProvider(f), Bind: session.BindAdminRepositories, Now: func() time.Time { return shared.ServiceTime }, EventID: eventIDs()}
}
func adminService(t *testing.T, d session.AdminDependencies) session.AdminService {
	t.Helper()
	s, e := session.NewAdminService(d)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func postgresAdminSingle(t *testing.T) {
	ctx, actor, f, _, target := adminFixture(t, sessionv1.SessionService_RevokeUserSession_FullMethodName)
	reqLogin := loginRequest()
	reqLogin.Email = "target@example.test"
	other, e := loginService(t, loginDependencies(t, ctx, f, 160)).Login(ctx, reqLogin)
	if e != nil {
		t.Fatal(e)
	}
	svc := adminService(t, adminDependencies(f))
	req := session.AdminSingleInput{UserID: 2, SessionID: target.SessionID, Key: "shared-key"}
	for n := 0; n < 2; n++ {
		r, e := svc.RevokeUserSession(actor, req)
		if e != nil || r == nil || !r.Changed || r.Version != 1 {
			t.Fatal("single response/replay", e)
		}
	}
	assertAdminCounts(t, ctx, f, 1, 1, 0, 1, 1, 1, 1)
	noop := req
	noop.Key = "new-noop-key"
	r, e := svc.RevokeUserSession(actor, noop)
	if e != nil || r == nil || r.Changed {
		t.Fatal("admin terminal no-op", e)
	}
	assertAdminCounts(t, ctx, f, 1, 1, 1, 1, 2, 1, 1)
	// Same key has an independent canonical scope for a different session target.
	otherReq := req
	otherReq.SessionID = other.SessionID
	r, e = svc.RevokeUserSession(actor, otherReq)
	if e != nil || r == nil || !r.Changed {
		t.Fatal("different target scope", e)
	}
	assertAdminCounts(t, ctx, f, 2, 2, 1, 2, 3, 1, 1)
	// Owned fixture makes the first record expired while retaining a valid actor.
	if _, e = f.DB.ExecContext(ctx, `UPDATE idempotency_records SET created_at=$1,expires_at=$2 WHERE route_scope=$3 AND key=$4`, shared.ServiceTime.Add(-25*time.Hour), shared.ServiceTime.Add(-time.Hour), "user/2/session/"+target.SessionID, req.Key); e != nil {
		t.Fatal("expiry fixture", e)
	}
	r, e = svc.RevokeUserSession(actor, req)
	if e != nil || r == nil || r.Changed {
		t.Fatal("expired result not replaced by new no-op", e)
	}
	assertAdminCounts(t, ctx, f, 2, 2, 2, 2, 3, 1, 1)
	var renewed, actorCorrect bool
	e = f.DB.QueryRowContext(ctx, `SELECT (SELECT created_at=$1 AND expires_at=$2 FROM idempotency_records WHERE route_scope=$3 AND key=$4),(SELECT bool_and(payload->>'actor_user_id'='1' AND payload->'data'->>'user_id'='2') FROM outbox_events)`, shared.ServiceTime, shared.ServiceTime.Add(24*time.Hour), "user/2/session/"+target.SessionID, req.Key).Scan(&renewed, &actorCorrect)
	if e != nil || !renewed || !actorCorrect {
		t.Fatal("24h replacement/actual admin event actor", e)
	}
	// Real wrong-owner session binding must never mutate the actor's own session.
	var actorSession string
	if e = f.DB.QueryRowContext(ctx, `SELECT id FROM sessions WHERE user_id=1`).Scan(&actorSession); e != nil {
		t.Fatal(e)
	}
	if r, e = svc.RevokeUserSession(actor, session.AdminSingleInput{UserID: 2, SessionID: actorSession, Key: "idor"}); r != nil || !errors.Is(e, common.ErrForbidden) {
		t.Fatal("admin session user IDOR", e)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("single admin pool leak")
	}
}

func assertAdminCounts(t *testing.T, ctx context.Context, f *fixtures.Schema, revoked, terminal, noops, events, records, version, authVersion int) {
	t.Helper()
	var a, b, c, d, e, v, av int
	var actorActive bool
	err := f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE user_id=2 AND status='REVOKED'),(SELECT count(*) FROM audit_logs WHERE action='SESSION_REVOKED'),(SELECT count(*) FROM audit_logs WHERE action='NO_OP'),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM idempotency_records),version,auth_version,(SELECT bool_and(status='ACTIVE') FROM sessions WHERE user_id=1) FROM auth WHERE id=2`).Scan(&a, &b, &c, &d, &e, &v, &av, &actorActive)
	if err != nil || a != revoked || b != terminal || c != noops || d != events || e != records || v != version || av != authVersion || !actorActive {
		t.Fatal("admin counts/scope", a, b, c, d, e, v, av, actorActive, err)
	}
}

func postgresAdminGlobal(t *testing.T) {
	ctx, actor, f, _, target := adminFixture(t, sessionv1.SessionService_RevokeAllUserSessions_FullMethodName)
	svc := adminService(t, adminDependencies(f))
	req := session.AdminGlobalInput{UserID: 2, ExpectedVersion: 1, Key: "global-key"}
	for n := 0; n < 2; n++ {
		r, e := svc.RevokeAllUserSessions(actor, req)
		if e != nil || r == nil || !r.Changed || r.Version != 2 {
			t.Fatal("global response/replay before version", e)
		}
	}
	conflict := req
	conflict.ExpectedVersion = 2
	if r, e := svc.RevokeAllUserSessions(actor, conflict); r != nil || !errors.Is(e, user.ErrIdempotencyConflict) {
		t.Fatal("same-key payload conflict", e)
	}
	stale := req
	stale.Key = "new-stale"
	if r, e := svc.RevokeAllUserSessions(actor, stale); r != nil || !errors.Is(e, user.ErrVersionConflict) {
		t.Fatal("new-key stale target version", e)
	}
	next := conflict
	next.Key = "next-global"
	if r, e := svc.RevokeAllUserSessions(actor, next); e != nil || r == nil || !r.Changed || r.Version != 3 {
		t.Fatal("empty target generation invalidation", e)
	}
	var v, av, active, refresh, audits, events, terminal, records int
	err := f.DB.QueryRowContext(ctx, `SELECT version,auth_version,(SELECT count(*) FROM sessions WHERE user_id=2 AND status='ACTIVE'),(SELECT count(*) FROM refresh_tokens rt JOIN sessions s ON s.id=rt.session_id WHERE s.user_id=2 AND rt.revoked_at IS NOT NULL),(SELECT count(*) FROM audit_logs WHERE action='USER_SESSIONS_REVOKED'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.user.sessions_revoked.v1' AND aggregate_id='2' AND aggregate_version IN (2,3) AND published_at IS NULL AND payload->>'actor_user_id'='1'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.session.revoked.v1'),(SELECT count(*) FROM idempotency_records) FROM auth WHERE id=2`).Scan(&v, &av, &active, &refresh, &audits, &events, &terminal, &records)
	if err != nil || v != 3 || av != 3 || active != 0 || refresh != 1 || audits != 2 || events != 2 || terminal != 0 || records != 2 {
		t.Fatal("global admin counts", v, av, active, refresh, audits, events, terminal, records, err)
	}
	rctx := refreshCaller(t, ctx)
	if r, e := refreshService(t, refreshDependencies(t, rctx, f, 160)).Refresh(rctx, target.RefreshToken); r != nil || !errors.Is(e, auth.ErrInvalidSession) {
		t.Fatal("global old refresh accepted", e)
	}
	login := loginRequest()
	login.Email = "target@example.test"
	fresh, e := loginService(t, loginDependencies(t, ctx, f, 230)).Login(ctx, login)
	if e != nil || fresh == nil {
		t.Fatal("fresh target Login", e)
	}
	var generation int
	if e = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, fresh.SessionID).Scan(&generation); e != nil || generation != 3 {
		t.Fatal("target generation", e)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("global admin pool leak")
	}
}

func postgresAdminSelf(t *testing.T) {
	for _, mode := range []string{"single", "global"} {
		t.Run(mode, func(t *testing.T) {
			method := sessionv1.SessionService_RevokeUserSession_FullMethodName
			if mode == "global" {
				method = sessionv1.SessionService_RevokeAllUserSessions_FullMethodName
			}
			ctx, actor, f, initial, _ := adminFixture(t, method)
			svc := adminService(t, adminDependencies(f))
			call := func() (*session.AdminResult, error) {
				if mode == "single" {
					return svc.RevokeUserSession(actor, session.AdminSingleInput{UserID: 1, SessionID: initial.SessionID, Key: "self-key"})
				}
				return svc.RevokeAllUserSessions(actor, session.AdminGlobalInput{UserID: 1, ExpectedVersion: 1, Key: "self-key"})
			}
			if r, e := call(); e != nil || r == nil || !r.Changed {
				t.Fatal("self admin first command", e)
			}
			if r, e := call(); r != nil || !errors.Is(e, auth.ErrInvalidSession) {
				t.Fatal("safe replay bypassed revoked actor", e)
			}
			var records, events int
			if e := f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM idempotency_records),(SELECT count(*) FROM outbox_events)`).Scan(&records, &events); e != nil || records != 1 || events != 1 {
				t.Fatal("self replay duplicate effect", e)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("self admin pool leak")
			}
		})
	}
}

func postgresAdminFailures(t *testing.T) {
	for _, mode := range []string{"single", "global"} {
		for _, stage := range []string{"counters", "sessions", "refresh", "audit", "outbox", "result", "commit", "event id", "cancel", "panic"} {
			if mode == "single" && stage == "counters" {
				continue
			}
			t.Run(mode+"/"+stage, func(t *testing.T) {
				method := sessionv1.SessionService_RevokeUserSession_FullMethodName
				if mode == "global" {
					method = sessionv1.SessionService_RevokeAllUserSessions_FullMethodName
				}
				ctx, actor, f, _, target := adminFixture(t, method)
				actor, cancel := context.WithCancel(actor)
				defer cancel()
				d := adminDependencies(f)
				switch stage {
				case "event id":
					d.EventID = func() (string, error) { return "", errors.New("controlled entropy") }
				case "cancel":
					d.EventID = func() (string, error) { cancel(); return "", actor.Err() }
				case "panic":
					d.EventID = func() (string, error) { panic("private panic") }
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
					if stage == "result" {
						table = "idempotency_records"
						op = "INSERT"
					}
					trigger := "CREATE TRIGGER reject_admin AFTER " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION reject_admin()"
					if stage == "commit" {
						trigger = "CREATE CONSTRAINT TRIGGER reject_admin AFTER INSERT ON idempotency_records DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_admin()"
					}
					if _, e := f.DB.ExecContext(ctx, `CREATE FUNCTION reject_admin() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF `+condition+` THEN RAISE EXCEPTION 'controlled failure' USING ERRCODE='23514'; END IF; RETURN NEW; END $$; `+trigger); e != nil {
						t.Fatal("admin fault setup", e)
					}
				}
				observed := 0
				attempt, e := interceptors.NewSecurityAttemptInterceptor(func(c context.Context, r domain.AuditRequest) error {
					assertAdminUnchanged(t, c, f)
					return featurecommon.RecordSecurityAttempt(c, productionProvider(f), r)
				}, func() { observed++ }, func() time.Time { return shared.ServiceTime })
				if e != nil {
					t.Fatal(e)
				}
				svc := adminService(t, d)
				_, e = attempt(actor, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(c context.Context, _ any) (any, error) {
					if mode == "single" {
						return svc.RevokeUserSession(c, session.AdminSingleInput{UserID: 2, SessionID: target.SessionID, Key: "failure-key"})
					}
					return svc.RevokeAllUserSessions(c, session.AdminGlobalInput{UserID: 2, ExpectedVersion: 1, Key: "failure-key"})
				})
				want := error(common.ErrDependencyUnavailable)
				if stage == "cancel" {
					want = context.Canceled
				}
				if stage == "panic" {
					want = common.ErrInternalServerError
				}
				if !errors.Is(e, want) || observed != 0 {
					t.Fatal("admin fault/audit outcome", e, observed)
				}
				var audits int
				if e = f.DB.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND actor_service='rest-orc'`).Scan(&audits); e != nil || audits != 1 {
					t.Fatal("admin independent audit", e)
				}
				if f.DB.Stats().InUse != 0 {
					t.Fatal("admin fault pool leak")
				}
			})
		}
	}
}
func assertAdminUnchanged(t *testing.T, ctx context.Context, f *fixtures.Schema) {
	t.Helper()
	var sessions, tokens, audits, events, records, v, av int
	e := f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE status='ACTIVE'),(SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL),(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM idempotency_records),version,auth_version FROM auth WHERE id=2`).Scan(&sessions, &tokens, &audits, &events, &records, &v, &av)
	if e != nil || sessions != 2 || tokens != 2 || audits != 2 || events != 0 || records != 0 || v != 1 || av != 1 {
		t.Fatal("partial admin writes survived rollback", sessions, tokens, audits, events, records, v, av, e)
	}
}

type adminOutcome struct {
	result *session.AdminResult
	err    error
}

func postgresAdminContention(t *testing.T) {
	for _, mode := range []string{"single", "global"} {
		t.Run(mode, func(t *testing.T) {
			method := sessionv1.SessionService_RevokeUserSession_FullMethodName
			if mode == "global" {
				method = sessionv1.SessionService_RevokeAllUserSessions_FullMethodName
			}
			ctx, actor, f, _, target := adminFixture(t, method)
			d := adminDependencies(f)
			first := &observedProvider{inner: d.Transactions}
			d.Transactions = first
			ready, release := make(chan struct{}), make(chan struct{})
			done := make(chan adminOutcome, 1)
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			finished := false
			defer func() {
				unblock()
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("admin worker cleanup")
					}
				}
			}()
			eventID := d.EventID
			d.EventID = func() (string, error) {
				close(ready)
				select {
				case <-release:
				case <-ctx.Done():
					return "", ctx.Err()
				}
				return eventID()
			}
			call := func(s session.AdminService) (*session.AdminResult, error) {
				if mode == "single" {
					return s.RevokeUserSession(actor, session.AdminSingleInput{UserID: 2, SessionID: target.SessionID, Key: "concurrent-key"})
				}
				return s.RevokeAllUserSessions(actor, session.AdminGlobalInput{UserID: 2, ExpectedVersion: 1, Key: "concurrent-key"})
			}
			svc := adminService(t, d)
			go func() { r, e := call(svc); done <- adminOutcome{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("admin failed before barrier", r.err)
			case <-ctx.Done():
				t.Fatal("admin barrier timeout")
			}
			assertAdminUnchanged(t, ctx, f)
			other := adminDependencies(f)
			second := &observedProvider{inner: other.Transactions}
			other.Transactions = second
			if r, e := call(adminService(t, other)); r != nil || !errors.Is(e, auth.ErrResourceBusy) || first.pid == 0 || second.pid == 0 || first.pid == second.pid {
				t.Fatal("same-key contention/two-backend proof", e)
			}
			t.Logf("admin %s holder backend=%d idem contender backend=%d", mode, first.pid, second.pid)
			if mode == "global" {
				r, e := adminService(t, other).RevokeAllUserSessions(actor, session.AdminGlobalInput{UserID: 2, ExpectedVersion: 1, Key: "competing-version-key"})
				if r != nil || !errors.Is(e, auth.ErrResourceBusy) {
					t.Fatal("different-key same-version contender", e)
				}
			}

			unblock()
			var winner adminOutcome
			select {
			case winner = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("admin winner timeout")
			}
			if winner.err != nil || winner.result == nil {
				t.Fatal("admin winner", winner.err)
			}
			if r, e := call(adminService(t, adminDependencies(f))); e != nil || r == nil || *r != *winner.result {
				t.Fatal("postcommit safe replay", e)
			}
			if mode == "global" {
				r, e := adminService(t, adminDependencies(f)).RevokeAllUserSessions(actor, session.AdminGlobalInput{UserID: 2, ExpectedVersion: 1, Key: "competing-version-key"})
				if r != nil || !errors.Is(e, user.ErrVersionConflict) {
					t.Fatal("postcommit stale concurrent version", e)
				}
			}
			// Hold the shared policy guard on a distinct backend while revoking the
			// actor's permission. Same-key replay must be busy, then denied.
			tx, e := productionProvider(f).Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			owner := featurecommon.NewTransactionOwner(ctx, tx)
			defer owner.Rollback()
			if e = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, "auth:guard:policy:global", featurecommon.UserLockKey(1), "auth:role:2", "auth:application:1"); e != nil {
				t.Fatal(e)
			}
			var holder struct {
				PID int `sql:"column:pid"`
			}
			if e = tx.UseModel(domain.SessionUserState{}).Select("pg_backend_pid() AS pid").Limit(1).Scan(&holder); e != nil {
				t.Fatal(e)
			}
			if _, e = tx.UseModel(&adminRoleStatus{Status: "DISABLED"}).Where("id = ?", 2).Updates(); e != nil {
				t.Fatal("policy revoke", e)
			}
			third := adminDependencies(f)
			contender := &observedProvider{inner: third.Transactions}
			third.Transactions = contender
			if r, e := call(adminService(t, third)); r != nil || !errors.Is(e, auth.ErrResourceBusy) || holder.PID == contender.pid || contender.pid == 0 {
				t.Fatal("policy guard/replay contention", e)
			}
			t.Logf("policy holder backend=%d replay contender backend=%d", holder.PID, contender.pid)
			if e = owner.Commit(); e != nil {
				t.Fatal(e)
			}
			if r, e := call(adminService(t, adminDependencies(f))); r != nil || !errors.Is(e, common.ErrForbidden) {
				t.Fatal("replay bypassed current permission", e)
			}
			var records, events, audits int
			if e = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM idempotency_records),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM audit_logs WHERE action IN ('SESSION_REVOKED','USER_SESSIONS_REVOKED'))`).Scan(&records, &events, &audits); e != nil || records != 1 || events != 1 || audits != 1 {
				t.Fatal("replay/contenders duplicated effects", e)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("admin contention pool leak")
			}
		})
	}
}

type adminRoleStatus struct {
	Status string `sql:"column:status"`
}

func (adminRoleStatus) TableName() string { return "role" }
