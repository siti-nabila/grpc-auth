//go:build postgres

package test_scenarios

import (
	"context"
	"crypto/tls"
	"errors"
	"maps"
	"reflect"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/emptypb"
)

func MeAccess() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered Me-only caller reads safe scoped access without writes or admin permission", Run: postgresMeAccess},
		{Name: "two physical backends preserve Me profile and privilege snapshots", Run: postgresMeSnapshots},
		{Name: "Me cancellation panic driver and finalization failure return no partial response and release pool", Run: postgresMeCleanup},
	}
}

func meSnapshotContext(t *testing.T, ctx context.Context, namespaces ...string) context.Context {
	t.Helper()
	cfg := shared.ServiceConfig()
	cfg.Callers[0].Methods = []string{userv1.UserService_Me_FullMethodName}
	cfg.Callers[0].Namespaces = namespaces
	v, err := serviceauth.NewVerifier(cfg, func() time.Time { return shared.ServiceTime })
	if err != nil {
		t.Fatal(err)
	}
	ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{HandshakeComplete: true}}})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(serviceauth.MetadataKey, shared.ServiceToken(1), "user_id", "999"))
	ctx, err = v.Authenticate(ctx, userv1.UserService_Me_FullMethodName)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func meRoleIDs(res *userv1.UserData) []uint64 {
	ids := make([]uint64, 0, len(res.GetRoles()))
	for _, role := range res.GetRoles() {
		ids = append(ids, role.RoleId)
	}
	return ids
}

