package test_scenarios

import (
	"context"
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

func Session() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "session state reads primary snapshot or supplied tx with request context", Run: sessionReads},
		{Name: "session writer uses supplied transaction and preserves failures", Run: sessionWrites},
	}
}

func sessionReads(t *testing.T) {
	columns := []string{"user_id", "user_status", "auth_version", "user_version", "must_change_password", "deleted_at", "session_id", "session_user_id", "session_status", "created_at", "expires_at", "revoked_at"}
	row := []driver.Value{int64(42), "ACTIVE", int64(3), int64(4), true, nil, fixtures.SessionID, int64(42), "ACTIVE", fixtures.ServiceTime, fixtures.ServiceTime.Add(7 * 24 * time.Hour), nil}
	for _, bound := range []bool{false, true} {
		for _, kind := range []string{"success", "missing", "failure", "cancelled"} {
			t.Run(kind+map[bool]string{true: " tx", false: " primary"}[bound], func(t *testing.T) {
				ctx := context.WithValue(context.Background(), struct{}{}, "session-context")
				cause := errors.New("controlled session read failure")
				step := mocks.SQLStep{Op: "query", SQL: "JOIN auth a ON a.id = s.user_id", Args: []driver.Value{fixtures.SessionID}, Columns: columns, Rows: [][]driver.Value{row}}
				if kind == "missing" {
					step.Rows = nil
				}
				if kind == "failure" {
					step.Error = cause
				}
				steps := []mocks.SQLStep{step}
				if bound {
					steps = append([]mocks.SQLStep{{Op: "begin"}}, steps...)
					steps = append(steps, mocks.SQLStep{Op: "rollback"})
				}
				mock := &mocks.SQL{Context: ctx, Steps: steps}
				db := mock.Open()
				defer db.Close()
				adapter := orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), database.GetORMConfig())
				var handle domain.QueryReader = adapter
				var tx domain.Transaction
				if bound {
					var err error
					tx, err = database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
					if err != nil {
						t.Fatal(err)
					}
					handle = tx
				}
				if kind == "cancelled" {
					cause = context.Canceled
					mock.Steps[map[bool]int{false: 0, true: 1}[bound]].Error = cause
				}
				got, err := reader.NewSessionStateReader(handle).GetSessionState(ctx, fixtures.SessionID)
				switch kind {
				case "success":
					if err != nil || got.UserStatus != "ACTIVE" || got.SessionStatus != "ACTIVE" || got.UserID != 42 || got.AuthVersion != 3 || got.UserVersion != 4 || !got.MustChangePassword || got.SessionID != fixtures.SessionID || got.SessionUserID != 42 || !got.ExpiresAt.Equal(fixtures.ServiceTime.Add(7*24*time.Hour)) {
						t.Fatal("session fields not mapped", got, err)
					}
				case "missing":
					if !errors.Is(err, auth.ErrNotFound) {
						t.Fatal("missing session mapping", err)
					}
				default:
					if !errors.Is(err, cause) {
						t.Fatal("session error cause lost", err)
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
			})
		}
	}
	if _, err := reader.NewSessionStateReader(nil).GetSessionState(context.Background(), fixtures.SessionID); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing handle used pool fallback")
	}
}

func sessionWrites(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{true: "failure", false: "success"}[fails], func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "session-context")
			session := domain.Session{ID: fixtures.SessionID, UserID: 42, Status: "ACTIVE", CreatedAt: fixtures.ServiceTime, LastSeenAt: fixtures.ServiceTime, ExpiresAt: fixtures.ServiceTime.Add(7 * 24 * time.Hour)}
			cause := errors.New("controlled session insert failure")
			step := mocks.SQLStep{Op: "query", SQL: "INSERT INTO sessions", Args: []driver.Value{session.ID, int64(42), "ACTIVE", session.CreatedAt, session.LastSeenAt, session.ExpiresAt, nil, nil, nil, nil, nil, nil}, Columns: []string{"id"}, Rows: [][]driver.Value{{session.ID}}}
			if fails {
				step.Error = cause
			}
			mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, step, {Op: "rollback"}}}
			db := mock.Open()
			defer db.Close()
			tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = writer.NewSessionWriter(tx).Create(session)
			if fails && !errors.Is(err, cause) || !fails && err != nil {
				t.Fatal("session write outcome", err)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
	if err := writer.NewSessionWriter(nil).Create(domain.Session{}); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("writer used missing handle")
	}
}
