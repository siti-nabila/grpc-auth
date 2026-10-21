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

var at = time.Unix(1735689600, 0).UTC()
var columns = []string{"application_id", "application_code", "name", "application_status", "is_system", "status", "granted_by", "granted_at", "revoked_by", "revoked_at", "revoke_reason"}

func row() []driver.Value {
	return []driver.Value{int64(2), "sales", "Sales", "DISABLED", false, "REVOKED", nil, at, nil, at, "Review access"}
}

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "bound optional access preserves absence and rejects malformed owner or provenance", Run: accessReads},
		{Name: "all application assignments are read in role order including more than20", Run: assignmentReads},
		{Name: "eleven scalar sorts count projection literal search and continuation share safe query stages", Run: accessLists},
		{Name: "pair writes keep identity and conditional provenance with unchanged version ownership", Run: accessWrites},
		{Name: "conditional dependency FK uniqueness context and partial read failures remain errors", Run: failures},
	}
}

func runSQL(t *testing.T, steps []mocks.SQLStep, act func(context.Context, domain.Transaction)) {
	t.Helper()
	ctx := context.WithValue(context.Background(), struct{}{}, "access-request")
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
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = mock.Verify(); err != nil {
		t.Fatal(err)
	}
}

func accessReads(t *testing.T) {
	for _, name := range []string{"absent", "revoked", "active", "foreign owner", "foreign app", "missing grant time", "missing revoke proof", "multiple"} {
		t.Run(name, func(t *testing.T) {
			values := append([]driver.Value{int64(7)}, row()...)
			rows := [][]driver.Value{values}
			switch name {
			case "absent":
				rows = nil
			case "active":
				values[6] = "ACTIVE"
				values[10] = nil
				values[11] = nil
			case "foreign owner":
				values[0] = int64(8)
			case "foreign app":
				values[1] = int64(3)
			case "missing grant time":
				values[8] = nil
			case "missing revoke proof":
				values[10] = nil
			case "multiple":
				rows = append(rows, values)
			}
			step := mocks.SQLStep{Op: "query", SQL: "access.user_id = $1 AND access.application_id = $2", Args: []driver.Value{int64(7), int64(2)}, Columns: append([]string{"user_id"}, columns...), Rows: rows}
			runSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
				got, err := reader.NewApplicationAccessReader(tx).Access(ctx, 7, 2)
				switch name {
				case "absent":
					if err != nil || got != nil {
						t.Fatal(got, err)
					}
				case "active", "revoked":
					if err != nil || got == nil || got.UserID != 7 || got.ApplicationID != 2 || got.GrantedBy != nil || got.GrantedAt == nil {
						t.Fatal(got, err)
					}
				default:
					if got != nil || !errors.Is(err, common.ErrDependencyUnavailable) {
						t.Fatal(got, err)
					}
				}
			})
		})
	}
}

func assignmentReads(t *testing.T) {
	cols := []string{"id", "role_id", "code", "role_code", "name", "status", "role_status", "application_id", "application_code", "application_status", "is_system", "application_access_status", "granted_by", "granted_at", "revoked_by", "revoked_at", "revoke_reason"}
	var rows [][]driver.Value
	for i := int64(1); i <= 25; i++ {
		rows = append(rows, []driver.Value{i, i, "sales.role", i, "Role", "ACTIVE", "DISABLED", int64(2), "sales", "DISABLED", false, nil, nil, nil, nil, nil, nil})
	}
	runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "ur.user_id = $1 AND r.application_id = $2 AND ur.status = $3", Args: []driver.Value{int64(7), int64(2), "ACTIVE"}, Columns: cols, Rows: rows}}, func(ctx context.Context, tx domain.Transaction) {
		got, err := reader.NewApplicationAccessReader(tx).ApplicationAssignments(ctx, 7, 2)
		if err != nil || len(got) != 25 || got[0].RoleID != 1 || got[24].RoleID != 25 || got[0].ApplicationAccessStatus != nil {
			t.Fatal(got, err)
		}
	})
	for _, name := range []string{"foreign application", "revoked row", "duplicate", "dependency"} {
		t.Run(name, func(t *testing.T) {
			values := append([]driver.Value(nil), rows[0]...)
			step := mocks.SQLStep{Op: "query", SQL: "ORDER BY r.id ASC", Args: []driver.Value{int64(7), int64(2), "ACTIVE"}, Columns: cols, Rows: [][]driver.Value{values}}
			want := error(common.ErrDependencyUnavailable)
			switch name {
			case "foreign application":
				values[7] = int64(3)
			case "revoked row":
				values[5] = "REVOKED"
			case "duplicate":
				step.Rows = append(step.Rows, values)
			case "dependency":
				want = errors.New("assignment read cause")
				step.Error = want
			}
			runSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
				got, err := reader.NewApplicationAccessReader(tx).ApplicationAssignments(ctx, 7, 2)
				if got != nil || !errors.Is(err, want) {
					t.Fatal(got, err)
				}
			})
		})
	}
}

