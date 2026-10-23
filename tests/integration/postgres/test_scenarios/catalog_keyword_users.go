//go:build postgres

package test_scenarios

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func CatalogKeywordUsers() []testutils.Scenario {
	return []testutils.Scenario{{Name: "supplied users formula and wrappers preserve email phone owner and cursor independently of catalog", Run: catalogKeywordUsers}}
}

func catalogKeywordUsers(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	baseline, err := os.ReadFile("fixtures/catalog_users_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.DB.ExecContext(ctx, string(baseline)); err != nil {
		t.Fatal("users baseline fixture", err)
	}
	if _, err = f.DB.ExecContext(ctx, `UPDATE auth SET email='base_line+tag@example.test' WHERE id=1; UPDATE profile SET name='Baseline User',phone='+62 812-3456',address='hiddenaddress',country_code='ID' WHERE id=1; INSERT INTO auth(email,password,status) VALUES('secondowner@example.test','synthetic','ACTIVE')`); err != nil {
		t.Fatal(err)
	}
	words := []string{"Baseline", "base_line+tag@example.test", "example", "base_line", "+62 812-3456", "628123456"}
	for _, word := range words {
		assertUserKeyword(t, ctx, f.DB, word, 1)
	}
	assertUserKeyword(t, ctx, f.DB, "hiddenaddress", 0)
	assertUserKeyword(t, ctx, f.DB, "ID", 0)
	opts := orm.QueryOptions{Page: 1, Limit: 1, Sort: []orm.SortField{{Field: domain.UserSortName, Desc: true}}}
	before, err := readBaselineUsers(ctx, f.DB, opts)
	if err != nil {
		t.Fatal(err)
	}
	installCatalogSQL(t, ctx, f.DB, "01_schema_functions_triggers.sql")
	after, err := readBaselineUsers(ctx, f.DB, opts)
	if err != nil || after.Total != before.Total || after.NextCursor != before.NextCursor || after.HasNext != before.HasNext || after.Items[0].AuthID != before.Items[0].AuthID {
		t.Fatal("catalog changed users pagination", err)
	}
	for _, word := range words {
		assertUserKeyword(t, ctx, f.DB, word, 1)
	}
	if _, err = f.DB.ExecContext(ctx, "UPDATE auth SET email='fresh_owner@example.test' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	assertUserKeyword(t, ctx, f.DB, "fresh_owner", 1)
	assertUserKeyword(t, ctx, f.DB, "base_line", 0)
	if _, err = f.DB.ExecContext(ctx, "UPDATE profile SET name='Changed Person',phone='0812-9999' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	assertUserKeyword(t, ctx, f.DB, "Changed", 1)
	assertUserKeyword(t, ctx, f.DB, "08129999", 1)
	assertUserKeyword(t, ctx, f.DB, "628123456", 0)
	if _, err = f.DB.ExecContext(ctx, "UPDATE profile SET user_id=2 WHERE id=1; UPDATE auth SET email='detached@example.test' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	assertUserKeyword(t, ctx, f.DB, "secondowner", 2)
	assertUserKeyword(t, ctx, f.DB, "fresh_owner", 0)
	assertUserKeyword(t, ctx, f.DB, "detached", 0)
	if _, err = f.DB.ExecContext(ctx, "INSERT INTO profile(user_id,name,phone) VALUES(1,'Inserted Profile','+62 811 222')"); err != nil {
		t.Fatal(err)
	}
	assertUserKeyword(t, ctx, f.DB, "Inserted", 1)
	assertUserKeyword(t, ctx, f.DB, "62811222", 1)
}

func readBaselineUsers(ctx context.Context, db *sql.DB, opts orm.QueryOptions) (orm.PageData[domain.UserSearchRow], error) {
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	h := orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), cfg)
	return reader.NewUserReaderInSnapshot(ctx, h).SearchUsers(opts, domain.UserListFilter{})
}

func assertUserKeyword(t *testing.T, ctx context.Context, db *sql.DB, word string, want int64) {
	t.Helper()
	opts := orm.QueryOptions{Page: 1, Limit: 10, Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: word, Mode: orm.SearchModeFullTextTrigram}}
	p, err := readBaselineUsers(ctx, db, opts)
	if err != nil {
		t.Fatal("users baseline search failed", err)
	}
	if want == 0 {
		if p.Total != 0 || len(p.Items) != 0 {
			t.Fatal("users matched absent source")
		}
		return
	}
	if p.Total != 1 || len(p.Items) != 1 || p.Items[0].AuthID != want {
		t.Fatalf("users baseline result id=%d total=%d count=%d", want, p.Total, len(p.Items))
	}
}
