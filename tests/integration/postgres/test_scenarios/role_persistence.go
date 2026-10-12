//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func RolePersistence() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "real role sequence conditional version mapping replacement and rollback", Run: postgresRoleWrites},
		{Name: "all catalog scalar directions mixed null keysets counts and literal filters", Run: postgresCatalogQueries},
	}
}

func postgresRoleWrites(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO applications(code,name) VALUES('sales','Sales'); INSERT INTO permissions(application_id,code,description) VALUES(2,'sales.order.read','Read'),(2,'sales.order.update','Update')`); err != nil {
		t.Fatal(err)
	}
	provider := database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig())
	tx, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	w := writer.NewRoleManagementWriter(tx)
	role, err := w.CreateRole(ctx, domain.RoleSummary{Name: "Editor", Code: "sales.editor", ApplicationID: 2, ApplicationCode: "sales", Status: types.RoleActive, Version: 1})
	if err != nil || role.ID != 2 || role.NumericCode != 2 || role.CreatedAt == nil || role.Description != nil || role.Version != 1 {
		t.Fatalf("sequence create %+v %v", role, err)
	}
	if err = w.ReplacePermissions(ctx, role.ID, []uint64{2, 1}); err != nil {
		t.Fatal(err)
	}
	permissions, err := reader.NewRoleManagementReader(tx).RolePermissions(ctx, role.ID)
	if err != nil || len(permissions) != 2 {
		t.Fatalf("mappings %+v %v", permissions, err)
	}
	role.Version = 2
	role.Name = "Edited"
	at := time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)
	role.UpdatedAt = &at
	if ok, err := w.UpdateRole(ctx, role, 1); err != nil || !ok {
		t.Fatalf("conditional update %v %v", ok, err)
	}
	if ok, err := w.UpdateRole(ctx, role, 1); err != nil || ok {
		t.Fatalf("stale update %v %v", ok, err)
	}
	if err = w.ReplacePermissions(ctx, role.ID, nil); err != nil {
		t.Fatal(err)
	}
	permissions, err = reader.NewRoleManagementReader(tx).RolePermissions(ctx, role.ID)
	if err != nil || len(permissions) != 0 {
		t.Fatal("clear", err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, err = provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	w = writer.NewRoleManagementWriter(tx)
	_, err = w.CreateRole(ctx, domain.RoleSummary{Name: "Duplicate", Code: "sales.editor", ApplicationID: 2, ApplicationCode: "sales", Status: types.RoleActive, Version: 1})
	if !errors.Is(err, roleerrors.ErrRoleCodeAlreadyExists) {
		t.Fatal("unique semantic", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	tx, err = provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	w = writer.NewRoleManagementWriter(tx)
	created, err := w.CreateRole(ctx, domain.RoleSummary{Name: "Rollback", Code: "sales.rollback", ApplicationID: 2, ApplicationCode: "sales", Status: types.RoleActive, Version: 1})
	if err != nil || created.NumericCode <= role.NumericCode {
		t.Fatal("sequence after gap", err)
	}
	if err = w.ReplacePermissions(ctx, created.ID, []uint64{1, 2147483647}); err == nil {
		t.Fatal("foreign key accepted")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var roots, mappings int
	if err = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM role WHERE code='sales.rollback'),(SELECT count(*) FROM role_permissions WHERE role_id=$1)`, created.ID).Scan(&roots, &mappings); err != nil || roots != 0 || mappings != 0 {
		t.Fatal("tentative writes survived rollback")
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("role persistence pool leak")
	}
}

type catalogTestRow interface {
	CatalogValue(string) domain.CatalogValue
}

func compareCatalogValue(a, b domain.CatalogValue) int {
	if a.Value == nil {
		if b.Value == nil {
			return 0
		}
		return 1
	}
	if b.Value == nil {
		return -1
	}
	if a.Kind == "integer" {
		x, _ := strconv.ParseInt(*a.Value, 10, 64)
		y, _ := strconv.ParseInt(*b.Value, 10, 64)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		return 0
	}
	return strings.Compare(*a.Value, *b.Value)
}

