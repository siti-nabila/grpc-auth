//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func EffectivePermissions() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "two physical backends preserve actor target and contribution snapshots", Run: postgresPermissionSnapshots},
		{Name: "registered E views follow B C D commands without credential invalidation", Run: postgresPermissionCommands},
		{Name: "nonactive targets retain active contributions without restoring revoked sessions", Run: postgresPermissionStatus},
		{Name: "snapshot cancellation panic driver and commit failures release the primary pool", Run: postgresPermissionCleanup},
	}
}

// The extra test server permits only E and the baseline methods, with explicit
// sales visibility. Existing mutation clients retain their original harness.
func permissionClient(t *testing.T, h *roleHarness) userv1.UserServiceClient {
	t.Helper()
	cfg := shared.ServiceConfig()
	cfg.Callers[0].Methods = append(cfg.Callers[0].Methods, userv1.UserService_GetMyEffectivePermissions_FullMethodName, userv1.UserService_GetUserEffectivePermissions_FullMethodName)
	cfg.Callers[0].Namespaces = []string{"iam", "sales", "clm", "unknown"}
	conn, _ := shared.FoundationServerWithService(t, h.f.DB, h.now, shared.LifecycleEntropy(), cfg)
	return userv1.NewUserServiceClient(conn)
}

func permissionSources(res *userv1.GetEffectivePermissionsResponse) map[string][]uint64 {
	m := map[string][]uint64{}
	for _, p := range res.GetPermissions() {
		for _, r := range p.Roles {
			m[p.Code] = append(m[p.Code], r.RoleId)
		}
	}
	return m
}

