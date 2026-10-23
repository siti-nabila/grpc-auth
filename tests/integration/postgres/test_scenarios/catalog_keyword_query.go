//go:build postgres

package test_scenarios

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func CatalogKeywordQueries() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "resumable backfill and projection constraints match migration", Run: catalogKeywordBackfill},
		{Name: "GIN indexes use provisioned extension schema", Run: catalogKeywordIndexes},
		{Name: "query branches preserve filters target and dynamic document sources", Run: catalogKeywordQueries},
	}
}

func catalogKeywordBackfill(t *testing.T) {
	ctx, f := catalogKeywordFixture(t)
	installCatalogSQL(t, ctx, f.DB, "02_backfill.sql")
	for _, x := range []struct{ table, source string }{{"role_search", "role"}, {"permission_search", "permissions"}, {"application_search", "applications"}, {"user_role_search", "user_role"}} {
		var columns, primary, foreign int
		err := f.DB.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name=$2 AND is_nullable='NO'),
 (SELECT count(*) FROM pg_constraint WHERE conrelid=($1||'.'||$2)::regclass AND contype='p'),
 (SELECT count(*) FROM pg_constraint WHERE conrelid=($1||'.'||$2)::regclass AND confrelid=($1||'.'||$3)::regclass AND contype='f' AND confdeltype='c')`, f.Name, x.table, x.source).Scan(&columns, &primary, &foreign)
		if err != nil || columns != 3 || primary != 1 || foreign != 1 {
			t.Fatalf("metadata %s %d/%d/%d: %v", x.table, columns, primary, foreign, err)
		}
	}
	last := map[string]int{"role": 0, "permissions": 0, "applications": 0, "user_role": 0}
	for pass := 0; pass < 2; pass++ {
		rows, err := f.DB.QueryContext(ctx, "SELECT * FROM catalog_search_backfill($1,$2,$3,$4,1)", last["role"], last["permissions"], last["applications"], last["user_role"])
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var source string
			var id, count int
			if err = rows.Scan(&source, &id, &count); err != nil {
				t.Fatal(err)
			}
			last[source] = id
			n += count
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		if pass == 0 && n != 3 || pass == 1 && n != 0 {
			t.Fatal("batch continuation", n)
		}
	}
	if _, err := f.DB.ExecContext(ctx, "SELECT * FROM catalog_search_backfill(0,0,0,0,1)"); err != nil {
		t.Fatal("resumable rerun", err)
	}
	for _, q := range []string{
		"INSERT INTO role_search(role_id,fts_keyword,fts_lexeme_text) VALUES(1,''::tsvector,'')",
		"INSERT INTO role_search(role_id,fts_keyword,fts_lexeme_text) VALUES(2147483647,''::tsvector,'')",
		"INSERT INTO permission_search(permission_id,fts_keyword,fts_lexeme_text) VALUES(2147483647,NULL,'')",
		"SELECT * FROM catalog_search_backfill(-1,0,0,0,1)",
	} {
		if _, err := f.DB.ExecContext(ctx, q); err == nil {
			t.Fatal("invalid projection/backfill accepted")
		}
	}
	assertCatalogComplete(t, ctx, f.DB)
}

func catalogKeywordIndexes(t *testing.T) {
	ctx, f := catalogKeywordFixture(t)
	installCatalogSQL(t, ctx, f.DB, "03_indexes.sql")
	var count int
	if err := f.DB.QueryRowContext(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname=$1 AND tablename IN ('role_search','permission_search','application_search','user_role_search') AND indexdef LIKE '%USING gin%'`, f.Name).Scan(&count); err != nil || count != 8 {
		t.Fatal("index metadata", count, err)
	}
	installCatalogSQL(t, ctx, f.DB, "05_rollback_check.sql")
}

type keywordPage struct {
	ids   []uint64
	total int64
	next  string
}

