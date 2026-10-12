package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/lib/pq"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

var roleColumns = []string{"id", "code", "name", "description", "application_id", "application_code", "status", "is_system", "version", "role_code", "created_at", "updated_at", "affected_user_count"}
var permissionColumns = []string{"id", "application_id", "application_code", "code", "description"}

func Persistence() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "role readers preserve safe fields and use one supplied handle", Run: roleReads},
		{Name: "missing and dependency reads preserve distinct semantics", Run: roleReadErrors},
		{Name: "sequence create conditional update and final mappings share one transaction", Run: roleWrites},
		{Name: "role writes preserve unique and dependency causes and affected row semantics", Run: roleWriteErrors},
		{Name: "nil and cancelled role repositories never fall back", Run: roleRepositoryGuards},
	}
}

func runRoleSQL(t *testing.T, steps []mocks.SQLStep, act func(context.Context, domain.Transaction)) {
	t.Helper()
	ctx := context.WithValue(context.Background(), struct{}{}, "role-request")
	steps = append([]mocks.SQLStep{{Op: "begin"}}, steps...)
	steps = append(steps, mocks.SQLStep{Op: "rollback"})
	mock := &mocks.SQL{Context: ctx, Steps: steps}
	db := mock.Open()
	defer db.Close()
	tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	act(ctx, tx)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}

func roleRow() []driver.Value {
	return []driver.Value{int64(7), "sales.editor", "Editor", nil, int64(2), "sales", "DISABLED", false, int64(3), int64(81), nil, nil, int64(4)}
}

func roleReads(t *testing.T) {
	runRoleSQL(t, []mocks.SQLStep{
		{Op: "query", SQL: "COUNT(DISTINCT ur.user_id)", Args: []driver.Value{int64(7)}, Columns: roleColumns, Rows: [][]driver.Value{roleRow()}},
		{Op: "query", SQL: "FROM applications a", Args: []driver.Value{"sales"}, Columns: []string{"id", "code", "name", "status", "is_system"}, Rows: [][]driver.Value{{int64(2), "sales", "Sales", "DISABLED", false}}},
		{Op: "query", SQL: `p.code COLLATE "C" ASC`, Args: []driver.Value{int64(7)}, Columns: permissionColumns, Rows: [][]driver.Value{{int64(9), int64(2), "sales", "sales.order.read", "Read"}}},
		{Op: "query", SQL: "p.application_id = $1", Args: []driver.Value{int64(2), "sales.order.read"}, Columns: permissionColumns, Rows: [][]driver.Value{{int64(9), int64(2), "sales", "sales.order.read", "Read"}}},
	}, func(ctx context.Context, tx domain.Transaction) {
		r := reader.NewRoleManagementReader(tx)
		role, err := r.Role(ctx, 7)
		if err != nil || role.Name != "Editor" || role.NumericCode != 81 || role.Version != 3 || role.AffectedUserCount != 4 || role.Description != nil || role.CreatedAt != nil || role.UpdatedAt != nil {
			t.Fatalf("role %+v %v", role, err)
		}
		app, err := r.Application(ctx, "sales")
		if err != nil || app.Status != types.ApplicationDisabled {
			t.Fatalf("app %+v %v", app, err)
		}
		perms, err := r.RolePermissions(ctx, 7)
		if err != nil || len(perms) != 1 || perms[0].ID != 9 {
			t.Fatalf("permissions %+v %v", perms, err)
		}
		perms, err = r.PermissionSet(ctx, 2, []string{"sales.order.read", "sales.order.read"})
		if err != nil || len(perms) != 1 {
			t.Fatalf("set %+v %v", perms, err)
		}
		perms, err = r.PermissionSet(ctx, 2, nil)
		if err != nil || len(perms) != 0 {
			t.Fatal("empty set queried database")
		}
	})
}

