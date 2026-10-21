package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"github.com/lib/pq"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
	"reflect"
	"testing"
	"time"
)

var columns = []string{"id", "role_id", "code", "role_code", "name", "status", "role_status", "application_id", "application_code", "application_status", "is_system", "application_access_status", "granted_by", "granted_at", "revoked_by", "revoked_at", "revoke_reason"}

func row() []driver.Value {
	return []driver.Value{int64(11), int64(2), "sales.editor", int64(8), "Editor", "ACTIVE", "DISABLED", int64(2), "sales", "DISABLED", false, nil, nil, nil, nil, nil, nil}
}
func Persistence() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered role absent pair and nullable historical sources stay distinct", Run: assignmentReads},
		{Name: "all17 scalar sorts literal filters total projection and typed continuation use bound SQL", Run: assignmentLists},
		{Name: "grant reactivation revoke and conditional version touch only intended columns", Run: assignmentWrites},
		{Name: "zero rows unique FK DB cancellation and nil handles preserve failures", Run: assignmentFailures},
	}
}

func runSQL(t *testing.T, steps []mocks.SQLStep, act func(context.Context, domain.Transaction)) {
	t.Helper()
	ctx := context.WithValue(context.Background(), struct{}{}, "assignment-request")
	steps = append([]mocks.SQLStep{{Op: "begin"}}, steps...)
	steps = append(steps, mocks.SQLStep{Op: "rollback"})
	mock := &mocks.SQL{Context: ctx, Steps: steps}
	db := mock.Open()
	defer db.Close()
	tx, e := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	act(ctx, tx)
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	if e = mock.Verify(); e != nil {
		t.Fatal(e)
	}
}
func assignmentReads(t *testing.T) {
	absent := row()
	absent[0] = int64(0)
	absent[5] = ""
	runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "LEFT JOIN user_role ur ON ur.user_id = target.id AND ur.role_id = r.id", Args: []driver.Value{int64(7), int64(2), int64(3)}, Columns: columns, Rows: [][]driver.Value{absent}}}, func(ctx context.Context, tx domain.Transaction) {
		rows, e := reader.NewUserRoleAssignmentReader(tx).AssignmentRoles(ctx, 7, []uint64{2, 3})
		if e != nil || len(rows) != 1 || rows[0].ID != 0 || rows[0].RoleID != 2 || rows[0].Status != "" || rows[0].GrantedAt != nil || rows[0].GrantedBy != nil || rows[0].ApplicationAccessStatus != nil {
			t.Fatal(rows, e)
		}
	})
	for _, name := range []string{"empty registered set", "dependency"} {
		t.Run(name, func(t *testing.T) {
			cause := errors.New("controlled query failure")
			step := mocks.SQLStep{Op: "query", SQL: "WHERE target.id = $1 AND r.id IN ($2)", Args: []driver.Value{int64(7), int64(2)}, Columns: columns}
			if name == "dependency" {
				step.Error = cause
			}
			runSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
				rows, e := reader.NewUserRoleAssignmentReader(tx).AssignmentRoles(ctx, 7, []uint64{2})
				if name == "dependency" {
					if !errors.Is(e, cause) {
						t.Fatal(e)
					}
				} else if e != nil || len(rows) != 0 {
					t.Fatal(rows, e)
				}
			})
		})
	}
}

