package test_scenarios

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	feature "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func IdempotencyStore() []testutils.Scenario {
	return []testutils.Scenario{{Name: "idempotency bound SQL uses complete PK and exact retention without lifecycle calls", Run: idempotencySQL}}
}

func idempotencySQL(t *testing.T) {
	for _, outcome := range []string{"missing", "expired", "unexpired", "lookup failure", "write failure", "credential hash"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), struct{}{}, "request")
			scope := fixtures.Scope()
			now := fixtures.PersistenceTime
			hash, _ := feature.Fingerprint(scope, feature.FingerprintPayload{ExpectedVersion: 1})
			result := domain.SafeCommandResult{UserID: 1, Version: 2, Changed: true}
			payload, _ := json.Marshal(result)
			lookup := mocks.SQLStep{Op: "query", SQL: "WHERE actor_id = $1 AND method = $2 AND route_scope = $3 AND key = $4", Args: []driver.Value{int64(1), scope.Method, scope.Target, scope.Key}, Columns: []string{"actor_id", "method", "route_scope", "key", "request_hash", "safe_response", "created_at", "expires_at"}}
			cause := errors.New("controlled database error")
			if outcome == "expired" || outcome == "unexpired" {
				expiry := now
				if outcome == "unexpired" {
					expiry = now.Add(domain.IdempotencyWindow)
				}
				lookup.Rows = [][]driver.Value{{int64(1), scope.Method, scope.Target, scope.Key, hash, string(payload), now.Add(-domain.IdempotencyWindow), expiry}}
			}
			write := mocks.SQLStep{Op: "exec", SQL: "INSERT INTO idempotency_records", Args: []driver.Value{int64(1), scope.Method, scope.Target, scope.Key, hash, string(payload), now, now.Add(domain.IdempotencyWindow)}}
			if outcome == "expired" {
				write = mocks.SQLStep{Op: "exec", SQL: "UPDATE idempotency_records SET request_hash = $1, safe_response = $2, created_at = $3, expires_at = $4 WHERE actor_id = $5 AND method = $6 AND route_scope = $7 AND key = $8 AND expires_at <= $9", Args: []driver.Value{hash, string(payload), now, now.Add(domain.IdempotencyWindow), int64(1), scope.Method, scope.Target, scope.Key, now}}
			}
			steps := []mocks.SQLStep{{Op: "begin"}, lookup, write, {Op: "rollback"}}
			if outcome == "lookup failure" {
				steps[1].Error = cause
				steps = []mocks.SQLStep{steps[0], steps[1], steps[3]}
			}
			if outcome == "unexpired" {
				steps = []mocks.SQLStep{steps[0], steps[1], steps[3]}
			}
			if outcome == "write failure" {
				steps[2].Error = cause
			}
			if outcome == "credential hash" {
				hash = []byte("$argon2id$login-hash-is-forbidden")
				steps = []mocks.SQLStep{{Op: "begin"}, {Op: "rollback"}}
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			defer db.Close()
			tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			err = writer.NewIdempotencyStore(tx).Save(scope, hash, result, now)
			switch outcome {
			case "unexpired", "credential hash":
				if !errors.Is(err, common.ErrBadRequest) {
					t.Fatal("unsafe overwrite/input accepted", err)
				}
			case "lookup failure", "write failure":
				if !errors.Is(err, cause) {
					t.Fatal("cause lost", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
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