func checkCatalogSorts[T catalogTestRow](t *testing.T, ctx context.Context, c domain.Catalog, read func(context.Context, domain.CatalogQuery) (orm.PageData[T], error)) {
	t.Helper()
	all, err := read(ctx, domain.CatalogQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	sorts := [][]orm.SortField{}
	for _, field := range c.Fields() {
		for _, desc := range []bool{false, true} {
			sorts = append(sorts, []orm.SortField{{Field: field.Name, Desc: desc}})
		}
	}
	if c == domain.RoleCatalog {
		sorts = append(sorts, []orm.SortField{{Field: "description", Desc: true}, {Field: "status"}, {Field: "name", Desc: true}}, []orm.SortField{{Field: "updated_at"}, {Field: "is_system", Desc: true}, {Field: "id", Desc: true}})
	}
	for _, s := range sorts {
		q, _ := c.Normalize(domain.CatalogQuery{Limit: 2, Sort: s, Fields: []string{"id"}})
		expected := append([]T(nil), all.Items...)
		sort.Slice(expected, func(i, j int) bool {
			for _, s := range q.Sort {
				a, b := expected[i].CatalogValue(s.Field), expected[j].CatalogValue(s.Field)
				n := compareCatalogValue(a, b)
				if n == 0 {
					continue
				}
				if a.Value == nil || b.Value == nil {
					return n < 0
				}
				if s.Desc {
					return n > 0
				}
				return n < 0
			}
			return false
		})
		ids := []string{}
		for batch := 0; batch < 10; batch++ {
			page, err := read(ctx, q)
			if err != nil {
				t.Fatalf("%s %v: %v", c, s, err)
			}
			if page.Total != all.Total || page.TotalPages != int((all.Total+1)/2) {
				t.Fatal("unique pre-cursor total")
			}
			for _, row := range page.Items {
				ids = append(ids, *row.CatalogValue("id").Value)
			}
			if !page.HasNext {
				if page.NextCursor != "" {
					t.Fatal("terminal cursor")
				}
				break
			}
			if page.NextCursor == "" {
				t.Fatal("missing continuation")
			}
			q.LastID = page.NextCursor
			q.Page++
		}
		want := []string{}
		for _, row := range expected {
			want = append(want, *row.CatalogValue("id").Value)
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("%s %v got %v want %v", c, s, ids, want)
		}
	}
}

func postgresCatalogQueries(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	_, err := f.DB.ExecContext(ctx, `
INSERT INTO applications(code,name,status) VALUES('sales','Sales','DISABLED'),('other','Other','ACTIVE');
INSERT INTO role(role_name,role_description,application_id,code,status,version,created_at,updated_at) VALUES
('Equal','literal %_\ value',2,'sales.editor','DISABLED',2,NULL,NULL),
('Equal','literal %_\ value',2,'sales.viewer','ACTIVE',3,'2026-01-01','2026-01-02'),
('Zulu',NULL,2,'sales.zero','ACTIVE',1,'2026-01-02',NULL),
('Alpha','',3,'other.viewer','ACTIVE',2,'2026-01-01','2026-01-01');
INSERT INTO permissions(application_id,code,description) VALUES(2,'sales.order.read','literal %_\ value'),(2,'sales.order.update','Equal'),(3,'other.order.read','Equal');
INSERT INTO role_permissions VALUES(2,1),(2,2),(3,1);
INSERT INTO auth(email,password,status,deleted_at) VALUES
('pending@catalog.test','fixture','PENDING',NULL),('suspended@catalog.test','fixture','SUSPENDED',NULL),('deactivated@catalog.test','fixture','DEACTIVATED',NULL),('deleted@catalog.test','fixture','ACTIVE',CURRENT_TIMESTAMP),('revoked@catalog.test','fixture','ACTIVE',NULL);
INSERT INTO user_role(user_id,role_id) VALUES(1,2),(2,2),(3,2),(4,2),(5,2);
INSERT INTO user_role(user_id,role_id,status,revoked_at,revoke_reason) VALUES(6,2,'REVOKED',CURRENT_TIMESTAMP,'fixture revoked');
INSERT INTO user_application_access(user_id,application_id,status) VALUES(1,2,'REVOKED');`)
	if err != nil {
		t.Fatal("catalog fixture", err)
	}
	provider := database.NewTransactionProvider(f.DB, dialect.NewPostgres(), database.GetORMConfig())
	tx, err := provider.BeginSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	r := reader.NewCatalogReader(tx)
	checkCatalogSorts(t, ctx, domain.RoleCatalog, r.Roles)
	checkCatalogSorts(t, ctx, domain.PermissionCatalog, r.Permissions)
	checkCatalogSorts(t, ctx, domain.ApplicationCatalog, r.Applications)
	role, err := reader.NewRoleManagementReader(tx).Role(ctx, 2)
	if err != nil || role.AffectedUserCount != 5 {
		t.Fatalf("count includes all existing ACTIVE assignments once %+v %v", role, err)
	}
	for _, x := range []struct {
		q   domain.CatalogQuery
		ids []uint64
	}{
		{domain.CatalogQuery{ApplicationCode: "sales", Statuses: []string{"ACTIVE", "DISABLED"}}, []uint64{2, 3, 4}},
		{domain.CatalogQuery{ApplicationCode: "sales", Statuses: []string{"DISABLED"}, Search: &orm.SearchQuery{Keyword: "equal"}}, []uint64{2}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Fields: []string{"description"}, Keyword: `%_\`}}, []uint64{2, 3}},
		{domain.CatalogQuery{Search: &orm.SearchQuery{Fields: []string{"description"}, Keyword: "literal", Mode: orm.SearchModePrefix}}, []uint64{2, 3}},
		{domain.CatalogQuery{ApplicationCode: "missing"}, []uint64{}},
		{domain.CatalogQuery{Page: 2, Limit: 2}, []uint64{3, 4}},
	} {
		page, err := r.Roles(ctx, x.q)
		if err != nil {
			t.Fatal(err)
		}
		ids := []uint64{}
		for _, row := range page.Items {
			ids = append(ids, row.ID)
		}
		if !reflect.DeepEqual(ids, x.ids) {
			t.Fatalf("filter/search/page got %v want %v", ids, x.ids)
		}
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("catalog pool leak")
	}
}
