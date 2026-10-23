//go:build postgres

package test_scenarios

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	ormconfig "github.com/siti-nabila/orm/config"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

var namespaceSequence atomic.Uint64

func FoundationHarness() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "fixture namespaces and replacement connections stay isolated", Run: fixtureIsolation},
		{Name: "setup failure does not adopt or delete another namespace", Run: fixtureSetupFailure},
		{Name: "two physical connections overlap through controlled barriers", Run: overlappingConnections},
		{Name: "controlled holder cancellation releases its scope", Run: canceledHolder},
	}
}

func dedicatedFixture(t *testing.T) (context.Context, fixtures.Config, *fixtures.Schema) {
	t.Helper()
	cfg, err := fixtures.LoadConfig(os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	name := fmt.Sprintf("grpc_auth_test_%d_%d", os.Getpid(), namespaceSequence.Add(1))
	fixture, err := fixtures.NewSchema(ctx, cfg, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := fixture.Close(cleanupCtx); err != nil {
			t.Error(err)
		}
	})
	return ctx, cfg, fixture
}

func physicalIdentity(t *testing.T, ctx context.Context, conn *sql.Conn, want string) int {
	t.Helper()
	var name string
	var pid int
	if err := conn.QueryRowContext(ctx, "SELECT current_schema(),pg_backend_pid()").Scan(&name, &pid); err != nil {
		t.Fatal("physical connection identity query failed")
	}
	if name != want {
		t.Fatalf("schema = %q, want fixture %q", name, want)
	}
	return pid
}

func fixtureIsolation(t *testing.T) {
	ctx, cfg, fixture := dedicatedFixture(t)
	a, err := fixture.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := fixture.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	pidA := physicalIdentity(t, ctx, a, fixture.Name)
	pidB := physicalIdentity(t, ctx, b, fixture.Name)
	if pidA == pidB {
		t.Fatal("two logical connections must use distinct physical backends")
	}
	var columns, constraints, indexes int
	const metadataQuery = `SELECT
      (SELECT count(*) FROM information_schema.columns WHERE table_schema=$1 AND table_name IN ('auth','profile','role','user_role','audit_logs','outbox_events','idempotency_records')),
      (SELECT count(*) FROM pg_constraint con JOIN pg_class c ON c.oid=con.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname IN ('auth','profile','role','user_role','audit_logs','outbox_events','idempotency_records') AND con.convalidated),
      (SELECT count(*) FROM pg_indexes WHERE schemaname=$1 AND tablename IN ('auth','profile','role','user_role','audit_logs','outbox_events','idempotency_records'))`
	// The existing fixture includes nullable profile.country_code.
	if err := a.QueryRowContext(ctx, metadataQuery, fixture.Name).Scan(&columns, &constraints, &indexes); err != nil || columns != 71 || constraints != 35 || indexes != 22 {
		t.Fatalf("fixture metadata counts = %d/%d/%d, want 71/35/22", columns, constraints, indexes)
	}
	var roles, rows int
	if err := a.QueryRowContext(ctx, "SELECT count(*) FROM role WHERE role_code=1 AND code='iam.member'").Scan(&roles); err != nil || roles != 1 {
		t.Fatal("synthetic default member fixture missing")
	}
	if err := b.QueryRowContext(ctx, "SELECT count(*) FROM auth WHERE email='fixture@example.test'").Scan(&rows); err != nil || rows != 1 {
		t.Fatal("fixture must not copy application users")
	}
	// Explicitly discard a physical connection, then verify startup options again.
	_ = a.Raw(func(any) error { return driver.ErrBadConn })
	_ = a.Close()
	replacement, err := fixture.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if pid := physicalIdentity(t, ctx, replacement, fixture.Name); pid == pidA {
		t.Fatal("replacement must have a new backend")
	}
	_ = replacement.Close()
	_ = b.Close()
	admin, err := fixtures.Connect(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := admin.QueryRowContext(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name IN ('auth','profile','role','user_role','audit_logs','outbox_events','idempotency_records')").Scan(&rows); err != nil || rows != 0 {
		t.Fatal("fixture tables escaped into public schema")
	}
	if err := fixture.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if fixture.DB.Stats().OpenConnections != 0 {
		t.Fatal("fixture pool connections not closed")
	}
	if err := admin.QueryRowContext(ctx, "SELECT count(*) FROM pg_namespace WHERE nspname=$1", fixture.Name).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("fixture namespace not cleaned")
	}
	if err := fixture.Close(ctx); err != nil {
		t.Fatal("duplicate cleanup must be harmless")
	}
}

