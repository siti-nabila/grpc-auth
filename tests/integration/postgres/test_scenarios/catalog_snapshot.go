//go:build postgres

package test_scenarios

import (
	"errors"
	"testing"
	"time"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/catalog"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func CatalogSnapshots() []testutils.Scenario {
	return []testutils.Scenario{{Name: "four catalog reads keep authority total safe page and detail on one primary snapshot", Run: postgresCatalogSnapshots}}
}

func postgresCatalogSnapshots(t *testing.T) {
	for _, method := range []string{rolev1.RoleService_ListRoles_FullMethodName, rolev1.RoleService_GetRole_FullMethodName, permissionv1.PermissionService_ListPermissions_FullMethodName, applicationv1.ApplicationService_ListApplications_FullMethodName} {
		t.Run(method, func(t *testing.T) {
			ctx, f := loginFixture(t, false)
			response, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
			if err != nil {
				t.Fatal(err)
			}
			claims, err := loginSecurity(t, func() time.Time { return shared.ServiceTime }).Verifier.Verify(response.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			ctx = shared.CallerContext(t, ctx, method, method)
			cfg := database.GetORMConfig()
			cfg.EnableDebug = false
			ctx, err = authfeature.AuthenticateSession(ctx, claims, reader.NewSessionStateReader(orm.NewSqlQueryAdapter(ctx, f.DB, dialect.NewPostgres(), cfg)), shared.ServiceTime)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.DB.ExecContext(ctx, `
INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.role.read','Role read'),(1,'iam.permission.read','Permission read');
INSERT INTO role(role_name,role_code,application_id,code) VALUES('Reader',2,1,'iam.reader');
INSERT INTO role_permissions(role_id,permission_id) VALUES(2,1),(2,2);INSERT INTO user_role(user_id,role_id) VALUES(1,2);
INSERT INTO applications(code,name) VALUES('sales','Sales before');
INSERT INTO role(role_name,role_description,role_code,application_id,code,created_at) VALUES('Before',NULL,3,2,'sales.editor',NULL),('Unassigned','',4,2,'sales.viewer',NULL);
INSERT INTO permissions(application_id,code,description) VALUES(2,'sales.order.read','Before read');INSERT INTO role_permissions(role_id,permission_id) VALUES(3,3);`)
			if err != nil {
				t.Fatal("catalog snapshot fixture", err)
			}
			provider := database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig())
			ready, release := make(chan struct{}), make(chan struct{})
			barrier := &snapshotBarrier{provider: provider, ready: ready, release: release}
			read := func(provider domain.ReadSnapshotProvider) (any, error) {
				switch method {
				case rolev1.RoleService_ListRoles_FullMethodName, rolev1.RoleService_GetRole_FullMethodName:
					s, e := role.NewReadService(role.ReadDependencies{Snapshots: provider, Bind: role.BindReadRepositories, Now: func() time.Time { return shared.ServiceTime }})
					if e != nil {
						return nil, e
					}
					if method == rolev1.RoleService_GetRole_FullMethodName {
						return s.GetRole(ctx, 3)
					}
					return s.ListRoles(ctx, domain.CatalogQuery{})
				default:
					s, e := catalog.NewService(catalog.Dependencies{Snapshots: provider, Bind: catalog.BindRepositories, Now: func() time.Time { return shared.ServiceTime }})
					if e != nil {
						return nil, e
					}
					if method == permissionv1.PermissionService_ListPermissions_FullMethodName {
						return s.ListPermissions(ctx, domain.CatalogQuery{})
					}
					return s.ListApplications(ctx, domain.CatalogQuery{})
				}
			}
			done := make(chan snapshotResult, 1)
			finished, released := false, false
			defer func() {
				if !released {
					close(release)
				}
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("catalog snapshot worker cleanup")
					}
				}
			}()
			go func() { res, err := read(barrier); done <- snapshotResult{res, err} }()
			select {
			case <-ready:
			case result := <-done:
				finished = true
				t.Fatal("read before barrier", result.err)
			case <-ctx.Done():
				t.Fatal("catalog snapshot barrier timeout")
			}
			conn, err := f.DB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, f.Name)
			if pid == barrier.pid || barrier.pid == 0 {
				t.Fatal("snapshot requires two physical backends")
			}
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			_, err = tx.ExecContext(ctx, `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2; UPDATE role SET role_name='After' WHERE id=3; UPDATE applications SET name='Sales after' WHERE id=2; UPDATE permissions SET description='After read' WHERE id=3; INSERT INTO applications(code,name) VALUES('later','Later'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Later',5,2,'sales.later'); INSERT INTO permissions(application_id,code,description) VALUES(2,'sales.order.update','Later')`)
			if err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			close(release)
			released = true
			var result snapshotResult
			select {
			case result = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("catalog snapshot completion timeout")
			}
			if result.err != nil {
				t.Fatal(result.err)
			}
			switch r := result.res.(type) {
			case orm.PageData[domain.RoleSummary]:
				if r.Total != 4 || len(r.Items) != 4 || r.Items[2].Name != "Before" || r.Items[2].Description != nil || r.Items[2].CreatedAt != nil || r.Items[2].AffectedUserCount != 0 {
					t.Fatal("mixed role page snapshot")
				}
			case domain.RoleDetail:
				if r.Role.Name != "Before" || len(r.Permissions) != 1 || r.Permissions[0].Description != "Before read" {
					t.Fatal("mixed role detail snapshot")
				}
			case orm.PageData[domain.PermissionItem]:
				if r.Total != 3 || len(r.Items) != 3 || r.Items[2].Description != "Before read" {
					t.Fatal("mixed permission page snapshot")
				}
			case orm.PageData[domain.ApplicationItem]:
				if r.Total != 2 || len(r.Items) != 2 || r.Items[1].Name != "Sales before" {
					t.Fatal("mixed application page snapshot")
				}
			default:
				t.Fatal("unexpected snapshot result")
			}
			conn.Close()
			if _, err = read(provider); !errors.Is(err, common.ErrForbidden) {
				t.Fatal("next snapshot reused stale permission", err)
			}
			if f.DB.Stats().InUse != 0 {
				t.Fatal("catalog snapshot leaked pool")
			}
			t.Logf("catalog RR backend=%d concurrent policy/catalog backend=%d", barrier.pid, pid)
		})
	}
}
