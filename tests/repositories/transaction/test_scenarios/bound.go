package test_scenarios

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/lib/pq"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	roleMock "github.com/siti-nabila/grpc-auth/tests/repositories/transaction/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"testing"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "provider and repositories forward one request context on one transaction", Run: bound}, {Name: "bound paths reject nil handles and preserve read or begin failure", Run: failures}, {Name: "bound writes preserve constraint and dependency causes without lifecycle calls", Run: writeFailures}, {Name: "legacy role writer requires a supplied immutable transaction", Run: roleBoundary}}
}

func writeFailures(t *testing.T) {
	for _, name := range []string{"auth duplicate", "profile dependency", "assignment dependency"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			cause := errors.New("write failed")
			step := mocks.SQLStep{Op: "exec", SQL: "UPDATE profile", Args: []driver.Value{"", "", "", int64(1)}, Error: cause}
			if name == "auth duplicate" {
				step = mocks.SQLStep{Op: "query", SQL: "INSERT INTO auth", Args: []driver.Value{"x@example.test", "encoded"}, Error: &pq.Error{Code: "23505"}}
			}
			if name == "assignment dependency" {
				step = mocks.SQLStep{Op: "query", SQL: "INSERT INTO user_role", Args: []driver.Value{int64(1), int64(1)}, Error: cause}
			}
			mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, step, {Op: "rollback"}}}
			db := mock.Open()
			defer db.Close()
			tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			repos := authfeature.BindRegisterRepositories(tx)
			switch name {
			case "auth duplicate":
				err = repos.AuthWriter.Create(&domain.AuthRequest{Email: "x@example.test", Password: "encoded"})
			case "profile dependency":
				err = repos.ProfileWriter.Update(&domain.UpdateProfileRequest{Id: 1})
			case "assignment dependency":
				err = repos.UserRoleWriter.Create(&domain.UserRoleRequest{UserId: 1, RoleId: 1})
			}
			want := cause
			if name == "auth duplicate" {
				want = auth.ErrDataExists
			}
			if !errors.Is(err, want) {
				t.Fatalf("mapped cause lost %v", err)
			}
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err = mock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func bound(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	driverMock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{
		{Op: "begin"},
		{Op: "query", SQL: "FROM auth", Args: []driver.Value{"user@example.test"}, Columns: []string{"id", "email", "password"}, Rows: [][]driver.Value{{int64(42), "user@example.test", "encoded"}}},
		{Op: "query", SQL: "FROM role", Args: []driver.Value{int64(1)}, Columns: []string{"id", "role_code", "role_name"}, Rows: [][]driver.Value{{int64(7), int64(1), "member"}}},
		{Op: "query", SQL: "FROM profile", Args: []driver.Value{int64(42)}, Columns: []string{"id", "user_id", "name", "address", "phone"}, Rows: [][]driver.Value{{int64(8), int64(42), "old", "old", "old"}}},
		{Op: "exec", SQL: "UPDATE profile SET name = $1, address = $2, phone = $3 WHERE id = $4", Args: []driver.Value{"new", "", "", int64(8)}},
		{Op: "query", SQL: "INSERT INTO auth", Args: []driver.Value{"new@example.test", "encoded"}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(43)}}},
		{Op: "query", SQL: "INSERT INTO profile", Args: []driver.Value{int64(43), "", "", ""}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(9)}}},
		{Op: "query", SQL: "INSERT INTO user_role", Args: []driver.Value{int64(43), int64(7)}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(10)}}},
		{Op: "commit"},
		{Op: "begin"}, {Op: "rollback"},
	}}
	db := driverMock.Open()
	defer db.Close()
	provider := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
	tx, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if driverMock.Options.Isolation != driver.IsolationLevel(sql.LevelReadCommitted) {
		t.Fatal("wrong isolation")
	}
	registration := authfeature.BindRegisterRepositories(tx)
	ownProfile := profile.BindRepositories(tx)
	a, err := registration.AuthReader.GetByEmail("user@example.test")
	if err != nil || a.Id != 42 {
		t.Fatalf("auth %+v err %v", a, err)
	}
	r, err := registration.RoleReader.GetDefaultRole()
	if err != nil || r.Id != 7 {
		t.Fatalf("role %+v err %v", r, err)
	}
	p, err := ownProfile.Reader.GetByUserId(42)
	if err != nil || p.Id != 8 || p.UserId != 42 {
		t.Fatalf("profile %+v err %v", p, err)
	}
	if err = ownProfile.Writer.Update(&domain.UpdateProfileRequest{Id: p.Id, Name: "new"}); err != nil {
		t.Fatal(err)
	}
	request := &domain.AuthRequest{Email: "new@example.test", Password: "encoded"}
	if err = registration.AuthWriter.Create(request); err != nil || request.Id != 43 {
		t.Fatalf("auth create id %d err %v", request.Id, err)
	}
	if err = registration.ProfileWriter.Create(&domain.ProfileRequest{UserId: 43}); err != nil {
		t.Fatal(err)
	}
	if err = registration.UserRoleWriter.Create(&domain.UserRoleRequest{UserId: 43, RoleId: 7}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	next, err := provider.Begin(ctx)
	if err != nil || next == tx {
		t.Fatal("new request reused closed handle")
	}
	if err = next.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = driverMock.Verify(); err != nil {
		t.Fatal(err)
	}
}