func roleReadErrors(t *testing.T) {
	cause := errors.New("controlled read failure")
	for _, name := range []string{"role missing", "app missing", "unknown permission", "dependency", "corrupt role"} {
		t.Run(name, func(t *testing.T) {
			step := mocks.SQLStep{Op: "query", SQL: "FROM role r", Args: []driver.Value{int64(7)}, Columns: roleColumns}
			var want error = roleerrors.ErrRoleNotFound
			switch name {
			case "app missing":
				step.SQL = "FROM applications a"
				step.Args = []driver.Value{"sales"}
				step.Columns = []string{"id"}
				want = applicationerrors.ErrApplicationNotFound
			case "unknown permission":
				step.SQL = "FROM permissions p"
				step.Args = []driver.Value{int64(2), "sales.order.read"}
				step.Columns = permissionColumns
				want = roleerrors.ErrInvalidPermissionSet
			case "dependency":
				step.Error = cause
				want = cause
			case "corrupt role":
				row := roleRow()
				row[8] = int64(0)
				step.Rows = [][]driver.Value{row}
				want = common.ErrDependencyUnavailable
			}
			runRoleSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
				r := reader.NewRoleManagementReader(tx)
				var err error
				switch name {
				case "app missing":
					_, err = r.Application(ctx, "sales")
				case "unknown permission":
					_, err = r.PermissionSet(ctx, 2, []string{"sales.order.read"})
				default:
					_, err = r.Role(ctx, 7)
				}
				if !errors.Is(err, want) {
					t.Fatalf("got %v want %v", err, want)
				}
			})
		})
	}
}

func roleWrites(t *testing.T) {
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	created := roleRow()
	created[6] = "ACTIVE"
	created[8] = int64(1)
	created[10] = at
	created[12] = int64(0)
	runRoleSQL(t, []mocks.SQLStep{
		{Op: "query", SQL: "INSERT INTO role (role_name, role_description, application_id, code, status, is_system, version)", Args: []driver.Value{"Editor", nil, int64(2), "sales.editor", "ACTIVE", false, int64(1)}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(7)}}},
		{Op: "query", SQL: "FROM role r", Args: []driver.Value{int64(7)}, Columns: roleColumns, Rows: [][]driver.Value{created}},
		{Op: "exec", SQL: "UPDATE role SET role_name = $1, role_description = $2, status = $3, version = $4, updated_at = $5 WHERE id = $6 AND version = $7", Args: []driver.Value{"Edited", nil, "ACTIVE", int64(2), at, int64(7), int64(1)}},
		{Op: "exec", SQL: "DELETE FROM role_permissions WHERE role_id = $1", Args: []driver.Value{int64(7)}, Result: driver.RowsAffected(2)},
		{Op: "exec", SQL: "INSERT INTO role_permissions (role_id, permission_id)", Args: []driver.Value{int64(7), int64(8)}},
		{Op: "exec", SQL: "INSERT INTO role_permissions (role_id, permission_id)", Args: []driver.Value{int64(7), int64(9)}},
		{Op: "exec", SQL: "DELETE FROM role_permissions WHERE role_id = $1", Args: []driver.Value{int64(7)}, Result: driver.RowsAffected(2)},
	}, func(ctx context.Context, tx domain.Transaction) {
		w := writer.NewRoleManagementWriter(tx)
		role, err := w.CreateRole(ctx, domain.RoleSummary{Code: "sales.editor", Name: "Editor", ApplicationID: 2, ApplicationCode: "sales", Status: types.RoleActive, Version: 1})
		if err != nil || role.ID != 7 || role.NumericCode != 81 || role.CreatedAt == nil {
			t.Fatalf("create %+v %v", role, err)
		}
		role.Name = "Edited"
		role.Version = 2
		role.UpdatedAt = &at
		ok, err := w.UpdateRole(ctx, role, 1)
		if err != nil || !ok {
			t.Fatalf("update %v %v", ok, err)
		}
		ids := []uint64{9, 8}
		if err = w.ReplacePermissions(ctx, 7, ids); err != nil {
			t.Fatal(err)
		}
		if ids[0] != 9 {
			t.Fatal("mutated caller ids")
		}
		if err = w.ReplacePermissions(ctx, 7, nil); err != nil {
			t.Fatal(err)
		}
	})
}