func postgresPermissionCommands(t *testing.T) {
	h := newApplicationHarness(t)
	client := permissionClient(t, h)
	credentialState := accessCredentialState(t, h)
	assertView := func(read, update []uint64) {
		t.Helper()
		before := managementBusinessState(t, h.managementHarness) + roleBusinessState(t, h)
		res, e := client.GetUserEffectivePermissions(h.actor("read-key"), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: " sales "})
		if e != nil || res.UserStatus != "ACTIVE" {
			t.Fatal(res, e)
		}
		sources := permissionSources(res)
		if !reflect.DeepEqual(sources["sales.order.read"], read) || !reflect.DeepEqual(sources["sales.order.update"], update) {
			t.Fatal(sources, read, update)
		}
		if before != managementBusinessState(t, h.managementHarness)+roleBusinessState(t, h) {
			t.Fatal("read produced business/audit/outbox/replay effects")
		}
		if credentialState != accessCredentialState(t, h) {
			t.Fatal("role/access changes invalidated credentials or unrelated state")
		}
		self, e := client.GetMyEffectivePermissions(shared.RPCContext(h.ctx, h.target.Token), &userv1.GetMyEffectivePermissionsRequest{ApplicationCode: "sales"})
		if e != nil || !reflect.DeepEqual(permissionSources(self), sources) {
			t.Fatal("original JWT self view", self, e)
		}
	}
	assertView([]uint64{3, 4}, []uint64{3})
	if _, e := h.users.RevokeUserRoles(h.actor("revoke-overlap"), assignmentRevoke(2, 1, 4)); e != nil {
		t.Fatal(e)
	}
	assertView([]uint64{3}, []uint64{3})
	if _, e := h.roles.ChangeRoleStatus(h.actor("disable"), roleStatus(7, "DISABLED")); e != nil {
		t.Fatal(e)
	}
	assertView(nil, nil)
	if _, e := h.roles.ChangeRoleStatus(h.actor("enable"), roleStatus(8, "ACTIVE")); e != nil {
		t.Fatal(e)
	}
	assertView([]uint64{3}, []uint64{3})
	if _, e := h.roles.ReplaceRolePermissions(h.actor("replace"), roleReplace(9, "sales.order.read")); e != nil {
		t.Fatal(e)
	}
	assertView([]uint64{3}, nil)
	if _, e := h.applications.RevokeApplicationAccess(h.actor("revoke-app"), applicationRevoke(2)); e != nil {
		t.Fatal(e)
	}
	assertView(nil, nil)
	if _, e := h.applications.GrantApplicationAccess(h.actor("regrant-subset"), applicationGrant(3, 4)); e != nil {
		t.Fatal(e)
	}
	assertView([]uint64{4}, nil)
	for _, app := range []string{"unknown", "clm"} {
		res, e := client.GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: app})
		if e != nil || len(res.Permissions) != 0 {
			t.Fatal("empty namespace", res, e)
		}
	}
	if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE applications SET status='DISABLED' WHERE id=2`); e != nil {
		t.Fatal(e)
	}
	res, e := client.GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: "sales"})
	if e != nil || len(res.Permissions) != 0 {
		t.Fatal("inactive application", res, e)
	}
}

func postgresPermissionStatus(t *testing.T) {
	h := newApplicationHarness(t)
	client := permissionClient(t, h)
	for i, state := range []string{"SUSPENDED", "DEACTIVATED", "ACTIVE"} {
		if _, e := h.users.ChangeUserStatus(h.actor("status-"+state), &userv1.ChangeUserStatusRequest{UserId: 2, Status: &state, ExpectedUserVersion: managementVersion(uint64(i + 1)), Reason: "Controlled account review"}); e != nil {
			t.Fatal(e)
		}
		res, e := client.GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: "sales"})
		if e != nil || res.UserStatus != state || res.Version != uint64(i+2) || !reflect.DeepEqual(permissionSources(res)["sales.order.read"], []uint64{3, 4}) {
			t.Fatal("retained contribution", res, e)
		}
		if _, e := client.GetMyEffectivePermissions(shared.RPCContext(h.ctx, h.target.Token), &userv1.GetMyEffectivePermissionsRequest{}); status.Code(e) != codes.Unauthenticated {
			t.Fatal("old target session restored", e)
		}
	}
	// PENDING has no target session, but an admin can inspect retained role configuration.
	if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE auth SET status='PENDING' WHERE id=2`); e != nil {
		t.Fatal(e)
	}
	res, e := client.GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: "sales"})
	if e != nil || res.UserStatus != "PENDING" || len(permissionSources(res)["sales.order.read"]) != 2 {
		t.Fatal(res, e)
	}
	if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE auth SET deleted_at=CURRENT_TIMESTAMP WHERE id=2`); e != nil {
		t.Fatal(e)
	}
	if _, e := client.GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2}); status.Code(e) != codes.NotFound {
		t.Fatal("deleted target", e)
	}
}

func postgresPermissionSnapshots(t *testing.T) {
	for _, kind := range []string{"self source", "self application", "admin target", "admin authority"} {
		t.Run(kind, func(t *testing.T) {
			h := newApplicationHarness(t)
			if _, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO user_role(user_id,role_id) VALUES(1,3),(1,4);INSERT INTO user_application_access(user_id,application_id) VALUES(1,2)`); e != nil {
				t.Fatal(e)
			}
			self := kind == "self source" || kind == "self application"
			method := userv1.UserService_GetUserEffectivePermissions_FullMethodName
			if self {
				method = userv1.UserService_GetMyEffectivePermissions_FullMethodName
			}
			ctx, cancel := context.WithTimeout(shared.PermissionCallerContext(t, method, "iam", "sales"), 5*time.Second)
			defer cancel()
			claims, e := loginSecurity(t, h.now).Verifier.Verify(h.admin.Token)
			if e != nil {
				t.Fatal(e)
			}
			provider := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
			read := func(p domain.ReadSnapshotProvider) (domain.EffectivePermissionsResult, error) {
				if self {
					return user.ReadMyEffectivePermissionsSnapshot(ctx, claims, domain.MyEffectivePermissionsInput{ApplicationCode: "sales"}, p, h.now())
				}
				return user.ReadUserEffectivePermissionsSnapshot(ctx, claims, domain.UserEffectivePermissionsInput{UserID: 2, ApplicationCode: "sales"}, p, h.now())
			}
			ready, release := make(chan struct{}), make(chan struct{})
			barrier := &snapshotBarrier{provider: provider, ready: ready, release: release}
			done := make(chan snapshotResult, 1)
			released, finished := false, false
			defer func() {
				if !released {
					close(release)
				}
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("snapshot worker cleanup")
					}
				}
			}()
			go func() { r, e := read(barrier); done <- snapshotResult{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal(r.err)
			case <-ctx.Done():
				t.Fatal("snapshot barrier timeout")
			}
			conn, e := h.f.DB.Conn(ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, h.f.Name)
			if pid == barrier.pid || barrier.pid == 0 {
				t.Fatal("two physical backends required")
			}
			tx, e := conn.BeginTx(ctx, nil)
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback()
			statement := `UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled revoke' WHERE user_id=1 AND role_id=4`
			switch kind {
			case "self application":
				statement = `UPDATE applications SET status='DISABLED' WHERE id=2`
			case "admin target":
				statement = `UPDATE auth SET status='SUSPENDED',version=version+1 WHERE id=2;UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled revoke' WHERE user_id=2 AND role_id=4`
			case "admin authority":
				statement = `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.read')`
			}
			if _, e = tx.ExecContext(ctx, statement); e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			_ = conn.Close()
			close(release)
			released = true
			var old snapshotResult
			select {
			case old = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("read timeout")
			}
			if old.err != nil {
				t.Fatal(old.err)
			}
			oldView := old.res.(domain.EffectivePermissionsResult)
			if oldView.UserStatus != "ACTIVE" || len(oldView.Permissions) != 2 || len(oldView.Permissions[0].Roles) != 2 {
				t.Fatal("mixed snapshot", oldView)
			}
			newView, e := read(provider)
			if kind == "admin authority" {
				if !errors.Is(e, common.ErrForbidden) {
					t.Fatal("stale actor authority", e)
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				if kind == "self application" {
					if len(newView.Permissions) != 0 {
						t.Fatal(newView)
					}
				} else {
					if len(newView.Permissions) == 0 || len(newView.Permissions[0].Roles) != 1 || newView.Permissions[0].Roles[0].ID != 3 {
						t.Fatal(newView)
					}
					if kind == "admin target" && (newView.UserStatus != "SUSPENDED" || newView.Version != 2) {
						t.Fatal(newView)
					}
				}
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("snapshot lease leaked")
			}
			t.Logf("E snapshot backend=%d concurrent commit backend=%d", barrier.pid, pid)
		})
	}
}