func assignmentLists(t *testing.T) {
	expr := map[string]string{"id": "ur.id", "role_id": "r.id", "code": "r.code", "role_code": "r.role_code", "name": "r.role_name", "status": "ur.status", "role_status": "r.status", "application_id": "a.id", "application_code": "a.code", "application_status": "a.status", "is_system": "r.is_system", "application_access_status": "access.status", "granted_by": "ur.granted_by", "granted_at": "ur.granted_at", "revoked_by": "ur.revoked_by", "revoked_at": "ur.revoked_at", "revoke_reason": "ur.revoke_reason"}
	for _, f := range domain.UserRoleAssignmentCatalog.Fields() {
		for _, desc := range []bool{false, true} {
			t.Run(f.Name+map[bool]string{true: " DESC", false: " ASC"}[desc], func(t *testing.T) {
				order := expr[f.Name] + map[bool]string{true: " DESC", false: " ASC"}[desc] + " NULLS LAST"
				q := domain.CatalogQuery{TargetUserID: 7, Sort: []orm.SortField{{Field: f.Name, Desc: desc}}, Fields: []string{"name"}}
				runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "COUNT(DISTINCT ur.id) AS total", Args: []driver.Value{int64(7)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(1)}}}, {Op: "query", SQL: order, Args: []driver.Value{int64(7)}, Columns: columns, Rows: [][]driver.Value{row()}}}, func(ctx context.Context, tx domain.Transaction) {
					page, e := reader.NewUserRoleAssignmentReader(tx).ListAssignments(ctx, q)
					if e != nil || page.Total != 1 || page.TotalPages != 1 || len(page.Items) != 1 || page.Items[0].ID != 11 || page.Items[0].RoleID != 2 || page.Items[0].Code != "" || page.Items[0].Name != "Editor" {
						t.Fatal(page, e)
					}
				})
			})
		}
	}
	args := []driver.Value{int64(7), "sales", "ACTIVE", "REVOKED", `%50\%\_%`, `%50\%\_%`}
	q := domain.CatalogQuery{TargetUserID: 7, ApplicationCode: "sales", Statuses: []string{"REVOKED", "ACTIVE"}, Search: &orm.SearchQuery{Keyword: "50%_"}, Limit: 1}
	a, b := row(), row()
	b[0] = int64(12)
	runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "ur.status IN ($3, $4)", Args: args, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(2)}}}, {Op: "query", SQL: `ILIKE $5 ESCAPE '\'`, Args: args, Columns: columns, Rows: [][]driver.Value{a, b}}}, func(ctx context.Context, tx domain.Transaction) {
		page, e := reader.NewUserRoleAssignmentReader(tx).ListAssignments(ctx, q)
		if e != nil || !page.HasNext || page.Total != 2 || page.NextCursor == "" || len(page.Items) != 1 {
			t.Fatal(page, e)
		}
		normalized, _ := domain.UserRoleAssignmentCatalog.Normalize(q)
		normalized.LastID = page.NextCursor
		keys, e := domain.UserRoleAssignmentCatalog.ParseCursor(normalized)
		if e != nil || *keys[0].Value != "11" {
			t.Fatal(keys, e)
		}
	})
	runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "COUNT(DISTINCT ur.id)", Args: []driver.Value{int64(7)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(1)}}}, {Op: "query", SQL: "ur.id > $2", Args: []driver.Value{int64(7), int64(11)}, Columns: columns}}, func(ctx context.Context, tx domain.Transaction) {
		page, e := reader.NewUserRoleAssignmentReader(tx).ListAssignments(ctx, domain.CatalogQuery{TargetUserID: 7, LastID: "11"})
		if e != nil || page.Total != 1 || len(page.Items) != 0 || page.NextCursor != "" {
			t.Fatal(page, e)
		}
	})
	runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "COUNT(DISTINCT ur.id)", Args: []driver.Value{int64(7)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(0)}}}, {Op: "query", SQL: "OFFSET 20", Args: []driver.Value{int64(7)}, Columns: columns}}, func(ctx context.Context, tx domain.Transaction) {
		page, e := reader.NewUserRoleAssignmentReader(tx).ListAssignments(ctx, domain.CatalogQuery{TargetUserID: 7, Page: 2})
		if e != nil || page.Total != 0 || page.TotalPages != 0 || page.NextCursor != "" {
			t.Fatal(page, e)
		}
	})
}

func assignmentWrites(t *testing.T) {
	at := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	runSQL(t, []mocks.SQLStep{
		{Op: "exec", SQL: "INSERT INTO user_role (user_id, role_id, status, granted_by, granted_at)", Args: []driver.Value{int64(7), int64(2), "ACTIVE", int64(42), at}},
		{Op: "exec", SQL: "revoked_by = $4, revoked_at = $5, revoke_reason = $6 WHERE id = $7 AND user_id = $8 AND role_id = $9 AND status = $10", Args: []driver.Value{"ACTIVE", int64(42), at, nil, nil, nil, int64(11), int64(7), int64(2), "REVOKED"}},
		{Op: "exec", SQL: "SET status = $1, revoked_by = $2, revoked_at = $3, revoke_reason = $4 WHERE id = $5 AND user_id = $6 AND role_id = $7 AND status = $8", Args: []driver.Value{"REVOKED", int64(42), at, "Review access", int64(11), int64(7), int64(2), "ACTIVE"}},
		{Op: "exec", SQL: "UPDATE auth SET version = $1, updated_at = $2 WHERE id = $3 AND version = $4", Args: []driver.Value{int64(8), at, int64(7), int64(7)}},
	}, func(ctx context.Context, tx domain.Transaction) {
		w := writer.NewUserRoleAssignmentWriter(tx)
		for _, state := range []domain.UserRoleAssignmentItem{{RoleID: 2}, {ID: 11, RoleID: 2, Status: "REVOKED"}} {
			ok, e := w.Activate(ctx, 7, state, 42, at)
			if e != nil || !ok {
				t.Fatal(ok, e)
			}
		}
		ok, e := w.Revoke(ctx, 7, domain.UserRoleAssignmentItem{ID: 11, RoleID: 2, Status: "ACTIVE"}, 42, "Review access", at)
		if e != nil || !ok {
			t.Fatal(ok, e)
		}
		ok, e = w.AdvanceVersion(ctx, 7, 7, at)
		if e != nil || !ok {
			t.Fatal(ok, e)
		}
	})
}

