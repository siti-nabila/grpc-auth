//go:build postgres

package test_scenarios

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func CatalogKeyword() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "reviewed migration helpers preserve vector positions and literal input", Run: catalogKeywordHelpers},
		{Name: "catalog triggers refresh projections and source rollback stays atomic", Run: catalogKeywordTriggers},
	}
}

func catalogKeywordFixture(t *testing.T) (context.Context, *fixtures.Schema) {
	t.Helper()
	ctx, _, f := dedicatedFixture(t)
	installCatalogSQL(t, ctx, f.DB, "01_schema_functions_triggers.sql")
	return ctx, f
}

func installCatalogSQL(t *testing.T, ctx context.Context, db *sql.DB, file string) {
	t.Helper()
	b, err := os.ReadFile("../../../docs/sql/catalog-keyword-search/" + file)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	// Execute the reviewed migration in one fixture-owned transaction. Removing
	// only its outer delimiters preserves PL/pgSQL BEGIN/END blocks verbatim.
	s = strings.Replace(s, "\nBEGIN;\n", "\n", 1)
	readOnly := strings.Contains(s, "\nBEGIN READ ONLY;\n")
	s = strings.Replace(s, "\nBEGIN READ ONLY;\n", "\n", 1)
	s = strings.TrimSuffix(s, "COMMIT;\n")
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: readOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, s); err != nil {
		t.Fatalf("catalog migration %s: %v", file, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func catalogKeywordHelpers(t *testing.T) {
	ctx, f := catalogKeywordFixture(t)
	for _, x := range []struct{ raw, vector string }{
		{"iam.role.create", "'create':3 'iam':1 'role':2"},
		{"iam auditor", "'auditor':2 'iam':1"},
		{"application-clm", "'application':1 'clm':2"},
		{"iam.-role  create", "'create':3 'iam':1 'role':2"},
	} {
		var got string
		if err := f.DB.QueryRowContext(ctx, "SELECT to_tsvector('simple',catalog_search_normalize($1))::text", x.raw).Scan(&got); err != nil || got != x.vector {
			t.Fatalf("vector got %s want %s: %v", got, x.vector, err)
		}
	}
	for _, raw := range []string{"", "  \t", ".-.", "%", "_", `\`, `'`, `"`, "&", "!", "(x)", "x' | !reviewer", "Senior.-Reviewer", "é柊", "a@b.test", "a_b", `x\'y`} {
		var query, prefix string
		var pattern sql.NullString
		if err := f.DB.QueryRowContext(ctx, "SELECT catalog_search_query($1)::text,catalog_search_prefix_query($1)::text,catalog_search_pattern($1)", raw).Scan(&query, &prefix, &pattern); err != nil {
			t.Fatalf("plain query parsing failed: %v", err)
		}
	}
	for _, raw := range []string{"%", "_", `\`, ".-."} {
		var match bool
		if err := f.DB.QueryRowContext(ctx, `SELECT coalesce('reviewer' ILIKE catalog_search_pattern($1) ESCAPE '\',false)`, raw).Scan(&match); err != nil || match {
			t.Fatal("wildcard/empty input expanded", err)
		}
	}
	var full, prefix, text bool
	if err := f.DB.QueryRowContext(ctx, `SELECT to_tsvector('simple','senior reviewer') @@ catalog_search_query('sen revi'), to_tsvector('simple','senior reviewer') @@ catalog_search_prefix_query('sen-revi'), 'reviewer senior' ILIKE catalog_search_pattern('view') ESCAPE '\'`).Scan(&full, &prefix, &text); err != nil || full || !prefix || !text {
		t.Fatal("independent full/prefix/text semantics", err)
	}
}

func catalogKeywordTriggers(t *testing.T) {
	ctx, f := catalogKeywordFixture(t)
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO applications(code,name) VALUES('clm','Claim Management'); INSERT INTO permissions(application_id,code,description) VALUES(2,'clm.document.read','dokumen'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Senior Reviewer',2,2,'clm.editor'); INSERT INTO user_role(user_id,role_id,status,revoked_by,revoked_at,revoke_reason) VALUES(1,2,'REVOKED',1,CURRENT_TIMESTAMP,'Fixture revoke')`); err != nil {
		t.Fatal(err)
	}
	assertCatalogText(t, ctx, f.DB, "role_search", "role_id", 2, "editor reviewer senior clm")
	var text string
	if err := f.DB.QueryRowContext(ctx, "SELECT fts_lexeme_text FROM permission_search WHERE permission_id=1").Scan(&text); err != nil || strings.Contains(text, "dokumen") {
		t.Fatal("permission formula too broad", err)
	}
	if err := f.DB.QueryRowContext(ctx, "SELECT fts_lexeme_text FROM user_role_search WHERE user_role_id=2").Scan(&text); err != nil || strings.Contains(text, "editor") {
		t.Fatal("assignment code entered formula", err)
	}
	var versionBefore, versionAfter string
	if err := f.DB.QueryRowContext(ctx, "SELECT xmin::text FROM user_role_search WHERE user_role_id=2").Scan(&versionBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.ExecContext(ctx, "UPDATE user_role SET status='ACTIVE',revoked_at=NULL,revoked_by=NULL,revoke_reason=NULL WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if err := f.DB.QueryRowContext(ctx, "SELECT xmin::text FROM user_role_search WHERE user_role_id=2").Scan(&versionAfter); err != nil || versionBefore != versionAfter {
		t.Fatal("same-role regrant rebuilt search", err)
	}
	if _, err := f.DB.ExecContext(ctx, "UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Fixture revoke' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if err := f.DB.QueryRowContext(ctx, "SELECT xmin::text FROM user_role_search WHERE user_role_id=2").Scan(&versionAfter); err != nil || versionBefore != versionAfter {
		t.Fatal("revoke rebuilt search", err)
	}
	if _, err := f.DB.ExecContext(ctx, "UPDATE permissions SET code='clm.document.update' WHERE id=1; UPDATE applications SET code='claims',name='Claim Platform' WHERE id=2; UPDATE role SET code='claims.editor' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	assertCatalogText(t, ctx, f.DB, "permission_search", "permission_id", 1, "clm document update")
	assertCatalogText(t, ctx, f.DB, "application_search", "application_id", 2, "claims claim platform")
	if _, err := f.DB.ExecContext(ctx, "UPDATE role SET role_name='Principal Reviewer' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if err := f.DB.QueryRowContext(ctx, "SELECT fts_lexeme_text FROM user_role_search WHERE user_role_id=2").Scan(&text); err != nil || text != "principal reviewer" {
		t.Fatal("revoked assignment stale", text, err)
	}
	tx, err := f.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE role SET role_name='Temporary' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = f.DB.QueryRowContext(ctx, "SELECT fts_lexeme_text FROM user_role_search WHERE user_role_id=2").Scan(&text); err != nil || text != "principal reviewer" {
		t.Fatal("rollback failed", err)
	}
	if _, err = f.DB.ExecContext(ctx, `CREATE FUNCTION fail_search() RETURNS TRIGGER LANGUAGE PLPGSQL AS $$ BEGIN RAISE EXCEPTION 'controlled projection failure'; END; $$; CREATE TRIGGER fail_search BEFORE UPDATE ON role_search FOR EACH ROW EXECUTE FUNCTION fail_search()`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.DB.ExecContext(ctx, "UPDATE role SET role_name='Should Roll Back' WHERE id=2"); err == nil {
		t.Fatal("projection failure did not abort source")
	}
	if err = f.DB.QueryRowContext(ctx, "SELECT role_name FROM role WHERE id=2").Scan(&text); err != nil || text != "Principal Reviewer" {
		t.Fatal("source survived failed trigger", err)
	}
	if _, err = f.DB.ExecContext(ctx, "UPDATE user_role SET role_id=1 WHERE id=2"); err == nil {
		t.Fatal("expected existing pair uniqueness rejection")
	}
	if _, err = f.DB.ExecContext(ctx, "DELETE FROM user_role WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = f.DB.QueryRowContext(ctx, "SELECT count(*) FROM user_role_search WHERE user_role_id=2").Scan(&n); err != nil || n != 0 {
		t.Fatal("projection cascade failed", err)
	}
}

func assertCatalogText(t *testing.T, ctx context.Context, db *sql.DB, table, key string, id int, want string) {
	t.Helper()
	var vector, text string
	// Identifiers are fixed test cases, never caller data.
	if err := db.QueryRowContext(ctx, "SELECT array_to_string(tsvector_to_array(fts_keyword),' '),fts_lexeme_text FROM "+table+" WHERE "+key+"=$1", id).Scan(&vector, &text); err != nil || vector != text {
		t.Fatal("lexeme text diverged", err)
	}
	for _, token := range strings.Fields(want) {
		if !strings.Contains(text, token) {
			t.Fatal("source token missing", token)
		}
	}
}