type permissionCommitFailure struct{ domain.ReadSnapshotProvider }

func (p permissionCommitFailure) BeginSnapshot(ctx context.Context) (domain.Transaction, error) {
	tx, e := p.ReadSnapshotProvider.BeginSnapshot(ctx)
	if e != nil {
		return nil, e
	}
	return permissionFailedCommit{tx}, nil
}

type permissionFailedCommit struct{ domain.Transaction }

func (t permissionFailedCommit) Commit() error {
	return errors.Join(t.Transaction.Commit(), common.ErrDependencyUnavailable)
}

func postgresPermissionCleanup(t *testing.T) {
	for _, kind := range []string{"cancel snapshot", "deadline", "panic", "driver", "commit"} {
		t.Run(kind, func(t *testing.T) {
			h := newApplicationHarness(t)
			method := userv1.UserService_GetMyEffectivePermissions_FullMethodName
			ctx, cancel := context.WithCancel(shared.PermissionCallerContext(t, method, "iam", "sales"))
			defer cancel()
			claims, e := loginSecurity(t, h.now).Verifier.Verify(h.admin.Token)
			if e != nil {
				t.Fatal(e)
			}
			base := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
			var provider domain.ReadSnapshotProvider = base
			want := error(common.ErrDependencyUnavailable)
			switch kind {
			case "deadline":
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, shared.ServiceTime)
				defer stop()
				want = context.DeadlineExceeded
			case "panic":
				provider = featuremocks.PermissionSnapshot{Provider: base, PanicAt: 2}
				want = common.ErrInternalServerError
			case "driver":
				if _, e := h.f.DB.ExecContext(h.ctx, `DROP TABLE role_permissions`); e != nil {
					t.Fatal(e)
				}
			case "commit":
				provider = permissionCommitFailure{base}
			}
			if kind == "cancel snapshot" {
				ready, release := make(chan struct{}), make(chan struct{})
				provider = &snapshotBarrier{provider: base, ready: ready, release: release}
				done := make(chan error, 1)
				go func() {
					_, e := user.ReadMyEffectivePermissionsSnapshot(ctx, claims, domain.MyEffectivePermissionsInput{}, provider, h.now())
					done <- e
				}()
				select {
				case <-ready:
				case e := <-done:
					t.Fatal(e)
				case <-h.ctx.Done():
					t.Fatal("cancel barrier timeout")
				}
				cancel()
				select {
				case e := <-done:
					if !errors.Is(e, context.Canceled) {
						t.Fatal(e)
					}
				case <-h.ctx.Done():
					t.Fatal("cancel cleanup timeout")
				}
			} else {
				res, e := user.ReadMyEffectivePermissionsSnapshot(ctx, claims, domain.MyEffectivePermissionsInput{}, provider, h.now())
				if !errors.Is(e, want) || !reflect.DeepEqual(res, domain.EffectivePermissionsResult{}) {
					t.Fatal(res, e, want)
				}
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("primary lease not returned")
			}
			var one int
			if e := h.f.DB.QueryRowContext(h.ctx, `SELECT 1`).Scan(&one); e != nil || one != 1 {
				t.Fatal("pool not reusable", e)
			}
		})
	}
}