func assignmentFailures(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	for _, kind := range []string{"zero", "many", "rows error", "unique", "FK", "dependency"} {
		t.Run(kind, func(t *testing.T) {
			step := mocks.SQLStep{Op: "exec", SQL: "INSERT INTO user_role", Args: []driver.Value{int64(7), int64(2), "ACTIVE", int64(42), at}}
			var want error
			switch kind {
			case "zero":
				step.Result = driver.RowsAffected(0)
			case "many":
				step.Result = driver.RowsAffected(2)
				want = common.ErrDependencyUnavailable
			case "rows error":
				want = errors.New("rows failed")
				step.Result = badResult{want}
			case "unique":
				step.Error = &pq.Error{Code: "23505"}
				want = step.Error
			case "FK":
				step.Error = &pq.Error{Code: "23503"}
				want = step.Error
			case "dependency":
				step.Error = errors.New("private persistence failure")
				want = step.Error
			}
			runSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
				ok, e := writer.NewUserRoleAssignmentWriter(tx).Activate(ctx, 7, domain.UserRoleAssignmentItem{RoleID: 2}, 42, at)
				if ok || !errors.Is(e, want) {
					t.Fatal(ok, e, want)
				}
				if step.Error != nil && !errors.Is(e, step.Error) {
					t.Fatal("cause lost", e)
				}
			})
		})
	}
	r := reader.NewUserRoleAssignmentReader(nil)
	w := writer.NewUserRoleAssignmentWriter(nil)
	ctx := context.Background()
	if _, e := r.AssignmentRoles(ctx, 7, []uint64{2}); !errors.Is(e, common.ErrDependencyUnavailable) {
		t.Fatal(e)
	}
	if _, e := r.ListAssignments(ctx, domain.CatalogQuery{TargetUserID: 7}); !errors.Is(e, common.ErrDependencyUnavailable) {
		t.Fatal(e)
	}
	if _, e := w.Activate(ctx, 7, domain.UserRoleAssignmentItem{RoleID: 2}, 42, at); !errors.Is(e, common.ErrDependencyUnavailable) {
		t.Fatal(e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := r.AssignmentRoles(canceled, 7, []uint64{2}); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := w.Revoke(canceled, 7, domain.UserRoleAssignmentItem{}, 42, "Review access", at); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	// Read failures return no partial page after total or page loading.
	for _, stage := range []string{"count", "page"} {
		cause := errors.New("read failed")
		steps := []mocks.SQLStep{{Op: "query", SQL: "COUNT(DISTINCT ur.id)", Args: []driver.Value{int64(7)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(2)}}}}
		if stage == "count" {
			steps[0].Error = cause
		} else {
			steps = append(steps, mocks.SQLStep{Op: "query", SQL: "FROM user_role ur", Args: []driver.Value{int64(7)}, Error: cause})
		}
		runSQL(t, steps, func(ctx context.Context, tx domain.Transaction) {
			page, e := reader.NewUserRoleAssignmentReader(tx).ListAssignments(ctx, domain.CatalogQuery{TargetUserID: 7})
			if !errors.Is(e, cause) || !reflect.DeepEqual(page, orm.PageData[domain.UserRoleAssignmentItem]{}) {
				t.Fatal(page, e)
			}
		})
	}
}

type badResult struct{ err error }

func (b badResult) LastInsertId() (int64, error) { return 0, b.err }
func (b badResult) RowsAffected() (int64, error) { return 0, b.err }
