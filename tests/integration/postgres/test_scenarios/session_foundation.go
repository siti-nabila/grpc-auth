//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func SessionFoundation() []testutils.Scenario {
	return []testutils.Scenario{{Name: "Login-produced Me and ListUsers share primary authorization and data snapshots", Run: postgresSnapshots}, {Name: "pending Register and Profile required writes are atomic", Run: postgresFoundationCommands}, {Name: "Register rejects inactive or privilege-bearing default member", Run: postgresRegisterMemberPolicy}}
}

func foundationActor(t *testing.T, ctx context.Context, fixture *fixtures.Schema, method string) context.Context {
	t.Helper()
	response, err := loginService(t, loginDependencies(t, ctx, fixture, 0)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal("real Login prerequisite failed", err)
	}
	security := loginSecurity(t, func() time.Time { return shared.ServiceTime })
	claims, err := security.Verifier.Verify(response.AccessToken)
	if err != nil {
		t.Fatal("committed Login credential failed verification")
	}
	ctx = loginCaller(t, ctx, method)
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	ctx, err = authfeature.AuthenticateSession(ctx, claims, reader.NewSessionStateReader(orm.NewSqlQueryAdapter(ctx, fixture.DB, dialect.NewPostgres(), cfg)), shared.ServiceTime)
	if err != nil {
		t.Fatal("Login-produced principal rejected", err)
	}
	return ctx
}

func foundationProfileDeps(fixture *fixtures.Schema) profile.SessionDependencies {
	return profile.SessionDependencies{Transactions: productionProvider(fixture), Bind: profile.BindSessionRepositories, Now: func() time.Time { return shared.ServiceTime }, EventID: eventIDs()}
}

func eventIDs() func() (string, error) {
	n := 0
	return func() (string, error) { n++; return fmt.Sprintf("00000000-0000-4000-8000-%012d", n), nil }
}

func pendingRegistrationDeps(fixture *fixtures.Schema) authfeature.PendingRegistrationDependencies {
	return authfeature.PendingRegistrationDependencies{Transactions: productionProvider(fixture), Bind: authfeature.BindPendingRegistrationRepositories, HashPassword: func(string) (string, error) { return shared.LoginPasswordHash(), nil }, Now: func() time.Time { return shared.ServiceTime }, EventID: eventIDs()}
}

type snapshotBarrier struct {
	provider domain.ReadSnapshotProvider
	ready    chan struct{}
	release  <-chan struct{}
	pid      int
}

func (p *snapshotBarrier) BeginSnapshot(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.provider.BeginSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	var row struct {
		PID int `sql:"column:pid"`
	}
	// This SELECT establishes the real RR snapshot before the concurrent commit.
	if err = tx.UseModel(domain.Auth{}).Select("pg_backend_pid() AS pid").Limit(1).Scan(&row); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	p.pid = row.PID
	close(p.ready)
	select {
	case <-p.release:
		return tx, nil
	case <-ctx.Done():
		_ = tx.Rollback()
		return nil, ctx.Err()
	}
}

type snapshotResult struct {
	res any
	err error
}