func postgresMeAccess(t *testing.T) {
	h := newRoleHarness(t)
	if _, err := h.f.DB.ExecContext(h.ctx, `UPDATE profile SET country_code='+62',phone='0812' WHERE user_id=2;
 INSERT INTO applications(code,name) VALUES('clm','Claims');
 INSERT INTO user_application_access(user_id,application_id) VALUES(2,3);
 INSERT INTO role(role_name,role_code,application_id,code) VALUES('No permissions',6,2,'sales.empty');
 INSERT INTO user_role(user_id,role_id) VALUES(2,6);`); err != nil {
		t.Fatal(err)
	}
	cfg := shared.ServiceConfig()
	cfg.Callers[0].Methods = []string{userv1.UserService_Me_FullMethodName}
	cfg.Callers[0].Namespaces = []string{"sales", "clm"}
	conn, _ := shared.FoundationServerWithService(t, h.f.DB, h.now, shared.LifecycleEntropy(), cfg)
	client := userv1.NewUserServiceClient(conn)
	read := func(t *testing.T) *userv1.UserData {
		t.Helper()
		h.clock.Add(int64(2 * time.Second))
		before := roleBusinessState(t, h)
		ctx := metadata.AppendToOutgoingContext(shared.RPCContext(h.ctx, h.target.Token), "idempotency-key", "same-Me-read")
		res, err := client.Me(ctx, &emptypb.Empty{})
		if err != nil || res.Id != 2 || res.Phone != "+620812" || res.CountryCode != "+62" || res.CreatedAt == nil || res.Version != 1 {
			t.Fatal("registered Me safe fields", res, err)
		}
		if roleBusinessState(t, h) != before {
			t.Fatal("Me changed business credential audit event or replay state")
		}
		return res
	}
	res := read(t)
	if !reflect.DeepEqual(meRoleIDs(res), []uint64{3, 4, 6}) || len(res.ApplicationAccess) != 2 || res.ApplicationAccess[0].ApplicationId != 2 || res.ApplicationAccess[1].ApplicationId != 3 || !maps.Equal(res.Permissions, map[string]bool{"sales.order.read": true, "sales.order.update": true}) {
		t.Fatal("role/application metadata without permission mapping or union lost", res)
	}
	var created time.Time
	if err := h.f.DB.QueryRowContext(h.ctx, `SELECT created_at FROM auth WHERE id=2`).Scan(&created); err != nil || !res.CreatedAt.AsTime().Equal(created) {
		t.Fatal("Me user creation time", err)
	}
	for _, c := range []struct {
		name, change, restore string
		roles                 []uint64
		permissions           map[string]bool
		apps                  int
	}{
		{"mapping soft delete", `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE permission_id=(SELECT id FROM permissions WHERE code='sales.order.read')`, `UPDATE role_permissions SET deleted_at=NULL WHERE permission_id=(SELECT id FROM permissions WHERE code='sales.order.read')`, []uint64{3, 4, 6}, map[string]bool{"sales.order.update": true}, 2},
		{"role disabled", `UPDATE role SET status='DISABLED' WHERE id=3`, `UPDATE role SET status='ACTIVE' WHERE id=3`, []uint64{4, 6}, map[string]bool{"sales.order.read": true}, 2},
		{"assignment revoked", `UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled Me test' WHERE user_id=2 AND role_id=3`, `UPDATE user_role SET status='ACTIVE',revoked_at=NULL,revoke_reason=NULL WHERE user_id=2 AND role_id=3`, []uint64{4, 6}, map[string]bool{"sales.order.read": true}, 2},
		{"business access revoked", `UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled Me test' WHERE user_id=2 AND application_id=2`, `UPDATE user_application_access SET status='ACTIVE',revoked_at=NULL,revoke_reason=NULL WHERE user_id=2 AND application_id=2`, []uint64{}, map[string]bool{}, 1},
		{"application disabled", `UPDATE applications SET status='DISABLED' WHERE id=2`, `UPDATE applications SET status='ACTIVE' WHERE id=2`, []uint64{}, map[string]bool{}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := h.f.DB.ExecContext(h.ctx, c.change); err != nil {
				t.Fatal(err)
			}
			res := read(t)
			if !reflect.DeepEqual(meRoleIDs(res), c.roles) || !maps.Equal(res.Permissions, c.permissions) || len(res.ApplicationAccess) != c.apps {
				t.Fatal("Me source gate ignored", res)
			}
			if _, err := h.f.DB.ExecContext(h.ctx, c.restore); err != nil {
				t.Fatal(err)
			}
		})
	}
	// A different caller sees only its own namespace with the same user token.
	cfg.Callers[0].Namespaces = []string{"clm"}
	scoped, _ := shared.FoundationServerWithService(t, h.f.DB, h.now, shared.LifecycleEntropy(), cfg)
	res, err := userv1.NewUserServiceClient(scoped).Me(shared.RPCContext(h.ctx, h.target.Token), &emptypb.Empty{})
	if err != nil || len(res.Permissions) != 0 || len(res.Roles) != 0 || len(res.ApplicationAccess) != 1 || res.ApplicationAccess[0].Code != "clm" || len(res.RoleCodes) == 0 {
		t.Fatal("caller namespace or legacy metadata", res, err)
	}
	if _, err := h.f.DB.ExecContext(h.ctx, `UPDATE auth SET must_change_password=true WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	res = read(t)
	if !res.MustChangePassword || len(res.RoleCodes) != 0 || len(res.RoleNames) != 0 || len(res.Roles) != 0 || len(res.ApplicationAccess) != 0 || len(res.Permissions) != 0 {
		t.Fatal("restricted Me exposes privilege", res)
	}
}

func postgresMeSnapshots(t *testing.T) {
	for _, kind := range []string{"mapping", "role", "application", "access", "user", "session"} {
		t.Run(kind, func(t *testing.T) {
			h := newRoleHarness(t)
			ctx := meSnapshotContext(t, h.ctx, "sales")
			claims, err := loginSecurity(t, h.now).Verifier.Verify(h.target.Token)
			if err != nil {
				t.Fatal(err)
			}
			provider := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
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
						t.Error("Me worker cleanup")
					}
				}
			}()
			go func() {
				res, err := authfeature.ReadUserDataSnapshot(ctx, claims, barrier, h.now())
				done <- snapshotResult{res, err}
			}()
			select {
			case <-ready:
			case res := <-done:
				finished = true
				t.Fatal(res.err)
			case <-ctx.Done():
				t.Fatal("Me barrier timeout")
			}
			conn, err := h.f.DB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, h.f.Name)
			if pid == barrier.pid || barrier.pid == 0 {
				t.Fatal("Me requires two physical backends")
			}
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			changes := map[string]string{
				"mapping":     `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id IN (3,4)`,
				"role":        `UPDATE role SET status='DISABLED' WHERE id IN (3,4)`,
				"application": `UPDATE applications SET status='DISABLED' WHERE id=2`,
				"access":      `UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled Me test' WHERE user_id=2 AND application_id=2`,
				"user":        `UPDATE auth SET status='SUSPENDED' WHERE id=2`,
				"session":     `UPDATE sessions SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled Me test' WHERE user_id=2`,
			}
			if _, err = tx.ExecContext(ctx, changes[kind]); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.ExecContext(ctx, `UPDATE profile SET name='Committed Me profile' WHERE user_id=2`); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			close(release)
			released = true
			var old snapshotResult
			select {
			case old = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("Me snapshot timeout")
			}
			if old.err != nil {
				t.Fatal(old.err)
			}
			oldView := old.res.(*userv1.UserData)
			if oldView.Fullname == "Committed Me profile" || len(oldView.Permissions) != 2 || len(oldView.Roles) != 2 || len(oldView.ApplicationAccess) != 1 {
				t.Fatal("mixed Me snapshot", oldView)
			}
			next, err := authfeature.ReadUserDataSnapshot(ctx, claims, provider, h.now())
			if kind == "user" || kind == "session" {
				if next != nil || !errors.Is(err, auth.ErrInvalidSession) {
					t.Fatal("stale Me principal", next, err)
				}
			} else if err != nil || next.Fullname != "Committed Me profile" || len(next.Permissions) != 0 {
				t.Fatal("Me next snapshot ignored commit", next, err)
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("Me snapshot lease leaked")
			}
			t.Logf("Me snapshot backend=%d concurrent commit backend=%d; %s", barrier.pid, pid, kind)
		})
	}
}

