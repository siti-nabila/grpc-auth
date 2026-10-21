//go:build postgres

package test_scenarios

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func RefreshLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "real rotation and repeated reuse preserve family expiry and terminal event uniqueness", Run: postgresRefresh}, {Name: "real rotation faults rollback successor predecessor and audit", Run: postgresRefreshFailures}, {Name: "two-backend barriers prove busy then postcommit reuse and cleanup", Run: postgresRefreshContention}, {Name: "required reuse security writes and deferred commit failures rollback all revocation", Run: postgresRefreshReuseFailures}}
}

func postgresRefreshReuseFailures(t *testing.T) {
	for _, stage := range []string{"session", "family", "reuse audit", "terminal audit", "outbox", "commit", "event id"} {
		t.Run(stage, func(t *testing.T) {
			ctx, f := loginFixture(t, false)
			initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			ctx = refreshCaller(t, ctx)
			next, err := refreshService(t, refreshDependencies(t, ctx, f, 80)).Refresh(ctx, initial.RefreshToken)
			if err != nil {
				t.Fatal(err)
			}
			d := refreshDependencies(t, ctx, f, 160)
			table := "sessions"
			operation := "UPDATE"
			condition := "TRUE"
			switch stage {
			case "family":
				table = "refresh_tokens"
			case "reuse audit":
				table = "audit_logs"
				operation = "INSERT"
				condition = "NEW.action='REFRESH_REUSE_DETECTED'"
			case "terminal audit":
				table = "audit_logs"
				operation = "INSERT"
				condition = "NEW.action='SESSION_REVOKED'"
			case "outbox", "commit":
				table = "outbox_events"
				operation = "INSERT"
			case "event id":
				d.EventID = func() (string, error) { return "", errors.New("controlled entropy failure") }
			}
			if stage != "event id" {
				trigger := "CREATE TRIGGER reject_reuse AFTER " + operation + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION reject_reuse()"
				if stage == "commit" {
					trigger = "CREATE CONSTRAINT TRIGGER reject_reuse AFTER INSERT ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_reuse()"
				}
				statement := `CREATE FUNCTION reject_reuse() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF ` + condition + ` THEN RAISE EXCEPTION 'controlled failure' USING ERRCODE='23514'; END IF; RETURN NEW; END $$; ` + trigger
				if _, err = f.DB.ExecContext(ctx, statement); err != nil {
					t.Fatal("security fault setup", err)
				}
			}
			result, err := refreshService(t, d).Refresh(ctx, initial.RefreshToken)
			if result != nil || !errors.Is(err, common.ErrDependencyUnavailable) || errors.Is(err, auth.ErrRefreshTokenReused) {
				t.Fatal("failed security write claimed committed reuse", err)
			}
			var active, unrevoked, audits, events, version, authVersion int
			err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE status='ACTIVE'),(SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NULL),(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM outbox_events),version,auth_version FROM auth WHERE id=1`).Scan(&active, &unrevoked, &audits, &events, &version, &authVersion)
			if err != nil || active != 1 || unrevoked != 2 || audits != 2 || events != 0 || version != 1 || authVersion != 1 {
				t.Fatal("partial security writes survived rollback", active, unrevoked, audits, events, version, authVersion, err)
			}
			if stage != "event id" {
				if _, err = f.DB.ExecContext(ctx, "DROP TRIGGER reject_reuse ON "+table); err != nil {
					t.Fatal("security fault cleanup", err)
				}
			}
			// The latest successor remains usable after a failed security transaction.
			if _, err = refreshService(t, refreshDependencies(t, ctx, f, 230)).Refresh(ctx, next.RefreshToken); err != nil {
				t.Fatal("failed security transaction corrupted successor", err)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("security rollback pool leak")
			}
		})
	}
}

func refreshDependencies(t *testing.T, ctx context.Context, f *fixtures.Schema, offset byte) authfeature.RefreshDependencies {
	t.Helper()
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	primary := orm.NewSqlQueryAdapter(ctx, f.DB, dialect.NewPostgres(), cfg)
	entropy := loginEntropy(offset)
	issuer, err := jwt.NewAccessIssuer(shared.AccessIssuer, shared.AccessKey())
	if err != nil {
		t.Fatal(err)
	}
	return authfeature.RefreshDependencies{Transactions: productionProvider(f), Tokens: reader.NewRefreshReader(primary), State: reader.NewSessionStateReader(primary), Bind: authfeature.BindRefreshRepositories, Admit: func(context.Context, string) error { return nil }, Credentials: func() (authfeature.RotatedCredentials, error) { return authfeature.GenerateRotatedCredentials(entropy) }, Issue: issuer.Issue, EventID: eventIDs(), Now: func() time.Time { return shared.ServiceTime }}
}
func refreshService(t *testing.T, d authfeature.RefreshDependencies) authfeature.RefreshService {
	t.Helper()
	s, err := authfeature.NewRefreshService(d)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func refreshCaller(t *testing.T, ctx context.Context) context.Context {
	t.Helper()
	return shared.CallerContext(t, ctx, authv1.AuthService_RefreshSession_FullMethodName, authv1.AuthService_RefreshSession_FullMethodName)
}

func postgresRefresh(t *testing.T) {
	ctx, f := loginFixture(t, false)
	initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	other, err := loginService(t, loginDependencies(t, ctx, f, 80)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	ctx = refreshCaller(t, ctx)
	d := refreshDependencies(t, ctx, f, 160)
	svc := refreshService(t, d)
	next, err := svc.Refresh(ctx, initial.RefreshToken)
	if err != nil || next == nil || next.SessionID != initial.SessionID || next.RefreshToken == initial.RefreshToken || !next.SessionExpiresAt.Equal(initial.SessionExpiresAt) {
		t.Fatal("rotation failed", err)
	}
	var safe bool
	digest := sha256.Sum256([]byte(initial.RefreshToken))
	err = f.DB.QueryRowContext(ctx, `SELECT p.consumed_at IS NOT NULL AND p.replaced_by=n.id AND p.family_id=n.family_id AND p.session_id=n.session_id AND p.expires_at=n.expires_at AND n.consumed_at IS NULL AND n.revoked_at IS NULL FROM refresh_tokens p JOIN refresh_tokens n ON p.replaced_by=n.id WHERE p.token_hash=$1`, digest[:]).Scan(&safe)
	if err != nil || !safe {
		t.Fatal("rotation FK/expiry/single-use proof", err)
	}
	for n := 0; n < 2; n++ {
		result, err := svc.Refresh(ctx, initial.RefreshToken)
		if result != nil || !errors.Is(err, auth.ErrRefreshTokenReused) {
			t.Fatal("reuse outcome", err)
		}
	}
	result, err := svc.Refresh(ctx, next.RefreshToken)
	if result != nil || !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("successor remained valid after reuse", err)
	}
	var revoked, rotations, reuses, terminal, events, version, authVersion, otherActive int
	err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM refresh_tokens WHERE session_id=$1 AND revoked_at IS NOT NULL),(SELECT count(*) FROM audit_logs WHERE action='SESSION_REFRESH_ROTATED'),(SELECT count(*) FROM audit_logs WHERE action='REFRESH_REUSE_DETECTED'),(SELECT count(*) FROM audit_logs WHERE action='SESSION_REVOKED'),(SELECT count(*) FROM outbox_events WHERE aggregate_id=$1::text AND aggregate_version=2 AND published_at IS NULL AND event_type='iam.session.revoked.v1'),a.version,a.auth_version,(SELECT count(*) FROM sessions WHERE id=$2 AND status='ACTIVE') FROM auth a WHERE id=1`, initial.SessionID, other.SessionID).Scan(&revoked, &rotations, &reuses, &terminal, &events, &version, &authVersion, &otherActive)
	if err != nil || revoked != 2 || rotations != 1 || reuses != 2 || terminal != 1 || events != 1 || version != 1 || authVersion != 1 || otherActive != 1 {
		t.Fatal("terminal/security counts", revoked, rotations, reuses, terminal, events, version, authVersion, otherActive, err)
	}
	// Losing the committed response never restores the predecessor or caches credentials.
	if _, err = refreshService(t, refreshDependencies(t, ctx, f, 230)).Refresh(ctx, other.RefreshToken); err != nil {
		t.Fatal("other device invalidated", err)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("refresh pool leak")
	}
}

func postgresRefreshFailures(t *testing.T) {
	for _, stage := range []string{"insert", "consume", "zero consume", "audit", "sign", "commit"} {
		t.Run(stage, func(t *testing.T) {
			ctx, f := loginFixture(t, false)
			initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			ctx = refreshCaller(t, ctx)
			d := refreshDependencies(t, ctx, f, 80)
			if stage == "sign" {
				d.Issue = func(jwt.AccessIssue) (jwt.IssuedAccess, error) {
					return jwt.IssuedAccess{}, errors.New("controlled signer failure")
				}
			} else {
				statement := `CREATE FUNCTION reject_rotation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled failure' USING ERRCODE='23514'; END $$; `
				switch stage {
				case "insert":
					statement += `CREATE TRIGGER reject_rotation AFTER INSERT ON refresh_tokens FOR EACH ROW EXECUTE FUNCTION reject_rotation()`
				case "consume":
					statement += `CREATE TRIGGER reject_rotation AFTER UPDATE ON refresh_tokens FOR EACH ROW EXECUTE FUNCTION reject_rotation()`
				case "audit":
					statement += `CREATE TRIGGER reject_rotation AFTER INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_rotation()`
				case "commit":
					statement += `CREATE CONSTRAINT TRIGGER reject_rotation AFTER INSERT ON refresh_tokens DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_rotation()`
				case "zero consume":
					statement = `CREATE FUNCTION reject_rotation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$; CREATE TRIGGER reject_rotation BEFORE UPDATE ON refresh_tokens FOR EACH ROW EXECUTE FUNCTION reject_rotation()`
				}
				if _, err = f.DB.ExecContext(ctx, statement); err != nil {
					t.Fatal("controlled fault setup", err)
				}
			}
			result, err := refreshService(t, d).Refresh(ctx, initial.RefreshToken)
			if result != nil || err == nil || stage != "zero consume" && !errors.Is(err, common.ErrDependencyUnavailable) || stage == "zero consume" && !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("failure outcome", err)
			}
			assertRefreshUnchanged(t, ctx, f)
			if stage != "sign" {
				table := "refresh_tokens"
				if stage == "audit" {
					table = "audit_logs"
				}
				if _, err = f.DB.ExecContext(ctx, "DROP TRIGGER reject_rotation ON "+table); err != nil {
					t.Fatal("fault cleanup", err)
				}
			}
			if _, err = refreshService(t, refreshDependencies(t, ctx, f, 160)).Refresh(ctx, initial.RefreshToken); err != nil {
				t.Fatal("pool/lock unusable after rollback", err)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("failed rotation pool leak")
			}
		})
	}
}

