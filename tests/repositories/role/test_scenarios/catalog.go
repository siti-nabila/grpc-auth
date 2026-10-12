package test_scenarios

import (
	"context"
	"database/sql/driver"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
)

func CatalogSQL() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "catalog filters search total projection and hidden cursor values share handle", Run: catalogSQL},
		{Name: "null and mixed direction cursor predicates bind all arguments", Run: catalogNullSQL},
	}
}

func catalogSQL(t *testing.T) {
	q := domain.CatalogQuery{Limit: 1, Fields: []string{"name"}, ApplicationCode: "sales", Statuses: []string{"DISABLED", "ACTIVE"}, Search: &orm.SearchQuery{Keyword: `%_\`, Fields: []string{"name"}}, Sort: []orm.SortField{{Field: "description", Desc: true}}}
	args := []driver.Value{"sales", "ACTIVE", "DISABLED", `%\%\_\\%`}
	runRoleSQL(t, []mocks.SQLStep{
		{Op: "query", SQL: "COUNT(DISTINCT r.id) AS total", Args: args, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(2)}}},
		{Op: "query", SQL: `ORDER BY r.role_description COLLATE "C" DESC NULLS LAST, r.id ASC NULLS LAST LIMIT 2 OFFSET 0`, Args: args, Columns: roleColumns, Rows: [][]driver.Value{roleRow(), {int64(8), "sales.viewer", "Viewer", nil, int64(2), "sales", "ACTIVE", false, int64(1), int64(82), nil, nil, int64(0)}}},
	}, func(ctx context.Context, tx domain.Transaction) {
		page, err := reader.NewCatalogReader(tx).Roles(ctx, q)
		if err != nil || page.Total != 2 || !page.HasNext || page.HasPrev || len(page.Items) != 1 || page.Items[0].ID != 7 || page.Items[0].Name != "Editor" || page.Items[0].Code != "" || page.Items[0].Version != 0 || page.Items[0].AffectedUserCount != 0 {
			t.Fatalf("projected page %+v %v", page, err)
		}
		normalized, _ := domain.RoleCatalog.Normalize(q)
		normalized.LastID = page.NextCursor
		values, err := domain.RoleCatalog.ParseCursor(normalized)
		if err != nil || len(values) != 2 || values[0].Value != nil || *values[1].Value != "7" {
			t.Fatal("hidden cursor lost", err)
		}
	})
}

func catalogNullSQL(t *testing.T) {
	q, _ := domain.RoleCatalog.Normalize(domain.CatalogQuery{Limit: 1, Sort: []orm.SortField{{Field: "description", Desc: true}, {Field: "is_system", Desc: true}}})
	text := "equal"
	boolean := "false"
	id := "7"
	values := []domain.CatalogValue{{Kind: "text", Value: &text}, {Kind: "bool", Value: &boolean}, {Kind: "integer", Value: &id}}
	q.LastID = domain.RoleCatalog.NextCursor(q, values)
	runRoleSQL(t, []mocks.SQLStep{
		{Op: "query", SQL: "COUNT(DISTINCT r.id) AS total", Columns: []string{"total"}, Rows: [][]driver.Value{{int64(2)}}},
		{Op: "query", SQL: `WHERE (((r.role_description COLLATE "C" < $1 OR r.role_description COLLATE "C" IS NULL)) OR (r.role_description COLLATE "C" = $2 AND r.is_system < $3) OR (r.role_description COLLATE "C" = $4 AND r.is_system = $5 AND r.id > $6))`, Args: []driver.Value{"equal", "equal", false, "equal", false, int64(7)}, Columns: roleColumns},
	}, func(ctx context.Context, tx domain.Transaction) {
		page, err := reader.NewCatalogReader(tx).Roles(ctx, q)
		if err != nil || page.Total != 2 || !page.HasPrev || page.NextCursor != "" || len(page.Items) != 0 {
			t.Fatalf("empty cursor page %+v %v", page, err)
		}
	})
	values[0].Value = nil
	q.LastID = domain.RoleCatalog.NextCursor(q, values)
	runRoleSQL(t, []mocks.SQLStep{
		{Op: "query", SQL: "COUNT(DISTINCT r.id) AS total", Columns: []string{"total"}, Rows: [][]driver.Value{{int64(2)}}},
		{Op: "query", SQL: `WHERE ((r.role_description COLLATE "C" IS NULL AND r.is_system < $1) OR (r.role_description COLLATE "C" IS NULL AND r.is_system = $2 AND r.id > $3))`, Args: []driver.Value{false, false, int64(7)}, Columns: roleColumns},
	}, func(ctx context.Context, tx domain.Transaction) {
		if _, err := reader.NewCatalogReader(tx).Roles(ctx, q); err != nil {
			t.Fatal(err)
		}
	})
}