func readKeywordPage(ctx context.Context, db *sql.DB, c domain.Catalog, q domain.CatalogQuery) (keywordPage, error) {
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	h := orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), cfg)
	var out keywordPage
	switch c {
	case domain.RoleCatalog:
		p, e := reader.NewCatalogReader(h).Roles(ctx, q)
		if e != nil {
			return out, e
		}
		out.total, out.next = p.Total, p.NextCursor
		for _, r := range p.Items {
			out.ids = append(out.ids, r.ID)
		}
	case domain.PermissionCatalog:
		p, e := reader.NewCatalogReader(h).Permissions(ctx, q)
		if e != nil {
			return out, e
		}
		out.total, out.next = p.Total, p.NextCursor
		for _, r := range p.Items {
			out.ids = append(out.ids, r.ID)
		}
	case domain.ApplicationCatalog:
		p, e := reader.NewCatalogReader(h).Applications(ctx, q)
		if e != nil {
			return out, e
		}
		out.total, out.next = p.Total, p.NextCursor
		for _, r := range p.Items {
			out.ids = append(out.ids, r.ID)
		}
	case domain.UserRoleAssignmentCatalog:
		p, e := reader.NewUserRoleAssignmentReader(h).ListAssignments(ctx, q)
		if e != nil {
			return out, e
		}
		out.total, out.next = p.Total, p.NextCursor
		for _, r := range p.Items {
			out.ids = append(out.ids, r.ID)
		}
	}
	return out, nil
}