func failures(t *testing.T) {
	registration := authfeature.BindRegisterRepositories(nil)
	ownProfile := profile.BindRepositories(nil)
	_, err := registration.AuthReader.GetByEmail("x")
	if !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("nil reader fell back")
	}
	_, err = registration.RoleReader.GetDefaultRole()
	if !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("nil role reader fell back")
	}
	_, err = ownProfile.Reader.GetByUserId(1)
	if !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("nil profile reader fell back")
	}
	for _, err := range []error{registration.AuthWriter.Create(&domain.AuthRequest{}), registration.ProfileWriter.Create(&domain.ProfileRequest{}), registration.UserRoleWriter.Create(&domain.UserRoleRequest{}), ownProfile.Writer.Update(&domain.UpdateProfileRequest{})} {
		if !errors.Is(err, common.ErrInternalServerError) {
			t.Fatal("nil writer fell back")
		}
	}
	for _, name := range []string{"begin", "missing", "dependency", "cancel"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("database failure")
			steps := []mocks.SQLStep{{Op: "begin"}, {Op: "query", SQL: "FROM auth", Args: []driver.Value{"missing@example.test"}, Columns: []string{"id"}}, {Op: "rollback"}}
			if name == "begin" {
				steps = []mocks.SQLStep{{Op: "begin", Error: failure}}
			}
			if name == "dependency" {
				steps[1].Error = failure
			}
			if name == "cancel" {
				steps = nil
				cancel()
			}
			driverMock := &mocks.SQL{Context: ctx, Steps: steps}
			db := driverMock.Open()
			defer db.Close()
			provider := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
			tx, err := provider.Begin(ctx)
			if name == "begin" {
				if !errors.Is(err, failure) || tx != nil {
					t.Fatal("begin failure lost")
				}
			} else if name == "cancel" {
				if !errors.Is(err, context.Canceled) || tx != nil {
					t.Fatal("canceled begin proceeded")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				repos := authfeature.BindRegisterRepositories(tx)
				_, err = repos.AuthReader.GetByEmail("missing@example.test")
				if name == "missing" && !errors.Is(err, auth.ErrNotFound) {
					t.Fatalf("missing mapping %v", err)
				}
				if name == "dependency" && !errors.Is(err, failure) {
					t.Fatalf("cause lost %v", err)
				}
				if err = tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			}
			if err = driverMock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func roleBoundary(t *testing.T) {
	if err := writer.NewRoleWriterInTransaction(nil).Create(&domain.RoleRequest{}); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("nil role handle fell back")
	}
	for _, cause := range []error{nil, errors.New("controlled role write failure")} {
		tx := &roleMock.RoleTransaction{Error: cause}
		req := &domain.RoleRequest{Name: "fixture"}
		err := writer.NewRoleWriterInTransaction(tx).Create(req)
		if !errors.Is(err, cause) || tx.Request != req || tx.Calls != 1 {
			t.Fatal("role delegation/cause differs")
		}
	}
}
