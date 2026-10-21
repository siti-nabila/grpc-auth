package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
)

func mappingInventory(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	for _, kind := range []string{"empty", "active", "deleted", "mixed", "dependency", "invalid identity", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			cause := errors.New("controlled inventory failure")
			step := mocks.SQLStep{Op: "query", SQL: "WHERE rp.role_id = $1 ORDER BY rp.permission_id ASC", Args: []driver.Value{int64(7)}, Columns: []string{"role_id", "permission_id", "deleted_at"}}
			switch kind {
			case "active":
				step.Rows = [][]driver.Value{{int64(7), int64(8), nil}}
			case "deleted":
				step.Rows = [][]driver.Value{{int64(7), int64(8), at}}
			case "mixed":
				step.Rows = [][]driver.Value{{int64(7), int64(8), nil}, {int64(7), int64(9), at}}
			case "dependency":
				step.Error = cause
			case "invalid identity":
				step.Rows = [][]driver.Value{{int64(6), int64(8), nil}}
			case "duplicate":
				step.Rows = [][]driver.Value{{int64(7), int64(8), nil}, {int64(7), int64(8), at}}
			}
			runRoleSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
				rows, err := reader.NewRoleManagementReader(tx).RolePermissionPairs(ctx, 7)
				if kind == "dependency" {
					if !errors.Is(err, cause) || rows != nil {
						t.Fatal(rows, err)
					}
					return
				}
				if kind == "invalid identity" || kind == "duplicate" {
					if !errors.Is(err, common.ErrDependencyUnavailable) || rows != nil {
						t.Fatal(rows, err)
					}
					return
				}
				if err != nil || len(rows) != len(step.Rows) {
					t.Fatal(rows, err)
				}
				for i, row := range rows {
					if row.RoleID != 7 || row.PermissionID != uint64(step.Rows[i][1].(int64)) || (row.DeletedAt == nil) != (step.Rows[i][2] == nil) {
						t.Fatal(rows)
					}
				}
			})
		})
	}
}

func mappingWrites(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	for _, operation := range []string{"remove", "restore", "insert"} {
		for _, kind := range []string{"success", "zero", "many", "negative", "rows error", "execution error"} {
			t.Run(operation+"/"+kind, func(t *testing.T) {
				cause := errors.New("controlled mapping failure")
				changes := domain.RolePermissionChanges{}
				step := mocks.SQLStep{Op: "exec", SQL: "AND deleted_at IS NULL", Args: []driver.Value{at, int64(7), int64(8)}}
				switch operation {
				case "remove":
					changes.Removed = []uint64{8}
				case "restore":
					changes.Restored = []uint64{8}
					step.SQL = "AND deleted_at IS NOT NULL"
					step.Args[0] = nil
				case "insert":
					changes.Inserted = []uint64{8}
					step.SQL = "INSERT INTO role_permissions (role_id, permission_id, deleted_at)"
					step.Args = []driver.Value{int64(7), int64(8), nil}
				}
				var want error
				switch kind {
				case "zero":
					step.Result = driver.RowsAffected(0)
					want = common.ErrDependencyUnavailable
				case "many":
					step.Result = driver.RowsAffected(2)
					want = common.ErrDependencyUnavailable
				case "negative":
					step.Result = driver.RowsAffected(-1)
					want = common.ErrDependencyUnavailable
				case "rows error":
					step.Result = mappingResultError{cause}
					want = cause
				case "execution error":
					step.Error = cause
					want = cause
				}
				runRoleSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
					if err := writer.NewRoleManagementWriter(tx).ReplacePermissions(ctx, 7, changes, at); !errors.Is(err, want) {
						t.Fatal(err, want)
					}
				})
			})
		}
	}
	runRoleSQL(t, nil, func(ctx context.Context, tx domain.Transaction) {
		for _, changes := range []domain.RolePermissionChanges{{Removed: []uint64{9, 8}}, {Restored: []uint64{8}, Inserted: []uint64{8}}, {Inserted: []uint64{0}}} {
			if err := writer.NewRoleManagementWriter(tx).ReplacePermissions(ctx, 7, changes, at); !errors.Is(err, common.ErrBadRequest) {
				t.Fatal(err)
			}
		}
		if err := writer.NewRoleManagementWriter(tx).ReplacePermissions(ctx, 7, domain.RolePermissionChanges{}, at); err != nil {
			t.Fatal(err)
		}
	})
}

type mappingResultError struct{ err error }

func (r mappingResultError) LastInsertId() (int64, error) { return 0, r.err }
func (r mappingResultError) RowsAffected() (int64, error) { return 0, r.err }