func catalogKeywordQueries(t *testing.T) {
	ctx, f := catalogKeywordFixture(t)
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO auth(email,password,status) VALUES('keyword-fixture@example.test','synthetic','ACTIVE');
INSERT INTO applications(code,name) VALUES('clm','Claim Management'),('other','Other');
INSERT INTO permissions(application_id,code,description) VALUES(2,'clm.document.read','dokumen ekstra');
INSERT INTO role(role_name,role_code,application_id,code) VALUES('Senior Reviewer',2,2,'clm.editor'),('Other Reviewer',3,3,'other.editor');
INSERT INTO user_role(user_id,role_id) VALUES(1,2),(2,3)`); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct {
		c               domain.Catalog
		word, substring string
		want            uint64
	}{{domain.RoleCatalog, "sen revi", "view", 2}, {domain.PermissionCatalog, "document-read", "cumen", 1}, {domain.ApplicationCatalog, "claim-management", "laim", 2}, {domain.UserRoleAssignmentCatalog, "reviewer", "view", 2}} {
		q := domain.CatalogQuery{TargetUserID: 1, Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: x.word, Mode: orm.SearchModeFullTextTrigram}}
		p, err := readKeywordPage(ctx, f.DB, x.c, q)
		if err != nil || p.total != 1 || len(p.ids) != 1 || p.ids[0] != x.want {
			t.Fatalf("query %s %+v %v", x.c, p, err)
		}
		q.Search.Keyword = x.substring
		p, err = readKeywordPage(ctx, f.DB, x.c, q)
		if err != nil || p.total < 1 {
			t.Fatal("text-only substring lost", err)
		}
		for _, literal := range []string{"%", ".-."} {
			q.Search.Keyword = literal
			p, err = readKeywordPage(ctx, f.DB, x.c, q)
			if err != nil || p.total != 0 {
				t.Fatal("wildcard/empty query widened", err)
			}
		}
	}
	q := domain.CatalogQuery{TargetUserID: 1, ApplicationCode: "other", Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: "view", Mode: orm.SearchModeFullTextTrigram}}
	p, err := readKeywordPage(ctx, f.DB, domain.UserRoleAssignmentCatalog, q)
	if err != nil || p.total != 0 {
		t.Fatal("text branch bypassed user/application", err)
	}
	q = domain.CatalogQuery{Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: "reviewer", Mode: orm.SearchModeFullText}, Limit: 1, Sort: []orm.SortField{{Field: "name", Desc: true}}}
	p, err = readKeywordPage(ctx, f.DB, domain.RoleCatalog, q)
	if err != nil || p.total != 2 || len(p.ids) != 1 || p.next == "" {
		t.Fatal("first page", p, err)
	}
	first := p.ids[0]
	q.LastID = p.next
	p, err = readKeywordPage(ctx, f.DB, domain.RoleCatalog, q)
	if err != nil || p.total != 2 || len(p.ids) != 1 || p.ids[0] == first || p.next != "" {
		t.Fatal("keyset page", p, err)
	}
	q.LastID = ""
	q.Page = 2
	p, err = readKeywordPage(ctx, f.DB, domain.RoleCatalog, q)
	if err != nil || len(p.ids) != 1 || p.ids[0] == first {
		t.Fatal("offset page", err)
	}
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	adapter := orm.NewSqlQueryAdapter(ctx, f.DB, dialect.NewPostgres(), cfg)
	checkCatalogSorts(t, ctx, domain.RoleCatalog, func(ctx context.Context, q domain.CatalogQuery) (orm.PageData[domain.RoleSummary], error) {
		q.Search = &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: "reviewer", Mode: orm.SearchModeFullTextTrigram}
		return reader.NewCatalogReader(adapter).Roles(ctx, q)
	})
	permission := domain.CatalogQuery{Search: &orm.SearchQuery{Fields: []string{"keyword"}, Keyword: "dokumen", Mode: orm.SearchModeFullTextTrigram}}
	p, err = readKeywordPage(ctx, f.DB, domain.PermissionCatalog, permission)
	if err != nil || p.total != 0 {
		t.Fatal("initial permission formula", err)
	}
	if _, err = f.DB.ExecContext(ctx, `CREATE OR REPLACE FUNCTION refresh_permission_search(source_id INTEGER) RETURNS VOID LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
DECLARE document TSVECTOR; BEGIN
SELECT to_tsvector('simple',catalog_search_normalize(concat_ws(' ',code,description))) INTO document FROM permissions WHERE id=source_id FOR SHARE;
IF NOT FOUND THEN RETURN; END IF;
INSERT INTO permission_search(permission_id,fts_keyword,fts_lexeme_text) VALUES(source_id,document,array_to_string(tsvector_to_array(document),' '))
ON CONFLICT(permission_id) DO UPDATE SET fts_keyword=EXCLUDED.fts_keyword,fts_lexeme_text=EXCLUDED.fts_lexeme_text; END; $$;
DROP TRIGGER permission_search_update ON permissions;
CREATE TRIGGER permission_search_update AFTER UPDATE OF code,description ON permissions FOR EACH ROW WHEN(OLD.code IS DISTINCT FROM NEW.code OR OLD.description IS DISTINCT FROM NEW.description) EXECUTE FUNCTION trg_refresh_permission_search();
SELECT refresh_permission_search(1)`); err != nil {
		t.Fatal(err)
	}
	p, err = readKeywordPage(ctx, f.DB, domain.PermissionCatalog, permission)
	if err != nil || p.total != 1 {
		t.Fatal("same request/binary lost new source", err)
	}
	for _, word := range []string{"doku eks", "okum"} {
		permission.Search.Keyword = word
		p, err = readKeywordPage(ctx, f.DB, domain.PermissionCatalog, permission)
		if err != nil || p.total != 1 {
			t.Fatal("expanded document independent branches", err)
		}
	}
	if _, err = f.DB.ExecContext(ctx, "DELETE FROM role_search"); err != nil {
		t.Fatal(err)
	}
	p, err = readKeywordPage(ctx, f.DB, domain.RoleCatalog, domain.CatalogQuery{})
	if err != nil || p.total != 3 {
		t.Fatal("unfiltered list requires projection", err)
	}
	p, err = readKeywordPage(ctx, f.DB, domain.RoleCatalog, q)
	if err != nil || p.total != 0 {
		t.Fatal("keyword silently fell back", err)
	}
	p, err = readKeywordPage(ctx, f.DB, domain.RoleCatalog, domain.CatalogQuery{Search: &orm.SearchQuery{Fields: []string{"name"}, Keyword: "view", Mode: orm.SearchModeContains}})
	if err != nil || p.total != 2 {
		t.Fatal("legacy rollback search requires projections", err)
	}
}

func assertCatalogComplete(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	b, err := os.ReadFile("../../../docs/sql/catalog-keyword-search/04_verify.sql")
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(b), "\nBEGIN READ ONLY;\n", "\n", 1)
	s = strings.TrimSuffix(s, "COMMIT;\n")
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	sets := 0
	for {
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(cols))
			targets := make([]any, len(cols))
			for i := range values {
				targets[i] = &values[i]
			}
			if err = rows.Scan(targets...); err != nil {
				t.Fatal(err)
			}
			for _, value := range values[1:] {
				n, ok := value.(int64)
				if !ok || n != 0 {
					t.Fatal("projection verification found discrepancies", cols)
				}
			}
			sets++
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		if !rows.NextResultSet() {
			break
		}
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if sets != 12 {
		t.Fatal("verification result coverage", sets)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