func postgresSnapshots(t *testing.T) {
	for _, kind := range []string{"Me role", "List role", "List application", "empty List permission"} {
		t.Run(kind, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			method := userv1.UserService_ListUsers_FullMethodName
			if kind == "Me role" {
				method = userv1.UserService_Me_FullMethodName
			}
			ctx = foundationActor(t, ctx, fixture, method)
			if _, err := fixture.DB.ExecContext(ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.read','fixture'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Reader',2,1,'iam.reader'); INSERT INTO role_permissions(role_id,permission_id) VALUES(2,1); INSERT INTO user_role(user_id,role_id) VALUES(1,2)`); err != nil {
				t.Fatal("read grant fixture setup failed")
			}
			provider := database.NewTransactionProvider(fixture.DB, dialect.NewPostgres(), database.GetORMConfig())
			ready, release := make(chan struct{}), make(chan struct{})
			barrier := &snapshotBarrier{provider: provider, ready: ready, release: release}
			reads, err := handler.NewSessionReadHandler(barrier, func() time.Time { return shared.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			req := &userv1.ListUsersRequest{Query: &paginatorv1.PageQuery{Limit: 10, LastId: "0"}, Filter: &userv1.UserFilter{RoleCodes: []uint64{1, 2}}}
			if kind == "empty List permission" {
				req.Query.LastId = "999"
			}
			done := make(chan snapshotResult, 1)
			finished := false
			released := false
			defer func() {
				if !released {
					close(release)
				}
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("snapshot worker cleanup failed")
					}
				}
			}()
			go func() {
				if method == userv1.UserService_Me_FullMethodName {
					res, err := reads.Me(ctx, nil)
					done <- snapshotResult{res, err}
				} else {
					res, err := reads.ListUsers(ctx, req)
					done <- snapshotResult{res, err}
				}
			}()
			select {
			case <-ready:
			case got := <-done:
				finished = true
				t.Fatal("snapshot failed before barrier", got.err)
			case <-ctx.Done():
				t.Fatal("snapshot barrier timed out")
			}
			conn, err := fixture.DB.Conn(ctx)
			if err != nil {
				t.Fatal("second read backend unavailable")
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, fixture.Name)
			if pid == barrier.pid || barrier.pid == 0 {
				t.Fatal("snapshot proof requires distinct physical backends")
			}
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			query := `UPDATE role SET status='DISABLED' WHERE id=2`
			if kind == "List application" {
				query = `UPDATE applications SET status='DISABLED' WHERE id=1`
			}
			if kind == "empty List permission" {
				query = `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2`
			}
			if _, err = tx.ExecContext(ctx, query); err != nil {
				t.Fatal("privilege change failed")
			}
			if _, err = tx.ExecContext(ctx, `UPDATE profile SET name='Committed later' WHERE user_id=1`); err != nil || tx.Commit() != nil {
				t.Fatal("concurrent change did not commit")
			}
			close(release)
			released = true
			var got snapshotResult
			select {
			case got = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("snapshot consumer timed out")
			}
			if got.err != nil {
				t.Fatal("valid prior snapshot denied", got.err)
			}
			fresh, _ := handler.NewSessionReadHandler(provider, func() time.Time { return shared.ServiceTime })
			if method == userv1.UserService_Me_FullMethodName {
				old := got.res.(*userv1.UserData)
				next, err := fresh.Me(ctx, nil)
				if err != nil || old.Fullname != "Fixture user" || len(old.RoleCodes) != 2 || next.Fullname != "Committed later" || len(next.RoleCodes) != 1 {
					t.Fatal("Me mixed snapshots or retained disabled role", err)
				}
			} else {
				old := got.res.(*userv1.ListUsersResponse)
				if kind == "empty List permission" {
					if len(old.Items) != 0 {
						t.Fatal("cursor empty page changed")
					}
				} else if len(old.Items) != 1 || old.Items[0].Id != 1 || old.Items[0].Name != "Fixture user" || old.NextCursor != "1" {
					t.Fatal("page mixed snapshots or lost distinct/filter/cursor mapping")
				}
				if next, err := fresh.ListUsers(ctx, req); next != nil || !errors.Is(err, common.ErrForbidden) {
					t.Fatal("next snapshot reused stale permission", err)
				}
			}
			var stable bool
			if err = conn.QueryRowContext(ctx, `SELECT a.auth_version=1 AND s.status='ACTIVE' FROM auth a JOIN sessions s ON s.user_id=a.id WHERE a.id=1`).Scan(&stable); err != nil || !stable {
				t.Fatal("privilege invalidation changed session/auth_version")
			}
			t.Logf("RR snapshot backend=%d, concurrent commit backend=%d; %s current snapshot consistent, next request observes commit", barrier.pid, pid, kind)
			if err = conn.Close(); err != nil || fixture.DB.Stats().InUse != 0 {
				t.Fatal("snapshot leaked pool slot")
			}
		})
	}
}

func postgresFoundationCommands(t *testing.T) {
	for _, command := range []string{"Register", "Profile"} {
		for _, stage := range []string{"success", "mutation", "audit_logs", "outbox_events", "commit"} {
			t.Run(command+"/"+stage, func(t *testing.T) {
				ctx, fixture := loginFixture(t, false)
				method := userv1.UserService_Register_FullMethodName
				if command == "Profile" {
					method = profilev1.ProfileService_UpdateProfile_FullMethodName
					ctx = foundationActor(t, ctx, fixture, method)
				} else {
					ctx = loginCaller(t, ctx, method)
				}
				table := stage
				if stage == "mutation" {
					table = "profile"
				}
				if stage == "commit" {
					table = "outbox_events"
				}
				if stage != "success" {
					trigger := "CREATE TRIGGER reject_command AFTER INSERT OR UPDATE ON " + table + " FOR EACH ROW EXECUTE FUNCTION reject_command()"
					if stage == "commit" {
						trigger = "CREATE CONSTRAINT TRIGGER reject_command AFTER INSERT ON outbox_events DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_command()"
					}
					if _, err := fixture.DB.ExecContext(ctx, `CREATE FUNCTION reject_command() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled command write failure' USING ERRCODE='23514'; END $$; `+trigger); err != nil {
						t.Fatal("command failure fixture failed")
					}
				}
				if command == "Register" {
					s, err := authfeature.NewPendingRegistrationService(pendingRegistrationDeps(fixture))
					if err != nil {
						t.Fatal(err)
					}
					h, _ := handler.NewPendingRegistrationHandler(s)
					res, err := h.Register(ctx, &userv1.AuthRequest{Email: " NEW@example.test ", Password: "secret12"})
					want := 0
					if stage == "success" {
						want = 1
						if err != nil || res == nil || res.Token != "" || res.RefreshToken != "" || res.SessionId != "" || res.RegistrationStatus != "PENDING" || res.RegistrationId != 2 {
							t.Fatal("pending registration response", res, err)
						}
					} else if res != nil || !errors.Is(err, common.ErrDependencyUnavailable) {
						t.Fatal("failed Register returned success", err)
					}
					assertRegistrationCounts(t, ctx, fixture, "new@example.test", want)
					var audit, event, credentials int
					var unsafe int
					err = fixture.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE action='USER_REGISTERED'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.user.created.v1' AND published_at IS NULL AND aggregate_version=1 AND payload->'data'->>'status'='PENDING' AND NOT payload ? 'actor_user_id'),(SELECT count(*) FROM sessions)+(SELECT count(*) FROM refresh_tokens),(SELECT count(*) FROM auth WHERE email='new@example.test' AND (status<>'PENDING' OR version<>1 OR auth_version<>1))`).Scan(&audit, &event, &credentials, &unsafe)
					if err != nil || audit != want || event != want || credentials != 0 || unsafe != 0 {
						t.Fatal("pending Register atomic counts", audit, event, credentials, unsafe, err)
					}
				} else {
					s, err := profile.NewSessionProfileService(foundationProfileDeps(fixture))
					if err != nil {
						t.Fatal(err)
					}
					h, _ := handler.NewSessionProfileHandler(s)
					res, err := h.UpdateProfile(ctx, &profilev1.ProfileRequest{Name: "Changed"})
					want := 0
					version := 1
					expectedName := "Fixture user"
					if stage == "success" {
						want = 1
						version = 2
						expectedName = "Changed"
						if err != nil || res == nil || !res.Changed || res.Version != 2 {
							t.Fatal("profile committed response", res, err)
						}
					} else if res != nil || !errors.Is(err, common.ErrDependencyUnavailable) {
						t.Fatal("failed Profile returned success", err)
					}
					assertFoundationProfile(t, ctx, fixture, expectedName, version, want, 0)
					if stage == "success" {
						res, err = h.UpdateProfile(ctx, &profilev1.ProfileRequest{Name: "Changed"})
						if err != nil || res.Changed || res.Version != 2 {
							t.Fatal("identical profile was not no-op", err)
						}
						assertFoundationProfile(t, ctx, fixture, "Changed", 2, 1, 1)
					}
				}
				if fixture.DB.Stats().InUse != 0 {
					t.Fatal("command leaked transaction pool slot")
				}
			})
		}
	}
}

