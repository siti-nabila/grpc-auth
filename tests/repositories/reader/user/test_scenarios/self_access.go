package test_scenarios

import (
	"context"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func SelfAccess() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "self roles bind user and active gates with IAM access exception", Run: selfRoleQuery},
		{Name: "self applications bind owner and active access independent of role mappings", Run: selfApplicationQuery},
		{Name: "self identity reads creation time and retains context", Run: selfIdentityQuery},
		{Name: "self access refuses nil handle canceled context and dependency errors", Run: selfAccessErrors},
	}
}

func selfRoleQuery(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "query", SQL: "(a.code = $5 OR access.status = $6)", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE", "ACTIVE", "iam", "ACTIVE"}, Columns: []string{"role_id", "code", "name", "status", "role_status", "application_code"}, Rows: [][]driver.Value{{int64(4), "clm.author", "Author", "ACTIVE", "ACTIVE", "clm"}}}}}
	db := mock.Open()
	defer db.Close()
	r := reader.NewSelfAccessReader(orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), database.GetORMConfig()))
	rows, err := r.Roles(ctx, 42)
	if err != nil || len(rows) != 1 || rows[0].ID != 4 || rows[0].ApplicationCode != "clm" {
		t.Fatal(rows, err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}

func selfApplicationQuery(t *testing.T) {
	ctx := context.Background()
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "query", SQL: "ORDER BY a.id ASC", Args: []driver.Value{int64(42), "ACTIVE", "ACTIVE"}, Columns: []string{"application_id", "code", "name", "status", "application_status"}, Rows: [][]driver.Value{{int64(2), "clm", "Claims", "ACTIVE", "ACTIVE"}}}}}
	db := mock.Open()
	defer db.Close()
	r := reader.NewSelfAccessReader(orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), database.GetORMConfig()))
	rows, err := r.Applications(ctx, 42)
	if err != nil || len(rows) != 1 || rows[0].ID != 2 || rows[0].Code != "clm" {
		t.Fatal(rows, err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}

func selfIdentityQuery(t *testing.T) {
	ctx := context.Background()
	created := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "query", SQL: "must_change_password, created_at FROM auth", Args: []driver.Value{int64(42)}, Columns: []string{"id", "email", "status", "version", "must_change_password", "created_at"}, Rows: [][]driver.Value{{int64(42), "member@example.test", "ACTIVE", int64(7), false, created}}}}}
	db := mock.Open()
	defer db.Close()
	r := reader.NewUserIdentityReader(orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), database.GetORMConfig()))
	row, err := r.Identity(ctx, 42)
	if err != nil || row.ID != 42 || !row.CreatedAt.Equal(created) {
		t.Fatal(row, err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}

func selfAccessErrors(t *testing.T) {
	r := reader.NewSelfAccessReader(nil)
	if _, err := r.Roles(context.Background(), 42); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("nil self role handle", err)
	}
	if _, err := r.Applications(context.Background(), 42); !errors.Is(err, common.ErrDependencyUnavailable) {
		t.Fatal("nil self application handle", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Roles(ctx, 42); !errors.Is(err, context.Canceled) {
		t.Fatal("role context lost", err)
	}
	if _, err := r.Applications(ctx, 42); !errors.Is(err, context.Canceled) {
		t.Fatal("application context lost", err)
	}
}
