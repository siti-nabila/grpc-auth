//go:build postgres

package test_scenarios

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func CatalogKeywordPerformance() []testutils.Scenario {
	return []testutils.Scenario{{Name: "synthetic query plans and assignment rename fanout are recorded without forcing scans", Run: catalogKeywordPerformance}}
}

func catalogKeywordPerformance(t *testing.T) {
	ctx, _, f := dedicatedFixture(t)
	start := time.Now()
	installCatalogSQL(t, ctx, f.DB, "01_schema_functions_triggers.sql")
	t.Logf("empty_fixture_schema_install_ms=%.3f", float64(time.Since(start).Microseconds())/1000)
	installCatalogSQL(t, ctx, f.DB, "02_backfill.sql")
	start = time.Now()
	installCatalogSQL(t, ctx, f.DB, "03_indexes.sql")
	t.Logf("empty_fixture_index_install_ms=%.3f", float64(time.Since(start).Microseconds())/1000)
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO role(role_name,role_code,application_id,code) VALUES('Senior Reviewer',2,1,'iam.reviewer');
INSERT INTO role(role_name,role_code,application_id,code) SELECT CASE WHEN n%5=0 THEN 'Senior Reviewer '||n ELSE 'Operator '||n END,n,1,'iam.fixture_'||n FROM generate_series(3,3002) n;
INSERT INTO auth(id,email,password,status) SELECT n,'performance-'||n||'@example.test','synthetic','ACTIVE' FROM generate_series(2,2001) n;
INSERT INTO user_role(user_id,role_id) SELECT id,2 FROM auth WHERE id>1;
SELECT refresh_role_search(1); SELECT refresh_application_search(1); SELECT refresh_user_role_search(1);
ANALYZE role; ANALYZE role_search; ANALYZE user_role; ANALYZE user_role_search;`); err != nil {
		t.Fatal(err)
	}
	catalogKeywordPlan(t, ctx, f.DB, "backfill_batch_100", "SELECT * FROM catalog_search_backfill(0,0,0,0,100)")
	for _, x := range []struct{ label, word string }{{"short", "r"}, {"substring", "view"}, {"multitoken", "sen revi"}, {"no_match", "notpresent"}} {
		predicate := ` FROM role r JOIN role_search rs ON rs.role_id=r.id WHERE (rs.fts_keyword @@ catalog_search_prefix_query($1) OR rs.fts_lexeme_text ILIKE catalog_search_pattern($1) ESCAPE '\')`
		catalogKeywordPlan(t, ctx, f.DB, x.label+"_count", "SELECT COUNT(DISTINCT r.id)"+predicate, x.word)
		catalogKeywordPlan(t, ctx, f.DB, x.label+"_page", "SELECT r.id"+predicate+" ORDER BY r.id LIMIT 21", x.word)
	}
	catalogKeywordPlan(t, ctx, f.DB, "rename_2000_assignments", "UPDATE role SET role_name='Principal Reviewer' WHERE id=2")
	var bytes, updated int64
	if err := f.DB.QueryRowContext(ctx, "SELECT sum(pg_relation_size(indexrelid)) FROM pg_index WHERE indrelid IN ('role_search'::regclass,'permission_search'::regclass,'application_search'::regclass,'user_role_search'::regclass)").Scan(&bytes); err != nil || bytes < 1 {
		t.Fatal("index size", err)
	}
	if err := f.DB.QueryRowContext(ctx, "SELECT count(*) FROM user_role_search WHERE fts_lexeme_text='principal reviewer'").Scan(&updated); err != nil || updated != 2000 {
		t.Fatal("rename fanout", updated, err)
	}
	t.Logf("synthetic workload roles=3002 assignments=2001 total_projection_index_bytes=%d; not a production latency SLO", bytes)
}

func catalogKeywordPlan(t *testing.T, ctx context.Context, db *sql.DB, label, statement string, args ...any) {
	t.Helper()
	var raw []byte
	if err := db.QueryRowContext(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+statement, args...).Scan(&raw); err != nil {
		t.Fatal("query plan", err)
	}
	var plan []struct {
		Execution float64 `json:"Execution Time"`
		Planning  float64 `json:"Planning Time"`
		Plan      struct {
			Node  string `json:"Node Type"`
			Hits  int64  `json:"Shared Hit Blocks"`
			Reads int64  `json:"Shared Read Blocks"`
		} `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil || len(plan) != 1 || plan[0].Plan.Node == "" {
		t.Fatal("plan shape", err)
	}
	p := plan[0]
	t.Logf("%s root=%s execution_ms=%.3f planning_ms=%.3f shared_hits=%d shared_reads=%d", label, p.Plan.Node, p.Execution, p.Planning, p.Plan.Hits, p.Plan.Reads)
}