func roleWriteErrors(t *testing.T) {
	unique := &pq.Error{Code: "23505"}
	cause := errors.New("controlled write failure")
	for _, name := range []string{"duplicate", "insert failure", "zero rows", "many rows", "delete failure", "mapping failure"} {
		t.Run(name, func(t *testing.T) {
			step := mocks.SQLStep{Op: "query", SQL: "INSERT INTO role", Args: []driver.Value{"Editor", nil, int64(2), "sales.editor", "ACTIVE", false, int64(1)}, Error: cause}
			steps := []mocks.SQLStep{}
			want := cause
			switch name {
			case "duplicate":
				step.Error = unique
				want = roleerrors.ErrRoleCodeAlreadyExists
			case "zero rows", "many rows":
				step = mocks.SQLStep{Op: "exec", SQL: "UPDATE role", Args: []driver.Value{"Editor", nil, "ACTIVE", int64(2), time.Unix(100, 0).UTC(), int64(7), int64(1)}, Result: driver.RowsAffected(0)}
				want = nil
				if name == "many rows" {
					step.Result = driver.RowsAffected(2)
					want = common.ErrDependencyUnavailable
				}
			case "delete failure", "mapping failure":
				step = mocks.SQLStep{Op: "exec", SQL: "DELETE FROM role_permissions", Args: []driver.Value{int64(7)}, Error: cause}
				if name == "mapping failure" {
					step.Error = nil
					steps = append(steps, step)
					step = mocks.SQLStep{Op: "exec", SQL: "INSERT INTO role_permissions", Args: []driver.Value{int64(7), int64(9)}, Error: cause}
				}
			}
			steps = append(steps, step)
			runRoleSQL(t, steps, func(ctx context.Context, tx domain.Transaction) {
				w := writer.NewRoleManagementWriter(tx)
				var err error
				switch name {
				case "zero rows", "many rows":
					at := time.Unix(100, 0).UTC()
					ok, e := w.UpdateRole(ctx, domain.RoleSummary{ID: 7, Name: "Editor", Code: "sales.editor", ApplicationID: 2, ApplicationCode: "sales", Status: types.RoleActive, Version: 2, NumericCode: 81, UpdatedAt: &at}, 1)
					err = e
					if ok {
						t.Fatal("accepted unexpected affected rows")
					}
				case "delete failure", "mapping failure":
					err = w.ReplacePermissions(ctx, 7, []uint64{9})
				default:
					_, err = w.CreateRole(ctx, domain.RoleSummary{Name: "Editor", Code: "sales.editor", ApplicationID: 2, ApplicationCode: "sales", Status: types.RoleActive, Version: 1})
				}
				if !errors.Is(err, want) {
					t.Fatalf("got %v want %v", err, want)
				}
				if name == "duplicate" && !errors.Is(err, unique) {
					t.Fatal("unique cause lost")
				}
			})
		})
	}
}

func roleRepositoryGuards(t *testing.T) {
	r := reader.NewRoleManagementReader(nil)
	w := writer.NewRoleManagementWriter(nil)
	ctx := context.Background()
	_, err := r.Role(ctx, 7)
	if !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	_, err = r.Application(ctx, "sales")
	if !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	_, err = r.RolePermissions(ctx, 7)
	if !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	_, err = r.PermissionSet(ctx, 2, nil)
	if !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	_, err = w.CreateRole(ctx, domain.RoleSummary{})
	if !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	_, err = w.UpdateRole(ctx, domain.RoleSummary{}, 1)
	if !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	if err = w.ReplacePermissions(ctx, 7, nil); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = r.Role(ctx, 7)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err = w.ReplacePermissions(ctx, 7, nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
