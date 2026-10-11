//go:build postgres

package test_scenarios

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func LogoutLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "real Logout preserves other devices terminal no-op and generation invalidation", Run: postgresLogout}, {Name: "required Logout writes cancellation panic and commit faults rollback before independent audit", Run: postgresLogoutFailures}}
}
func logoutDependencies(ctx context.Context, f *fixtures.Schema) authfeature.LogoutDependencies {
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	primary := orm.NewSqlQueryAdapter(ctx, f.DB, dialect.NewPostgres(), cfg)
	return authfeature.LogoutDependencies{Transactions: productionProvider(f), Tokens: reader.NewRefreshReader(primary), State: reader.NewSessionStateReader(primary), Bind: authfeature.BindLogoutRepositories, Now: func() time.Time { return shared.ServiceTime }, EventID: eventIDs()}
}
func logoutService(t *testing.T, d authfeature.LogoutDependencies) authfeature.LogoutService {
	t.Helper()
	s, err := authfeature.NewLogoutService(d)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func logoutProof(t *testing.T, ctx context.Context, access string, refresh *string) (context.Context, sessions.LogoutCredential) {
	t.Helper()
	ctx = shared.CallerContext(t, ctx, authv1.AuthService_Logout_FullMethodName, authv1.AuthService_Logout_FullMethodName)
	if access != "" {
		md, _ := metadata.FromIncomingContext(ctx)
		md = md.Copy()
		md.Set("authorization", "Bearer "+access)
		ctx = metadata.NewIncomingContext(ctx, md)
	}
	v, err := jwt.NewAccessVerifier(shared.AccessIssuer, shared.AccessKey().Public().(ed25519.PublicKey), func() time.Time { return shared.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	proof, err := sessions.VerifyLogoutCredential(ctx, refresh, v)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, proof
}

func postgresLogout(t *testing.T) {
	for _, mode := range []string{"access", "refresh", "restricted", "consumed"} {
		t.Run(mode, func(t *testing.T) {
			ctx, f := loginFixture(t, mode == "restricted")
			initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			other, err := loginService(t, loginDependencies(t, ctx, f, 80)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			latest := initial.RefreshToken
			if mode == "consumed" {
				refreshCtx := refreshCaller(t, ctx)
				next, err := refreshService(t, refreshDependencies(t, refreshCtx, f, 160)).Refresh(refreshCtx, initial.RefreshToken)
				if err != nil {
					t.Fatal(err)
				}
				latest = next.RefreshToken
				oldCtx, oldProof := logoutProof(t, ctx, "", &initial.RefreshToken)
				if r, err := logoutService(t, logoutDependencies(oldCtx, f)).Logout(oldCtx, oldProof); r != nil || !errors.Is(err, auth.ErrInvalidSession) {
					t.Fatal("consumed Logout triggered mutation", err)
				}
			}
			access := ""
			var refresh *string
			if mode == "access" || mode == "restricted" {
				access = initial.AccessToken
			} else {
				refresh = &latest
			}
			actor, proof := logoutProof(t, ctx, access, refresh)
			service := logoutService(t, logoutDependencies(actor, f))
			for n := 0; n < 2; n++ {
				result, err := service.Logout(actor, proof)
				if err != nil || result == nil || result.Changed != (n == 0) {
					t.Fatal("terminal Logout", err)
				}
			}
			refreshCtx, refreshProof := logoutProof(t, ctx, "", &latest)
			if r, err := service.Logout(refreshCtx, refreshProof); err != nil || r == nil || r.Changed {
				t.Fatal("latest revoked refresh repeat Logout", err)
			}
			var revoked, events, audits, reuse, otherActive, version, authVersion int
			err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE id=$1 AND status='REVOKED'),(SELECT count(*) FROM outbox_events WHERE aggregate_id=$1::text AND aggregate_version=2 AND published_at IS NULL),(SELECT count(*) FROM audit_logs WHERE action='SESSION_REVOKED'),(SELECT count(*) FROM audit_logs WHERE action='REFRESH_REUSE_DETECTED'),(SELECT count(*) FROM sessions WHERE id=$2 AND status='ACTIVE'),version,auth_version FROM auth WHERE id=1`, initial.SessionID, other.SessionID).Scan(&revoked, &events, &audits, &reuse, &otherActive, &version, &authVersion)
			if err != nil || revoked != 1 || events != 1 || audits != 1 || reuse != 0 || otherActive != 1 || version != 1 || authVersion != 1 {
				t.Fatal("Logout atomic counts", revoked, events, audits, reuse, otherActive, version, authVersion, err)
			}
			// Controlled invalidation setup follows the same locks and production writers;
			// production revoke-all/password RPC evidence remains in its own tasks.
			tx, err := productionProvider(f).Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			owner := featurecommon.NewTransactionOwner(ctx, tx)
			if err = featurecommon.AcquireLocks(owner, auth.ErrResourceBusy, featurecommon.UserLockKey(1), "auth:session:"+initial.SessionID, "auth:session:"+other.SessionID); err != nil {
				_ = owner.Rollback()
				t.Fatal(err)
			}
			changed, err := writer.NewCredentialInvalidationWriter(tx).Invalidate(ctx, domain.CredentialInvalidation{UserID: 1, ExpectedVersion: 1, ExpectedAuthVersion: 1, At: shared.ServiceTime})
			if err != nil || !changed {
				_ = owner.Rollback()
				t.Fatal("generation fixture writer", err)
			}
			if _, err = writer.NewSessionRevocationWriter(tx).RevokeUserSessions(ctx, domain.SessionRevocation{UserID: 1, ActorID: 1, At: shared.ServiceTime, Reason: types.AuditSelfSessionsRevoked}); err != nil {
				_ = owner.Rollback()
				t.Fatal(err)
			}
			if _, err = writer.NewRefreshRevocationWriter(tx).RevokeUserRefresh(ctx, 1, shared.ServiceTime); err != nil {
				_ = owner.Rollback()
				t.Fatal(err)
			}
			if err = owner.Commit(); err != nil {
				t.Fatal(err)
			}
			if r, err := service.Logout(refreshCtx, refreshProof); r != nil || !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("revoked refresh survived later global generation invalidation", err)
			}
			if r, err := service.Logout(actor, proof); r != nil || !errors.Is(err, auth.ErrInvalidSession) {
				t.Fatal("old access/refresh generation survived", err)
			}
			fresh, err := loginService(t, loginDependencies(t, ctx, f, 230)).Login(ctx, loginRequest())
			if err != nil || fresh == nil {
				t.Fatal("new generation Login failed", err)
			}
			var generation int64
			if err = f.DB.QueryRowContext(ctx, `SELECT auth_version FROM sessions WHERE id=$1`, fresh.SessionID).Scan(&generation); err != nil || generation != 2 {
				t.Fatal("fresh Login generation", err)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("Logout pool leak")
			}
		})
	}
}

func postgresLogoutFailures(t *testing.T) {
	for _, stage := range []string{"session", "zero transition", "refresh", "audit", "outbox", "commit", "event id", "cancel", "panic"} {
		t.Run(stage, func(t *testing.T) {
			ctx, f := loginFixture(t, false)
			initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			actor, proof := logoutProof(t, ctx, initial.AccessToken, nil)
			actor, cancel := context.WithCancel(actor)
			defer cancel()
			d := logoutDependencies(actor, f)
			if stage == "event id" {
				d.EventID = func() (string, error) { return "", errors.New("controlled event entropy") }
			} else if stage == "cancel" {
				d.EventID = func() (string, error) { cancel(); return "", actor.Err() }
			} else if stage == "panic" {
				d.EventID = func() (string, error) { panic("controlled private panic") }
			} else {
				table, operation := "sessions", "UPDATE"
				condition := "TRUE"
				if stage == "refresh" {
					table = "refresh_tokens"
				}
				if stage == "audit" {
					table = "audit_logs"
					operation = "INSERT"
					condition = "NEW.action='SESSION_REVOKED'"
				}
				if stage == "outbox" || stage == "commit" {
					table = "outbox_events"
					operation = "INSERT"
				}
				body := `IF ` + condition + ` THEN RAISE EXCEPTION 'controlled failure' USING ERRCODE='23514'; END IF; RETURN NEW;`
				if stage == "zero transition" {
					body = "RETURN NULL;"
				}
				trigger := "CREATE TRIGGER reject_logout AFTER " + operation + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION reject_logout()"
				if stage == "zero transition" {
					trigger = "CREATE TRIGGER reject_logout BEFORE UPDATE ON sessions FOR EACH ROW EXECUTE FUNCTION reject_logout()"
				}
				if stage == "commit" {
					trigger = "CREATE CONSTRAINT TRIGGER reject_logout AFTER INSERT ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_logout()"
				}
				if _, err = f.DB.ExecContext(ctx, `CREATE FUNCTION reject_logout() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN `+body+` END $$; `+trigger); err != nil {
					t.Fatal("Logout fault setup", err)
				}
			}
			observed := 0
			attempt, err := interceptors.NewSecurityAttemptInterceptor(func(attemptCtx context.Context, r domain.AuditRequest) error {
				assertRefreshUnchanged(t, attemptCtx, f)
				return featurecommon.RecordSecurityAttempt(attemptCtx, productionProvider(f), r)
			}, func() { observed++ }, func() time.Time { return shared.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			_, err = attempt(actor, nil, &grpc.UnaryServerInfo{FullMethod: authv1.AuthService_Logout_FullMethodName}, func(c context.Context, _ any) (any, error) { return logoutService(t, d).Logout(c, proof) })
			want := error(common.ErrDependencyUnavailable)
			if stage == "zero transition" {
				want = auth.ErrInvalidSession
			}
			if stage == "cancel" {
				want = context.Canceled
			}
			if stage == "panic" {
				want = common.ErrInternalServerError
			}
			if !errors.Is(err, want) || observed != 0 {
				t.Fatal("Logout fault/audit outcome", err, observed)
			}
			var audits, events, active int
			if err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND actor_service='rest-orc'),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM sessions WHERE status='ACTIVE')`).Scan(&audits, &events, &active); err != nil || audits != 1 || events != 0 || active != 1 {
				t.Fatal("safe audit after rollback", audits, events, active, err)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("Logout fault pool leak")
			}
		})
	}
}