func assertFoundationProfile(t *testing.T, ctx context.Context, fixture *fixtures.Schema, name string, version, changes, noops int) {
	t.Helper()
	var gotName string
	var gotVersion, authVersion, audits, events, noop int
	var credentialStable bool
	err := fixture.DB.QueryRowContext(ctx, `SELECT p.name,a.version,a.auth_version,(SELECT count(*) FROM audit_logs WHERE action='USER_PROFILE_UPDATED'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.user.profile_updated.v1' AND published_at IS NULL AND aggregate_version<=a.version AND (payload->>'aggregate_version')::bigint=aggregate_version),(SELECT count(*) FROM audit_logs WHERE action='NO_OP'),(SELECT count(*)=1 AND bool_and(status='ACTIVE') FROM sessions) AND (SELECT count(*)=1 AND bool_and(consumed_at IS NULL AND revoked_at IS NULL) FROM refresh_tokens) FROM profile p JOIN auth a ON a.id=p.user_id WHERE a.id=1`).Scan(&gotName, &gotVersion, &authVersion, &audits, &events, &noop, &credentialStable)
	if err != nil || gotName != name || gotVersion != version || authVersion != 1 || audits != changes || events != changes || noop != noops || !credentialStable {
		t.Fatalf("profile atomic outcome name/version/auth/audit/event/noop/stable=%s/%d/%d/%d/%d/%d/%v, err=%v", gotName, gotVersion, authVersion, audits, events, noop, credentialStable, err)
	}
}

func postgresRegisterMemberPolicy(t *testing.T) {
	for _, kind := range []string{"inactive", "wrong code", "privileged"} {
		t.Run(kind, func(t *testing.T) {
			ctx, fixture := loginFixture(t, false)
			ctx = loginCaller(t, ctx, userv1.UserService_Register_FullMethodName)
			query := `UPDATE role SET status='DISABLED' WHERE id=1`
			if kind == "wrong code" {
				query = `UPDATE role SET code='iam.platform_admin' WHERE id=1`
			}
			if kind == "privileged" {
				query = `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.read','fixture'); INSERT INTO role_permissions(role_id,permission_id) VALUES(1,1)`
			}
			if _, err := fixture.DB.ExecContext(ctx, query); err != nil {
				t.Fatal("member policy fixture failed")
			}
			service, err := authfeature.NewPendingRegistrationService(pendingRegistrationDeps(fixture))
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Register(ctx, domain.AuthRequest{Email: "new@example.test", Password: "secret12"})
			if result != nil || !errors.Is(err, common.ErrDependencyUnavailable) {
				t.Fatal("unsafe default member accepted", err)
			}
			assertRegistrationCounts(t, ctx, fixture, "new@example.test", 0)
			var effects int
			if err := fixture.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM audit_logs)+(SELECT count(*) FROM outbox_events)+(SELECT count(*) FROM sessions)+(SELECT count(*) FROM refresh_tokens)`).Scan(&effects); err != nil || effects != 0 {
				t.Fatal("unsafe registration has effects")
			}
		})
	}
}
