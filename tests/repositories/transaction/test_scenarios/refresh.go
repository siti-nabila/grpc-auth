package test_scenarios

import (
	"bytes"
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
	repomocks "github.com/siti-nabila/grpc-auth/tests/repositories/transaction/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func RefreshPersistence() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "refresh digest lookup preserves authoritative state on primary and bound transaction", Run: refreshReads},
		{Name: "refresh rotation inserts successor before conditional link and reports affected rows", Run: refreshConsumes},
		{Name: "refresh adapters reject missing handles malformed input and cancelled context without SQL", Run: refreshGuards},
	}
}

func refreshReads(t *testing.T) {
	digest := bytes.Repeat([]byte{7}, 32)
	at := fixtures.ServiceTime
	cols := []string{"id", "session_id", "token_hash", "family_id", "issued_at", "expires_at", "consumed_at", "revoked_at", "replaced_by"}
	row := []driver.Value{fixtures.AccessTokenID, fixtures.SessionID, digest, "00000000-0000-4000-8000-000000000003", at, at.Add(time.Hour), at, at, "00000000-0000-4000-8000-000000000004"}
	for _, bound := range []bool{false, true} {
		for _, kind := range []string{"success", "missing", "failure", "cancelled"} {
			t.Run(kind+map[bool]string{true: " tx", false: " primary"}[bound], func(t *testing.T) {
				ctx := context.WithValue(context.Background(), struct{}{}, "refresh-digest-context")
				cause := errors.New("controlled refresh lookup failure")
				step := mocks.SQLStep{Op: "query", SQL: "WHERE token_hash = $1 LIMIT 1", Args: []driver.Value{digest}, Columns: cols, Rows: [][]driver.Value{row}}
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
				defer db.Close()
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
				got, err := reader.NewRefreshReader(handle).GetByDigest(ctx, digest)
				switch kind {
				case "success":
					if err != nil || got.ID != fixtures.AccessTokenID || got.SessionID != fixtures.SessionID || !bytes.Equal(got.TokenHash, digest) || !got.ConsumedAt.Valid || !got.RevokedAt.Valid || !got.ReplacedBy.Valid || got.FamilyID != row[3] || !got.ExpiresAt.Equal(at.Add(time.Hour)) {
						t.Fatal("refresh state lost", err)
					}
				case "missing":
					if !errors.Is(err, auth.ErrNotFound) {
						t.Fatal("missing mapping", err)
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
			})
		}
	}
}

func refreshConsumes(t *testing.T) {
	at := fixtures.ServiceTime
	predecessor := "00000000-0000-4000-8000-000000000003"
	successor := domain.InitialRefresh{ID: fixtures.AccessTokenID, SessionID: fixtures.SessionID, TokenHash: bytes.Repeat([]byte{8}, 32), FamilyID: "00000000-0000-4000-8000-000000000004", IssuedAt: at, ExpiresAt: at.Add(time.Hour)}
	cause := errors.New("controlled refresh consume failure")
	cases := []struct {
		name    string
		result  driver.Result
		failure error
		changed bool
		want    error
	}{
		{"one", driver.RowsAffected(1), nil, true, nil},
		{"zero", driver.RowsAffected(0), nil, false, nil},
		{"too many", driver.RowsAffected(2), nil, false, common.ErrInternalServerError},
		{"negative", driver.RowsAffected(-1), nil, false, common.ErrInternalServerError},
		{"write failure", nil, cause, false, cause},
		{"cancelled", nil, context.Canceled, false, context.Canceled},
		{"result failure", repomocks.AffectedFailure{Err: cause}, nil, false, cause},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "refresh-consume-context")
			mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{
				{Op: "begin"},
				{Op: "query", SQL: "INSERT INTO refresh_tokens", Args: []driver.Value{successor.ID, successor.SessionID, successor.TokenHash, successor.FamilyID, at, successor.ExpiresAt, nil, nil, nil}, Columns: []string{"id"}, Rows: [][]driver.Value{{successor.ID}}},
				{Op: "exec", SQL: "WHERE id = $3 AND consumed_at IS NULL AND revoked_at IS NULL AND replaced_by IS NULL AND expires_at > $4", Args: []driver.Value{at, successor.ID, predecessor, at}, Result: tc.result, Error: tc.failure},
				{Op: "rollback"},
			}}
			db := mock.Open()
			defer db.Close()
			tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			w := writer.NewRefreshRotationWriter(tx)
			if err = w.Create(successor); err != nil {
				t.Fatal(err)
			}
			changed, err := w.Consume(ctx, domain.RefreshConsumption{ID: predecessor, SuccessorID: successor.ID, At: at})
			if changed != tc.changed || (tc.want == nil && err != nil) || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatal("consume outcome", changed, err)
			}
			// Repository returns state; the caller owns rollback including its successor.
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err = mock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func refreshGuards(t *testing.T) {
	ctx := context.Background()
	if _, err := reader.NewRefreshReader(nil).GetByDigest(ctx, make([]byte, 32)); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing reader fallback", err)
	}
	if _, err := writer.NewRefreshRotationWriter(nil).Consume(ctx, domain.RefreshConsumption{}); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing writer fallback", err)
	}
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, {Op: "rollback"}}}
	db := mock.Open()
	defer db.Close()
	handle := orm.NewSqlQueryAdapter(ctx, db, dialect.NewPostgres(), database.GetORMConfig())
	for _, digest := range [][]byte{nil, {}, make([]byte, 31), make([]byte, 33)} {
		if _, err := reader.NewRefreshReader(handle).GetByDigest(ctx, digest); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("invalid digest accepted", err)
		}
	}
	tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := writer.NewRefreshRotationWriter(tx)
	base := domain.RefreshConsumption{ID: fixtures.AccessTokenID, SuccessorID: fixtures.SessionID, At: fixtures.ServiceTime}
	for _, change := range []func(*domain.RefreshConsumption){func(r *domain.RefreshConsumption) { r.ID = "bad" }, func(r *domain.RefreshConsumption) { r.SuccessorID = "bad" }, func(r *domain.RefreshConsumption) { r.SuccessorID = r.ID }, func(r *domain.RefreshConsumption) { r.At = time.Time{} }} {
		r := base
		change(&r)
		if !errors.Is(r.Validate(), common.ErrBadRequest) {
			t.Fatal("invalid consumption accepted")
		}
		if _, err := w.Consume(ctx, r); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("invalid consume reached SQL", err)
		}
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := reader.NewRefreshReader(nil).GetByDigest(cancelled, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("read cancellation lost", err)
	}
	if _, err := writer.NewRefreshRotationWriter(nil).Consume(cancelled, base); !errors.Is(err, context.Canceled) {
		t.Fatal("write cancellation lost", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
}
