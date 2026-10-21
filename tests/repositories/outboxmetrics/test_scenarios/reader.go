package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"testing"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "single safe aggregate forwards context and preserves empty dependency cancel semantics", Run: aggregate}}
}

func aggregate(t *testing.T) {
	for _, kind := range []string{"empty", "pending", "negative", "missing oldest", "unexpected oldest", "database", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), struct{}{}, "request"))
			defer cancel()
			step := mocks.SQLStep{Op: "query", SQL: "SELECT COUNT(*) AS pending_count, MIN(created_at) AS oldest_created_at FROM outbox_events WHERE published_at IS NULL", Columns: []string{"pending_count", "oldest_created_at"}, Rows: [][]driver.Value{{int64(0), nil}}}
			cause := errors.New("private-database-sentinel")
			var want error
			switch kind {
			case "pending":
				step.Rows[0] = []driver.Value{int64(2), fixtures.ServiceTime}
			case "negative":
				step.Rows[0][0] = int64(-1)
				want = common.ErrDependencyUnavailable
			case "missing oldest":
				step.Rows[0][0] = int64(1)
				want = common.ErrDependencyUnavailable
			case "unexpected oldest":
				step.Rows[0][1] = fixtures.ServiceTime
				want = common.ErrDependencyUnavailable
			case "database":
				step.Error = cause
				want = cause
			case "cancel":
				cancel()
				want = context.Canceled
			}
			m := &mocks.SQL{Context: ctx}
			if kind != "cancel" {
				m.Steps = []mocks.SQLStep{step}
			}
			db := m.Open()
			defer db.Close()
			cfg := database.GetORMConfig()
			cfg.EnableDebug = false
			r := reader.NewOutboxMetricsReader(orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), cfg))
			v, err := r.Pending(ctx)
			if !errors.Is(err, want) {
				t.Fatal("aggregate result/cause", v, err, want)
			}
			if kind == "pending" && (v.Count != 2 || !v.Oldest.Valid || !v.Oldest.Time.Equal(fixtures.ServiceTime)) {
				t.Fatal(v)
			}
			if want != nil && (v.Count != 0 || v.Oldest.Valid) {
				t.Fatal("partial failure gauge")
			}
			if err := m.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := reader.NewOutboxMetricsReader(nil).Pending(context.Background()); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("nil handle", err)
	}
}