func postgresMeCleanup(t *testing.T) {
	for _, kind := range []string{"cancel", "deadline", "panic", "driver", "commit"} {
		t.Run(kind, func(t *testing.T) {
			h := newRoleHarness(t)
			ctx, cancel := context.WithCancel(meSnapshotContext(t, h.ctx, "sales"))
			defer cancel()
			claims, err := loginSecurity(t, h.now).Verifier.Verify(h.target.Token)
			if err != nil {
				t.Fatal(err)
			}
			base := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
			var provider domain.ReadSnapshotProvider = base
			want := error(common.ErrDependencyUnavailable)
			before := roleBusinessState(t, h)
			switch kind {
			case "cancel":
				provider = featuremocks.PermissionSnapshot{Provider: base, CancelAt: 7, Cancel: cancel}
				want = context.Canceled
			case "deadline":
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, shared.ServiceTime)
				defer stop()
				want = context.DeadlineExceeded
			case "panic":
				provider = featuremocks.PermissionSnapshot{Provider: base, PanicAt: 7}
				want = common.ErrInternalServerError
			case "driver":
				if _, err := h.f.DB.ExecContext(h.ctx, `ALTER TABLE role_permissions RENAME TO unavailable_me_mappings`); err != nil {
					t.Fatal(err)
				}
			case "commit":
				provider = permissionCommitFailure{base}
			}
			res, err := authfeature.ReadUserDataSnapshot(ctx, claims, provider, h.now())
			if res != nil || !errors.Is(err, want) {
				t.Fatal("Me returned partial response", res, err)
			}
			if kind == "driver" {
				if _, err := h.f.DB.ExecContext(h.ctx, `ALTER TABLE unavailable_me_mappings RENAME TO role_permissions`); err != nil {
					t.Fatal(err)
				}
			}
			if before != roleBusinessState(t, h) {
				t.Fatal("Me failure changed business state")
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("Me cleanup leaked primary lease")
			}
			var one int
			if err := h.f.DB.QueryRowContext(h.ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
				t.Fatal("Me pool not reusable", err)
			}
		})
	}
}