func fixtureSetupFailure(t *testing.T) {
	ctx, cfg, fixture := dedicatedFixture(t)
	other, err := fixtures.NewSchema(ctx, cfg, fixture.Name)
	if err == nil || other != nil {
		t.Fatal("existing schema must not be adopted")
	}
	var rows int
	if err := fixture.DB.QueryRowContext(ctx, "SELECT count(*) FROM role").Scan(&rows); err != nil || rows != 1 {
		t.Fatal("failed setup damaged the owning fixture")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	other, err = fixtures.NewSchema(canceled, cfg, fixture.Name+"_canceled")
	if err == nil || other != nil {
		t.Fatal("canceled setup must fail without opening a fixture")
	}
	if err := fixture.DB.QueryRowContext(ctx, "SELECT count(*) FROM pg_namespace WHERE nspname=$1", fixture.Name+"_canceled").Scan(&rows); err != nil || rows != 0 {
		t.Fatal("canceled setup leaked a namespace")
	}
	if _, err := fixtures.NewSchema(ctx, cfg, "public"); err == nil {
		t.Fatal("public namespace must be rejected before DDL")
	}
}

func overlappingConnections(t *testing.T) {
	ctx, _, fixture := dedicatedFixture(t)
	a, err := fixture.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := fixture.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if physicalIdentity(t, ctx, a, fixture.Name) == physicalIdentity(t, ctx, b, fixture.Name) {
		t.Fatal("contention proof requires distinct physical connections")
	}
	released, finished := false, false
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		tx, err := a.BeginTx(ctx, nil)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback()
		adapter := orm.NewSqlTransactionAdapter(ctx, tx, dialect.NewPostgres(), ormconfig.Config{})
		acquired, err := adapter.TryLock(ctx, "auth:user:1")
		if err != nil {
			done <- err
			return
		}
		if !acquired {
			done <- fmt.Errorf("first harness scope unavailable")
			return
		}
		close(held)
		select {
		case <-release:
			done <- adapter.Commit()
		case <-ctx.Done():
			done <- ctx.Err()
		}
	}()
	// Ensure release and goroutine completion before fixture/pool cleanup on failure.
	defer func() {
		if !released {
			close(release)
		}
		if finished {
			return
		}
		select {
		case err := <-done:
			if err != nil {
				t.Error("controlled connection A failed")
			}
		case <-ctx.Done():
			t.Error("controlled connection A did not finish")
		}
	}()
	select {
	case <-held:
	case <-done:
		finished = true
		t.Fatal("connection A failed before holding barrier")
	case <-ctx.Done():
		t.Fatal("connection A did not reach holding barrier")
	}
	tx, err := b.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	adapter := orm.NewSqlTransactionAdapter(ctx, tx, dialect.NewPostgres(), ormconfig.Config{})
	if acquired, err := adapter.TryLock(ctx, "auth:user:1"); err != nil || acquired {
		t.Fatal("connection B must observe same-key contention while A holds scope")
	}
	if acquired, err := adapter.TryLock(ctx, "auth:user:2"); err != nil || !acquired {
		t.Fatal("connection B must make progress on an independent scope before A releases")
	}
	if err := adapter.Rollback(); err != nil {
		t.Fatal("connection B rollback failed")
	}
	close(release)
	released = true
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal("connection A commit failed")
		}
	case <-ctx.Done():
		t.Fatal("connection A failed to finish after barrier release")
	}
	after, err := b.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer after.Rollback()
	next := orm.NewSqlTransactionAdapter(ctx, after, dialect.NewPostgres(), ormconfig.Config{})
	if acquired, err := next.TryLock(ctx, "auth:user:1"); err != nil || !acquired {
		t.Fatal("scope not available after connection A committed")
	}
	if err := next.Rollback(); err != nil {
		t.Fatal("final harness rollback failed")
	}
}

// This proves harness-controlled cancellation/rollback, not feature request-context propagation.
func canceledHolder(t *testing.T) {
	ctx, _, fixture := dedicatedFixture(t)
	a, err := fixture.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := fixture.DB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if physicalIdentity(t, ctx, a, fixture.Name) == physicalIdentity(t, ctx, b, fixture.Name) {
		t.Fatal("cancellation harness needs distinct backends")
	}
	held := make(chan struct{})
	done := make(chan error, 1)
	stop, cancel := context.WithCancel(ctx)
	finished := false
	defer func() {
		cancel()
		if finished {
			return
		}
		select {
		case err := <-done:
			if err != nil {
				t.Error("canceled holder rollback failed")
			}
		case <-ctx.Done():
			t.Error("canceled holder did not finish")
		}
	}()
	go func() {
		// Parent transaction stays available for explicit, acknowledged rollback.
		tx, err := a.BeginTx(ctx, nil)
		if err != nil {
			done <- err
			return
		}
		defer tx.Rollback()
		adapter := orm.NewSqlTransactionAdapter(stop, tx, dialect.NewPostgres(), ormconfig.Config{})
		ok, err := adapter.TryLock(stop, "auth:user:3")
		if err != nil {
			done <- err
			return
		}
		if !ok {
			done <- fmt.Errorf("cancellation harness scope unavailable")
			return
		}
		close(held)
		<-stop.Done()
		done <- adapter.Rollback()
	}()
	select {
	case <-held:
	case <-done:
		finished = true
		t.Fatal("holder failed before cancellation barrier")
	case <-ctx.Done():
		t.Fatal("holder barrier timed out")
	}
	tx, err := b.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	other := orm.NewSqlTransactionAdapter(ctx, tx, dialect.NewPostgres(), ormconfig.Config{})
	if ok, err := other.TryLock(ctx, "auth:user:3"); err != nil || ok {
		t.Fatal("contender acquired held cancellation scope")
	}
	if err := other.Rollback(); err != nil {
		t.Fatal("contender rollback failed")
	}
	cancel()
	select {
	case err := <-done:
		finished = true
		if err != nil {
			t.Fatal("holder did not acknowledge rollback")
		}
	case <-ctx.Done():
		t.Fatal("cancellation rollback timed out")
	}
	next, err := b.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Rollback()
	adapter := orm.NewSqlTransactionAdapter(ctx, next, dialect.NewPostgres(), ormconfig.Config{})
	if ok, err := adapter.TryLock(ctx, "auth:user:3"); err != nil || !ok {
		t.Fatal("canceled scope not released after acknowledged rollback")
	}
	if err := adapter.Rollback(); err != nil {
		t.Fatal("final cancellation harness rollback failed")
	}
}