func accessLists(t *testing.T) {
	expressions := map[string]string{"application_id": "a.id", "application_code": `a.code COLLATE "C"`, "name": `a.name COLLATE "C"`, "application_status": `a.status COLLATE "C"`, "is_system": "a.is_system", "status": `access.status COLLATE "C"`, "granted_by": "access.granted_by", "granted_at": "access.granted_at", "revoked_by": "access.revoked_by", "revoked_at": "access.revoked_at", "revoke_reason": `access.revoke_reason COLLATE "C"`}
	for _, field := range domain.UserApplicationAccessCatalog.Fields() {
		for _, desc := range []bool{false, true} {
			t.Run(field.Name+map[bool]string{true: " DESC", false: " ASC"}[desc], func(t *testing.T) {
				q := domain.CatalogQuery{TargetUserID: 7, Fields: []string{"name"}, Sort: []orm.SortField{{Field: field.Name, Desc: desc}}}
				order := expressions[field.Name] + map[bool]string{true: " DESC", false: " ASC"}[desc] + " NULLS LAST"
				runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "COUNT(DISTINCT a.id) AS total", Args: []driver.Value{int64(7)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(1)}}}, {Op: "query", SQL: order, Args: []driver.Value{int64(7)}, Columns: columns, Rows: [][]driver.Value{row()}}}, func(ctx context.Context, tx domain.Transaction) {
					page, err := reader.NewApplicationAccessReader(tx).ListAccess(ctx, q)
					if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ApplicationID != 2 || page.Items[0].Name != "Sales" || page.Items[0].Status != "" || page.Items[0].GrantedAt != nil || page.Items[0].UserID != 0 {
						t.Fatal(page, err)
					}
				})
			})
		}
	}
	q := domain.CatalogQuery{TargetUserID: 7, ApplicationCode: "sales", Statuses: []string{"REVOKED", "ACTIVE"}, Search: &orm.SearchQuery{Keyword: "50%_"}, Limit: 1}
	args := []driver.Value{int64(7), "sales", "ACTIVE", "REVOKED", `%50\%\_%`, `%50\%\_%`}
	a, b := row(), row()
	b[0] = int64(3)
	runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "access.status IN ($3, $4)", Args: args, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(2)}}}, {Op: "query", SQL: `a.code ILIKE $5 ESCAPE '\'`, Args: args, Columns: columns, Rows: [][]driver.Value{a, b}}}, func(ctx context.Context, tx domain.Transaction) {
		page, err := reader.NewApplicationAccessReader(tx).ListAccess(ctx, q)
		if err != nil || page.Total != 2 || !page.HasNext || len(page.Items) != 1 || page.NextCursor == "" {
			t.Fatal(page, err)
		}
		n, _ := domain.UserApplicationAccessCatalog.Normalize(q)
		n.LastID = page.NextCursor
		values, err := domain.UserApplicationAccessCatalog.ParseCursor(n)
		if err != nil || len(values) != 1 || *values[0].Value != "2" {
			t.Fatal(values, err)
		}
	})
	for _, name := range []string{"numeric terminal", "offset empty"} {
		t.Run(name, func(t *testing.T) {
			q := domain.CatalogQuery{TargetUserID: 7}
			sql := "a.id > $2"
			args := []driver.Value{int64(7), int64(2)}
			total := int64(2)
			if name == "numeric terminal" {
				q.LastID = "2"
			} else {
				q.Page = 2
				sql = "OFFSET 20"
				args = args[:1]
				total = 0
			}
			runSQL(t, []mocks.SQLStep{{Op: "query", SQL: "COUNT(DISTINCT a.id)", Args: []driver.Value{int64(7)}, Columns: []string{"total"}, Rows: [][]driver.Value{{total}}}, {Op: "query", SQL: sql, Args: args, Columns: columns}}, func(ctx context.Context, tx domain.Transaction) {
				page, err := reader.NewApplicationAccessReader(tx).ListAccess(ctx, q)
				if err != nil || page.Total != total || len(page.Items) != 0 || page.NextCursor != "" {
					t.Fatal(page, err)
				}
			})
		})
	}
}

