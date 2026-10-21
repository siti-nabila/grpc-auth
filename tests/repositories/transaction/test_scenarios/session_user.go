package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"testing"
)

func SessionUserPersistence() []testutils.Scenario {
	return []testutils.Scenario{{Name: "session admin target reader projects only locked identity status and counters", Run: sessionUserPersistence}}
}
func sessionUserPersistence(t *testing.T) {
	for _, kind := range []string{"success", "missing", "failure", "cancel", "zero", "overflow", "nil handle"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			id := uint64(43)
			cause := errors.New("controlled dependency")
			step := mocks.SQLStep{Op: "query", SQL: "SELECT id, status, version, auth_version, deleted_at FROM auth WHERE id = $1 LIMIT 1", Args: []driver.Value{int64(43)}, Columns: []string{"id", "status", "version", "auth_version", "deleted_at"}, Rows: [][]driver.Value{{int64(43), "ACTIVE", int64(7), int64(9), nil}}}
			if kind == "missing" {
				step.Rows = nil
			}
			if kind == "failure" {
				step.Error = cause
			}
			mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, step, {Op: "rollback"}}}
			if kind == "cancel" || kind == "zero" || kind == "overflow" || kind == "nil handle" {
				mock.Steps = []mocks.SQLStep{{Op: "begin"}, {Op: "rollback"}}
			}
			db := mock.Open()
			defer db.Close()
			tx, e := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
			if e != nil {
				t.Fatal(e)
			}
			r := reader.NewSessionUserReader(tx)
			switch kind {
			case "cancel":
				cancel()
			case "zero":
				id = 0
			case "overflow":
				id = 2147483648
			case "nil handle":
				r = reader.NewSessionUserReader(nil)
			}
			row, err := r.GetByID(ctx, id)
			switch kind {
			case "success":
				if err != nil || row.ID != 43 || row.Status != "ACTIVE" || row.Version != 7 || row.AuthVersion != 9 || row.DeletedAt.Valid {
					t.Fatal("target mapping", err)
				}
			case "missing":
				if !errors.Is(err, auth.ErrNotFound) {
					t.Fatal("not found", err)
				}
			case "failure":
				if !errors.Is(err, cause) {
					t.Fatal("cause lost", err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation", err)
				}
			case "zero", "overflow":
				if !errors.Is(err, common.ErrBadRequest) {
					t.Fatal("invalid ID", err)
				}
			case "nil handle":
				if !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal("nil handle", err)
				}
			}
			_ = tx.Rollback()
			if e = mock.Verify(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
