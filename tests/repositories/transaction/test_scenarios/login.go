package test_scenarios

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func LoginPersistence() []testutils.Scenario {
	return []testutils.Scenario{{Name: "initial refresh persists digest and NULL state on supplied transaction", Run: refreshWrites}, {Name: "candidate and bound Login credential reads retain authoritative fields and context", Run: loginReads}}
}

func refreshWrites(t *testing.T) {
	r := domain.InitialRefresh{ID: fixtures.AccessTokenID, SessionID: fixtures.SessionID, FamilyID: "00000000-0000-4000-8000-000000000003", TokenHash: bytes.Repeat([]byte{1}, 32), IssuedAt: fixtures.ServiceTime, ExpiresAt: fixtures.ServiceTime.Add(7 * 24 * time.Hour)}
	for _, fails := range []bool{false, true} {
		ctx := context.WithValue(context.Background(), struct{}{}, "refresh-context")
		cause := errors.New("controlled refresh insert failure")
		step := mocks.SQLStep{Op: "query", SQL: "INSERT INTO refresh_tokens", Args: []driver.Value{r.ID, r.SessionID, r.TokenHash, r.FamilyID, r.IssuedAt, r.ExpiresAt, nil, nil, nil}, Columns: []string{"id"}, Rows: [][]driver.Value{{r.ID}}}
		if fails {
			step.Error = cause
		}
		mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, step, {Op: "rollback"}}}
		db := mock.Open()
		tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = writer.NewInitialRefreshWriter(tx).Create(r)
		if fails && !errors.Is(err, cause) || !fails && err != nil {
			t.Fatal("refresh write outcome", err)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err := mock.Verify(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	for _, change := range []func(*domain.InitialRefresh){func(r *domain.InitialRefresh) { r.ID = "bad" }, func(r *domain.InitialRefresh) { r.SessionID = "bad" }, func(r *domain.InitialRefresh) { r.FamilyID = "bad" }, func(r *domain.InitialRefresh) { r.TokenHash = []byte("raw-credential") }, func(r *domain.InitialRefresh) { r.ConsumedAt = sql.NullTime{Valid: true, Time: r.IssuedAt} }, func(r *domain.InitialRefresh) { r.RevokedAt = sql.NullTime{Valid: true, Time: r.IssuedAt} }, func(r *domain.InitialRefresh) { r.ReplacedBy = sql.NullString{Valid: true, String: fixtures.SessionID} }, func(r *domain.InitialRefresh) { r.ExpiresAt = r.IssuedAt }, func(r *domain.InitialRefresh) { r.ExpiresAt = r.IssuedAt.Add(7*24*time.Hour + time.Second) }} {
		x := r
		change(&x)
		if !errors.Is(x.Validate(), common.ErrBadRequest) {
			t.Fatal("invalid initial state accepted")
		}
	}
	if !errors.Is(writer.NewInitialRefreshWriter(nil).Create(r), common.ErrInternalServerError) {
		t.Fatal("missing tx fallback")
	}
}

func loginReads(t *testing.T) {
	cols := []string{"id", "email", "password", "status", "auth_version", "version", "must_change_password", "deleted_at"}
	row := []driver.Value{int64(42), "member@example.com", "test-only-hash", "ACTIVE", int64(3), int64(4), true, nil}
	for _, bound := range []bool{false, true} {
		for _, kind := range []string{"success", "missing", "failure", "cancelled"} {
			ctx := context.WithValue(context.Background(), struct{}{}, "login-context")
			cause := errors.New("controlled credential reader failure")
			arg := driver.Value("member@example.com")
			query := "email ="
			if bound {
				arg = int64(42)
				query = "id ="
			}
			step := mocks.SQLStep{Op: "query", SQL: query, Args: []driver.Value{arg}, Columns: cols, Rows: [][]driver.Value{row}}
			if kind == "missing" {
				step.Rows = nil
			}
			if kind == "failure" {
				step.Error = cause
			}
			if kind == "cancelled" {
				cause = context.Canceled
				step.Error = cause
			}
			steps := []mocks.SQLStep{step}
			if bound {
				steps = append([]mocks.SQLStep{{Op: "begin"}}, steps...)
				steps = append(steps, mocks.SQLStep{Op: "rollback"})
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			var handle domain.QueryReader = orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), database.GetORMConfig())
			var tx domain.Transaction
			if bound {
				var err error
				tx, err = database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				handle = tx
			}
			r := reader.NewLoginAccountReader(handle)
			var got domain.LoginAccount
			var err error
			if bound {
				got, err = r.GetByID(ctx, 42)
			} else {
				got, err = r.GetByEmail(ctx, " MEMBER@example.com ")
			}
			switch kind {
			case "success":
				if err != nil || got.ID != 42 || got.Password != "test-only-hash" || got.AuthVersion != 3 || got.Version != 4 || !got.MustChangePassword || got.DeletedAt.Valid {
					t.Fatal("credential fields lost", err)
				}
			case "missing":
				if !errors.Is(err, auth.ErrNotFound) {
					t.Fatal("not found mapping", err)
				}
			default:
				if !errors.Is(err, cause) {
					t.Fatal("cause lost", err)
				}
			}
			if bound {
				if err := tx.Rollback(); err != nil {
					t.Fatal(err)
				}
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
			db.Close()
		}
	}
	if _, err := reader.NewLoginAccountReader(nil).GetByID(context.Background(), 42); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing handle fallback")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.NewLoginAccountReader(nil).GetByEmail(ctx, "member@example.com"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost")
	}
}