func accessWrites(t *testing.T) {
	runSQL(t, []mocks.SQLStep{
		{Op: "exec", SQL: "INSERT INTO user_application_access", Args: []driver.Value{int64(7), int64(2), "ACTIVE", int64(42), at}},
		{Op: "exec", SQL: "revoked_by = $4, revoked_at = $5, revoke_reason = $6 WHERE user_id = $7 AND application_id = $8 AND status = $9", Args: []driver.Value{"ACTIVE", int64(42), at, nil, nil, nil, int64(7), int64(2), "REVOKED"}},
		{Op: "exec", SQL: "SET status = $1, revoked_by = $2, revoked_at = $3, revoke_reason = $4 WHERE user_id = $5 AND application_id = $6 AND status = $7", Args: []driver.Value{"REVOKED", int64(42), at, "Review access", int64(7), int64(2), "ACTIVE"}},
	}, func(ctx context.Context, tx domain.Transaction) {
		w := writer.NewApplicationAccessWriter(tx)
		for _, old := range []*domain.ApplicationAccessItem{nil, {UserID: 7, ApplicationID: 2, Status: "REVOKED"}} {
			if ok, err := w.Activate(ctx, 7, 2, old, 42, at); err != nil || !ok {
				t.Fatal(ok, err)
			}
		}
		if ok, err := w.Revoke(ctx, 7, domain.ApplicationAccessItem{UserID: 7, ApplicationID: 2, Status: "ACTIVE"}, 42, "Review access", at); err != nil || !ok {
			t.Fatal(ok, err)
		}
	})
	runSQL(t, nil, func(ctx context.Context, tx domain.Transaction) {
		w := writer.NewApplicationAccessWriter(tx)
		old := domain.ApplicationAccessItem{UserID: 7, ApplicationID: 2, Status: "REVOKED"}
		if ok, err := w.Revoke(ctx, 7, old, 42, "Review access", at); ok || !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("revoked row rewritten", ok, err)
		}
		old.Status = "ACTIVE"
		if ok, err := w.Activate(ctx, 7, 2, &old, 42, at); ok || !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("active row rewritten", ok, err)
		}
	})
}

func failures(t *testing.T) {
	for _, name := range []string{"zero", "multiple", "affected error", "unique", "FK", "dependency"} {
		t.Run(name, func(t *testing.T) {
			step := mocks.SQLStep{Op: "exec", SQL: "INSERT INTO user_application_access", Args: []driver.Value{int64(7), int64(2), "ACTIVE", int64(42), at}}
			var want error
			switch name {
			case "zero":
				step.Result = driver.RowsAffected(0)
			case "multiple":
				step.Result = driver.RowsAffected(2)
				want = common.ErrDependencyUnavailable
			case "affected error":
				want = errors.New("affected rows error")
				step.Result = failedResult{want}
			case "unique":
				step.Error = &pq.Error{Code: "23505"}
				want = step.Error
			case "FK":
				step.Error = &pq.Error{Code: "23503"}
				want = step.Error
			case "dependency":
				step.Error = errors.New("private DB cause")
				want = step.Error
			}
			runSQL(t, []mocks.SQLStep{step}, func(ctx context.Context, tx domain.Transaction) {
				ok, err := writer.NewApplicationAccessWriter(tx).Activate(ctx, 7, 2, nil, 42, at)
				if ok || !errors.Is(err, want) {
					t.Fatal(ok, err, want)
				}
			})
		})
	}
	for _, stage := range []string{"count", "page", "malformed"} {
		t.Run(stage, func(t *testing.T) {
			cause := errors.New("controlled read failure")
			steps := []mocks.SQLStep{{Op: "query", SQL: "COUNT(DISTINCT a.id)", Args: []driver.Value{int64(7)}, Columns: []string{"total"}, Rows: [][]driver.Value{{int64(1)}}}}
			if stage == "count" {
				steps[0].Error = cause
			} else {
				step := mocks.SQLStep{Op: "query", SQL: "access.user_id = $1", Args: []driver.Value{int64(7)}, Columns: columns, Error: cause}
				if stage == "malformed" {
					bad := row()
					bad[7] = nil
					step.Error = nil
					step.Rows = [][]driver.Value{bad}
					cause = common.ErrDependencyUnavailable
				}
				steps = append(steps, step)
			}
			runSQL(t, steps, func(ctx context.Context, tx domain.Transaction) {
				page, err := reader.NewApplicationAccessReader(tx).ListAccess(ctx, domain.CatalogQuery{TargetUserID: 7})
				if !errors.Is(err, cause) || !reflect.DeepEqual(page, orm.PageData[domain.ApplicationAccessItem]{}) {
					t.Fatal(page, err)
				}
			})
		})
	}
	ctx := context.Background()
	r := reader.NewApplicationAccessReader(nil)
	w := writer.NewApplicationAccessWriter(nil)
	if _, err := r.Access(ctx, 7, 2); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	if _, err := w.Activate(ctx, 7, 2, nil, 42, at); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.ApplicationAssignments(canceled, 7, 2); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := w.Revoke(canceled, 7, domain.ApplicationAccessItem{}, 42, "Review access", at); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type failedResult struct{ err error }

func (r failedResult) LastInsertId() (int64, error) { return 0, r.err }
func (r failedResult) RowsAffected() (int64, error) { return 0, r.err }
