//go:build postgres

package test_scenarios

import (
	"context"
	"testing"

	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func CatalogKeywordConcurrency() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "two physical backends serialize rename grant reassignment and backfill", Run: catalogKeywordConcurrency},
		{Name: "canceled blocked assignment rolls back and releases pool and projection locks", Run: catalogKeywordCancellation},
	}
}

func catalogKeywordCancellation(t *testing.T) {
	ctx, f := catalogKeywordFixture(t)
	if _, err := f.DB.ExecContext(ctx, `INSERT INTO auth(email,password,status) VALUES('cancel-fixture@example.test','synthetic','ACTIVE'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Reviewer',2,1,'iam.reviewer')`); err != nil {
		t.Fatal(err)
	}
	a, err := f.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := f.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	pidA, pidB := physicalIdentity(t, ctx, a, f.Name), physicalIdentity(t, ctx, b, f.Name)
	if pidA == pidB {
		t.Fatal("requires two physical backends")
	}
	ta, err := a.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ta.Rollback()
	if _, err = ta.ExecContext(ctx, "UPDATE role SET role_name='Principal Reviewer' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	stop, cancel := context.WithCancel(ctx)
	defer cancel()
	tb, err := b.BeginTx(stop, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Rollback()
	done := make(chan error, 1)
	finished := false
	defer func() {
		cancel()
		_ = ta.Rollback()
		if !finished {
			select {
			case <-done:
			case <-ctx.Done():
				t.Error("cancel worker cleanup")
			}
		}
	}()
	go func() {
		_, e := tb.ExecContext(stop, "INSERT INTO user_role(user_id,role_id) VALUES(2,2)")
		_ = tb.Rollback()
		done <- e
	}()
	blocked := false
	for !blocked {
		select {
		case e := <-done:
			finished = true
			t.Fatal("worker did not reach lock barrier", e)
		default:
		}
		if err = ta.QueryRowContext(ctx, "SELECT $1 = ANY(pg_blocking_pids($2))", pidA, pidB).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	select {
	case e := <-done:
		finished = true
		if e == nil {
			t.Fatal("canceled insert succeeded")
		}
	case <-ctx.Done():
		t.Fatal("cancellation cleanup timed out")
	}
	if err = ta.Commit(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = a.QueryRowContext(ctx, "SELECT count(*) FROM user_role WHERE user_id=2").Scan(&count); err != nil || count != 0 {
		t.Fatal("canceled source committed", err)
	}
	a.Close()
	b.Close()
	if _, err = f.DB.ExecContext(ctx, "INSERT INTO user_role(user_id,role_id) VALUES(2,2)"); err != nil {
		t.Fatal("pool or lock not reusable", err)
	}
	var document string
	if err = f.DB.QueryRowContext(ctx, "SELECT fts_lexeme_text FROM user_role_search WHERE user_role_id=(SELECT id FROM user_role WHERE user_id=2)").Scan(&document); err != nil || document != "principal reviewer" {
		t.Fatal("retry projection", err)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("cancellation leaked a connection")
	}
}

func catalogKeywordConcurrency(t *testing.T) {
	for _, x := range []struct{ name, setup, first, second, want string }{
		{"rename then grant", "", "UPDATE role SET role_name='Principal Reviewer' WHERE id=2", "INSERT INTO user_role(user_id,role_id) VALUES(2,2)", "principal reviewer"},
		{"grant then rename", "", "INSERT INTO user_role(user_id,role_id) VALUES(2,2)", "UPDATE role SET role_name='Principal Reviewer' WHERE id=2", "principal reviewer"},
		{"rename then reassignment", "INSERT INTO user_role(user_id,role_id) VALUES(2,2)", "UPDATE role SET role_name='Principal Reviewer' WHERE id=2", "UPDATE user_role SET role_id=3 WHERE id=2", "auditor"},
		{"reassignment then rename", "INSERT INTO user_role(user_id,role_id) VALUES(2,2)", "UPDATE user_role SET role_id=3 WHERE id=2", "UPDATE role SET role_name='Principal Reviewer' WHERE id=2", "auditor"},
		{"backfill then rename", "INSERT INTO user_role(user_id,role_id) VALUES(2,2)", "SELECT * FROM catalog_search_backfill(0,0,0,0,100)", "UPDATE role SET role_name='Principal Reviewer' WHERE id=2", "principal reviewer"},
		{"rename then backfill", "INSERT INTO user_role(user_id,role_id) VALUES(2,2)", "UPDATE role SET role_name='Principal Reviewer' WHERE id=2", "SELECT * FROM catalog_search_backfill(0,0,0,0,100)", "principal reviewer"},
	} {
		t.Run(x.name, func(t *testing.T) {
			ctx, f := catalogKeywordFixture(t)
			installCatalogSQL(t, ctx, f.DB, "02_backfill.sql")
			if _, err := f.DB.ExecContext(ctx, `INSERT INTO auth(email,password,status) VALUES('race-fixture@example.test','synthetic','ACTIVE'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Reviewer',2,1,'iam.reviewer'),('Auditor',3,1,'iam.auditor')`); err != nil {
				t.Fatal(err)
			}
			if x.setup != "" {
				if _, err := f.DB.ExecContext(ctx, x.setup); err != nil {
					t.Fatal(err)
				}
			}
			a, err := f.DB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			b, err := f.DB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			pidA, pidB := physicalIdentity(t, ctx, a, f.Name), physicalIdentity(t, ctx, b, f.Name)
			if pidA == pidB {
				t.Fatal("requires physical backends")
			}
			ta, err := a.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ta.Rollback()
			tb, err := b.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tb.Rollback()
			if _, err = ta.ExecContext(ctx, x.first); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			finished := false
			defer func() {
				_ = ta.Rollback()
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("keyword worker cleanup timed out")
					}
				}
			}()
			go func() {
				_, e := tb.ExecContext(ctx, x.second)
				if e == nil {
					e = tb.Commit()
				} else {
					_ = tb.Rollback()
				}
				done <- e
			}()
			// Observe a real lock dependency before releasing A, rather than
			// assuming scheduler timing or using a sleep as contention proof.
			blocked := false
			for !blocked {
				select {
				case e := <-done:
					finished = true
					t.Fatal("worker completed before lock barrier", e)
				default:
				}
				if err = ta.QueryRowContext(ctx, "SELECT $1 = ANY(pg_blocking_pids($2))", pidA, pidB).Scan(&blocked); err != nil {
					t.Fatal(err)
				}
			}
			if err = ta.Commit(); err != nil {
				t.Fatal(err)
			}
			select {
			case e := <-done:
				finished = true
				if e != nil {
					t.Fatal(e)
				}
			case <-ctx.Done():
				t.Fatal("worker did not complete")
			}
			var document string
			if err = a.QueryRowContext(ctx, "SELECT fts_lexeme_text FROM user_role_search WHERE user_role_id=2").Scan(&document); err != nil || document != x.want {
				t.Fatalf("stale document %q want %q: %v", document, x.want, err)
			}
			t.Logf("keyword lock barrier backends=%d/%d", pidA, pidB)
		})
	}
}
