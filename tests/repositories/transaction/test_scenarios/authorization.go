package test_scenarios

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func Authorization() []testutils.Scenario {
	return []testutils.Scenario{{Name: "permission reader binds exact owner and joins access on read-only snapshot", Run: permissionRead}}
}

func permissionRead(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "snapshot")
	columns := []string{"permission_code", "permission_application_id", "role_id", "role_code", "role_application_id", "role_status", "assignment_status", "application_code", "application_status", "access_status"}
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, {Op: "query", SQL: "LEFT JOIN user_application_access access ON access.user_id = ur.user_id AND access.application_id = app.id", Args: []driver.Value{int64(42)}, Columns: columns, Rows: [][]driver.Value{{"iam.user.read", int64(1), int64(2), "iam.admin", int64(1), "ACTIVE", "ACTIVE", "iam", "ACTIVE", nil}}}, {Op: "commit"}}}
	db := mock.Open()
	defer db.Close()
	tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).BeginSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.NewPermissionReader(tx).Permissions(ctx, 42)
	if err != nil || len(got) != 1 || got[0].PermissionCode != "iam.user.read" || got[0].RoleID != 2 || got[0].AccessStatus.Valid {
		t.Fatal("permission snapshot mapping", got, err)
	}
	if mock.Options.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) || !mock.Options.ReadOnly {
		t.Fatal("read snapshot is not RepeatableRead READ ONLY")
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}