func assertRefreshUnchanged(t *testing.T, ctx context.Context, f *fixtures.Schema) {
	t.Helper()
	var tokens, active, audits, events, version, authVersion int
	var clean bool
	err := f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM refresh_tokens),(SELECT count(*) FROM sessions WHERE status='ACTIVE'),(SELECT count(*) FROM audit_logs),(SELECT count(*) FROM outbox_events),version,auth_version,(SELECT bool_and(consumed_at IS NULL AND revoked_at IS NULL AND replaced_by IS NULL) FROM refresh_tokens) FROM auth WHERE id=1`).Scan(&tokens, &active, &audits, &events, &version, &authVersion, &clean)
	if err != nil || tokens != 1 || active != 1 || audits != 1 || events != 0 || version != 1 || authVersion != 1 || !clean {
		t.Fatal("partial refresh writes survived rollback", tokens, active, audits, events, version, authVersion, clean, err)
	}
}

func postgresRefreshContention(t *testing.T) {
	for _, outcome := range []string{"commit", "failure", "panic", "cancellation"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, f := loginFixture(t, false)
			initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			ctx = refreshCaller(t, ctx)
			requestCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			d := refreshDependencies(t, requestCtx, f, 80)
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
			sign := d.Issue
			d.Issue = func(r jwt.AccessIssue) (jwt.IssuedAccess, error) {
				close(ready)
				select {
				case <-release:
				case <-requestCtx.Done():
					return jwt.IssuedAccess{}, requestCtx.Err()
				}
				if outcome == "failure" {
					return jwt.IssuedAccess{}, errors.New("controlled sign failure")
				}
				if outcome == "panic" {
					panic("controlled private panic")
				}
				return sign(r)
			}
			svc := refreshService(t, d)
			go func() { r, e := svc.Refresh(requestCtx, initial.RefreshToken); done <- loginResult{r, e} }()
			select {
			case <-ready:
			case <-ctx.Done():
				t.Fatal("refresh barrier timed out")
			case <-done:
				finished = true
				t.Fatal("refresh failed before barrier")
			}
			// Winner's successor and consumption are still invisible on backend B.
			assertRefreshUnchanged(t, ctx, f)
			other := refreshDependencies(t, ctx, f, 160)
			second := &observedProvider{inner: other.Transactions}
			other.Transactions = second
			r, e := refreshService(t, other).Refresh(ctx, initial.RefreshToken)
			if r != nil || !errors.Is(e, auth.ErrResourceBusy) || first.pid == 0 || second.pid == 0 || first.pid == second.pid {
				t.Fatal("refresh contender missing busy/two-backend proof", e)
			}
			t.Logf("precommit refresh holder backend=%d contender backend=%d", first.pid, second.pid)
			if outcome == "cancellation" {
				cancel()
			} else {
				unblock()
			}
			winner := waitLogin(t, ctx, done)
			finished = true
			if outcome == "commit" {
				if winner.err != nil || winner.response == nil {
					t.Fatal("rotation winner", winner.err)
				}
				r, e = refreshService(t, refreshDependencies(t, ctx, f, 230)).Refresh(ctx, initial.RefreshToken)
				if r != nil || !errors.Is(e, auth.ErrRefreshTokenReused) {
					t.Fatal("postcommit reuse", e)
				}
				r, e = refreshService(t, refreshDependencies(t, ctx, f, 180)).Refresh(ctx, winner.response.RefreshToken)
				if r != nil || !errors.Is(e, auth.ErrInvalidSession) {
					t.Fatal("winner successor remained usable", e)
				}
			} else {
				want := error(common.ErrDependencyUnavailable)
				if outcome == "panic" {
					want = common.ErrInternalServerError
				}
				if outcome == "cancellation" {
					want = context.Canceled
				}
				if winner.response != nil || !errors.Is(winner.err, want) {
					t.Fatal("rollback outcome", winner.err)
				}
				assertRefreshUnchanged(t, ctx, f)
				if _, e = refreshService(t, refreshDependencies(t, ctx, f, 230)).Refresh(ctx, initial.RefreshToken); e != nil {
					t.Fatal("rollback lost original credential", e)
				}
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("refresh terminal pool leak")
			}
		})
	}
}
