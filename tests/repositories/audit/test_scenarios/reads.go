package test_scenarios

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"math"
	"testing"
	"time"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "parameterized exact filters count and BIGINT cursor pages", Run: pages}, {Name: "reader errors suppress partial pages and use same snapshot", Run: failures}}
}
func run(t *testing.T, steps []mocks.SQLStep, act func(context.Context, domain.Transaction)) {
	t.Helper()
	ctx := context.WithValue(context.Background(), struct{}{}, "audit-request")
	m := &mocks.SQL{Context: ctx, Steps: append(append([]mocks.SQLStep{{Op: "begin"}}, steps...), mocks.SQLStep{Op: "rollback"})}
	db := m.Open()
	defer db.Close()
	tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).BeginSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	act(ctx, tx)
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = m.Verify(); err != nil {
		t.Fatal(err)
	}
	if !m.Options.ReadOnly || m.Options.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) || db.Stats().InUse != 0 {
		t.Fatal("snapshot/lease", m.Options, db.Stats())
	}
}
func pages(t *testing.T) {
	id := uint64(7)
	from := fixtures.ServiceTime.Add(1)
	before := fixtures.ServiceTime.Add(time.Second + 1)
	q := domain.AuditLogQuery{Limit: 1, ActorUserID: &id, ActorService: "service", Action: "literal%_", TargetType: "rpc", TargetID: domain.AuditLogMethod, OccurredFrom: &from, OccurredBefore: &before}
	args := []driver.Value{int64(7), "service", "literal%_", "rpc", domain.AuditLogMethod, fixtures.ServiceTime.Add(time.Microsecond), fixtures.ServiceTime.Add(time.Second + time.Microsecond)}
	count := fixtures.AuditCountStep(2)
	count.Args = args
	page := fixtures.AuditPageStep(fixtures.AuditRow(math.MaxInt64), fixtures.AuditRow(math.MaxInt32+1))
	page.Args = args
	page.SQL = "ORDER BY occurred_at DESC, id DESC LIMIT 2 OFFSET 0"
	run(t, []mocks.SQLStep{count, page}, func(ctx context.Context, tx domain.Transaction) {
		p, err := reader.NewAuditReader(tx).List(ctx, q)
		if err != nil || p.Total != 2 || p.TotalPages != 2 || !p.HasNext || p.HasPrev || len(p.Items) != 1 || p.Items[0].ID != math.MaxInt64 {
			t.Fatal(p, err)
		}
		q, _ = q.Normalized()
		q.LastID = p.NextCursor
		c, err := q.Cursor()
		if err != nil || c.ID != "9223372036854775807" {
			t.Fatal(c, err)
		}
	})
	page = fixtures.AuditPageStep()
	page.Args = append(append([]driver.Value{}, args...), fixtures.ServiceTime, fixtures.ServiceTime, int64(math.MaxInt64))
	page.SQL = "(occurred_at < $8 OR (occurred_at = $9 AND id < $10))"
	run(t, []mocks.SQLStep{count, page}, func(ctx context.Context, tx domain.Transaction) {
		p, err := reader.NewAuditReader(tx).List(ctx, q)
		if err != nil || p.Total != 2 || p.HasNext || !p.HasPrev || p.NextCursor != "" {
			t.Fatal(p, err)
		}
	})
	page = fixtures.AuditPageStep()
	page.SQL = "LIMIT 21 OFFSET 40"
	run(t, []mocks.SQLStep{fixtures.AuditCountStep(0), page}, func(ctx context.Context, tx domain.Transaction) {
		p, err := reader.NewAuditReader(tx).List(ctx, domain.AuditLogQuery{Page: 3})
		if err != nil || p.TotalPages != 0 || p.Total != 0 || !p.HasPrev || p.Items == nil {
			t.Fatal(p, err)
		}
	})
}
func failures(t *testing.T) {
	for _, name := range []string{"count", "page", "corrupt", "negative count", "metadata overflow", "cancel", "invalid"} {
		t.Run(name, func(t *testing.T) {
			steps := []mocks.SQLStep{fixtures.AuditCountStep(1), fixtures.AuditPageStep(fixtures.AuditRow(7))}
			cause := errors.New("synthetic private database failure")
			var want error
			q := domain.AuditLogQuery{}
			switch name {
			case "count":
				steps[0].Error = cause
				steps = steps[:1]
				want = cause
			case "page":
				steps[1].Error = cause
				want = cause
			case "corrupt":
				steps[1].Rows[0][7] = `{"password":"synthetic"}`
				want = common.ErrDependencyUnavailable
			case "negative count":
				steps[0].Rows[0][0] = int64(-1)
				steps = steps[:1]
				want = common.ErrDependencyUnavailable
			case "metadata overflow":
				steps[0].Rows[0][0] = int64(math.MaxInt64)
				steps = steps[:1]
				want = common.ErrDependencyUnavailable
			case "cancel":
				steps = nil
				want = context.Canceled
			case "invalid":
				steps = nil
				q.Limit = 101
				want = common.ErrBadRequest
			}
			run(t, steps, func(ctx context.Context, tx domain.Transaction) {
				if name == "cancel" {
					c, cancel := context.WithCancel(ctx)
					cancel()
					ctx = c
				}
				p, err := reader.NewAuditReader(tx).List(ctx, q)
				if !errors.Is(err, want) || p.Items != nil || p.Total != 0 {
					t.Fatal(p, err, want)
				}
				if (name == "count" || name == "page") && !errors.Is(err, cause) {
					t.Fatal("cause lost", err)
				}
			})
		})
	}
}
