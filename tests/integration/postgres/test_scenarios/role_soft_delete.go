//go:build postgres

package test_scenarios

import (
	"database/sql"
	"os"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/orm/dialect"
)

func postgresMappingMigration(t *testing.T) {
	script, err := os.ReadFile("../../../docs/sql/role-permission-soft-delete/01_add_deleted_at.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"expand", "compatible", "wrong type", "not nullable", "default"} {
		t.Run(kind, func(t *testing.T) {
			ctx, _, f := dedicatedFixture(t)
			if _, err := f.DB.ExecContext(ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.read','fixture');INSERT INTO role_permissions(role_id,permission_id) VALUES(1,1)`); err != nil {
				t.Fatal(err)
			}
			setup := map[string]string{"expand": "ALTER TABLE role_permissions DROP COLUMN deleted_at", "wrong type": "ALTER TABLE role_permissions ALTER COLUMN deleted_at TYPE text", "not nullable": "UPDATE role_permissions SET deleted_at='2026-01-01';ALTER TABLE role_permissions ALTER COLUMN deleted_at SET NOT NULL", "default": "ALTER TABLE role_permissions ALTER COLUMN deleted_at SET DEFAULT CURRENT_TIMESTAMP"}[kind]
			if setup != "" {
				if _, err := f.DB.ExecContext(ctx, setup); err != nil {
					t.Fatal(err)
				}
			}
			_, err := f.DB.ExecContext(ctx, string(script))
			if kind != "expand" && kind != "compatible" {
				if err == nil {
					t.Fatal("incompatible schema accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.DB.ExecContext(ctx, string(script)); err != nil {
				t.Fatal("repeat migration", err)
			}
			var active bool
			if err = f.DB.QueryRowContext(ctx, `SELECT count(*)=1 AND bool_and(deleted_at IS NULL) FROM role_permissions WHERE role_id=1 AND permission_id=1`).Scan(&active); err != nil || !active {
				t.Fatal("existing pair changed", err)
			}
			if _, err = f.DB.ExecContext(ctx, `INSERT INTO role_permissions(role_id,permission_id) VALUES(1,1)`); err == nil {
				t.Fatal("pair uniqueness lost")
			}
		})
	}
}

func postgresMappingHistory(t *testing.T) {
	h := newRoleHarness(t)
	client := permissionClient(t, h)
	before := roleCredentialState(t, h)
	firstContext := h.actor("first-diff")
	at := h.now()
	// A/B -> B/C -> A/B. C starts absent; B must never be rewritten.
	if _, err := h.f.DB.ExecContext(h.ctx, `INSERT INTO permissions(application_id,code,description) VALUES(2,'sales.order.delete','fixture')`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.roles.ReplaceRolePermissions(firstContext, roleReplace(7, "sales.order.update", "sales.order.delete")); err != nil {
		t.Fatal(err)
	}
	var deleted sql.NullTime
	if err := h.f.DB.QueryRowContext(h.ctx, `SELECT deleted_at FROM role_permissions rp JOIN permissions p ON p.id=rp.permission_id WHERE rp.role_id=3 AND p.code='sales.order.read'`).Scan(&deleted); err != nil || !deleted.Valid || !deleted.Time.Equal(at) {
		t.Fatal("missing command timestamp", deleted, err)
	}
	view, err := client.GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: "sales"})
	if err != nil || len(permissionSources(view)["sales.order.read"]) != 1 || permissionSources(view)["sales.order.read"][0] != 4 {
		t.Fatal(view, err)
	}
	if _, err = h.roles.ReplaceRolePermissions(h.actor("restore"), roleReplace(8, "sales.order.read", "sales.order.update")); err != nil {
		t.Fatal(err)
	}
	var pairs, active int
	if err = h.f.DB.QueryRowContext(h.ctx, `SELECT count(*),count(*) FILTER(WHERE deleted_at IS NULL) FROM role_permissions WHERE role_id=3`).Scan(&pairs, &active); err != nil || pairs != 3 || active != 2 {
		t.Fatal(pairs, active, err)
	}
	if before != roleCredentialState(t, h) {
		t.Fatal("mapping diff changed credentials/assignments")
	}
	if _, err = h.roles.ReplaceRolePermissions(h.actor("clear"), roleReplace(9)); err != nil {
		t.Fatal(err)
	}
	state := roleBusinessState(t, h)
	if _, err = h.roles.ReplaceRolePermissions(h.actor("clear"), roleReplace(9)); err != nil || state != roleBusinessState(t, h) {
		t.Fatal("replay changed history", err)
	}
	var mappingBefore string
	if err = h.f.DB.QueryRowContext(h.ctx, `SELECT jsonb_agg(to_jsonb(rp) ORDER BY permission_id)::text FROM role_permissions rp WHERE role_id=3`).Scan(&mappingBefore); err != nil {
		t.Fatal(err)
	}
	if _, err = h.roles.ReplaceRolePermissions(h.actor("noop"), roleReplace(10)); err != nil {
		t.Fatal(err)
	}
	if err = h.f.DB.QueryRowContext(h.ctx, `SELECT count(*),count(*) FILTER(WHERE deleted_at IS NULL) FROM role_permissions WHERE role_id=3`).Scan(&pairs, &active); err != nil || pairs != 3 || active != 0 {
		t.Fatal(pairs, active, err)
	}
	var mappingAfter string
	if err = h.f.DB.QueryRowContext(h.ctx, `SELECT jsonb_agg(to_jsonb(rp) ORDER BY permission_id)::text FROM role_permissions rp WHERE role_id=3`).Scan(&mappingAfter); err != nil || mappingAfter != mappingBefore {
		t.Fatal("no-op overwrote deletion history", err)
	}
	if _, err = h.f.DB.ExecContext(h.ctx, `UPDATE role_permissions SET deleted_at='2026-01-01' WHERE role_id=4`); err != nil {
		t.Fatal(err)
	}
	suspended := "SUSPENDED"
	if _, err = h.users.ChangeUserStatus(h.actor("suspend-target"), &userv1.ChangeUserStatusRequest{UserId: 2, ExpectedUserVersion: managementVersion(1), Status: &suspended, Reason: "Controlled review"}); err != nil {
		t.Fatal(err)
	}
	view, err = client.GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: "sales"})
	if err != nil || view.UserStatus != "SUSPENDED" || len(view.Permissions) != 0 {
		t.Fatal("inactive view included deleted mappings", view, err)
	}

}

func postgresDeletedDefaultMember(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.read','fixture');INSERT INTO role_permissions(role_id,permission_id,deleted_at) VALUES(1,1,'2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	tx, err := database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if id, err := reader.NewRegistrationRoleReader(tx).DefaultMember(); err != nil || id != 1 {
		t.Fatal(id, err)
	}
	if err = writer.NewRoleManagementWriter(tx).ReplacePermissions(ctx, 1, domain.RolePermissionChanges{Restored: []uint64{1}}, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if id, err := reader.NewRegistrationRoleReader(tx).DefaultMember(); err == nil && id != 0 {
		t.Fatal("privileged member accepted", id)
	}
}

func prepareRoleMappingDiff(t *testing.T, h *roleHarness) {
	t.Helper()
	if _, err := h.f.DB.ExecContext(h.ctx, `INSERT INTO permissions(application_id,code,description) VALUES(2,'sales.order.create','fixture');UPDATE role_permissions SET deleted_at='2026-01-01' WHERE role_id=3 AND permission_id=(SELECT id FROM permissions WHERE code='sales.order.read')`); err != nil {
		t.Fatal(err)
	}
}
