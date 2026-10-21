package test_scenarios

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	repomocks "github.com/siti-nabila/grpc-auth/tests/repositories/transaction/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func CredentialInvalidationPersistence() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "credential generation and password writes are conditional and field limited", Run: credentialInvalidationWrites},
		{Name: "invalid credential writes never execute and safe lifecycle audit/event data exclude secrets", Run: credentialInvalidationGuards},
	}
}
func credentialInvalidationWrites(t *testing.T) {
	cause := errors.New("controlled credential update failure")
	req := domain.CredentialInvalidation{UserID: 42, ExpectedVersion: 4, ExpectedAuthVersion: 7, At: fixtures.ServiceTime}
	for _, password := range []bool{false, true} {
		for _, kind := range []string{"one", "zero", "many", "write failure", "result failure", "cancelled"} {
			t.Run(kind+map[bool]string{true: " password", false: " generation"}[password], func(t *testing.T) {
				ctx := context.WithValue(context.Background(), struct{}{}, "credential-update-context")
				var result driver.Result = driver.RowsAffected(1)
				var failure, want error
				if kind == "zero" {
					result = driver.RowsAffected(0)
				}
				if kind == "many" {
					result = driver.RowsAffected(2)
					want = common.ErrInternalServerError
				}
				if kind == "write failure" {
					failure = cause
					want = cause
				}
				if kind == "cancelled" {
					failure = context.Canceled
					want = context.Canceled
				}
				if kind == "result failure" {
					result = repomocks.AffectedFailure{Err: cause}
					want = cause
				}
				sql := "UPDATE auth SET version = $1, auth_version = $2, updated_at = $3 WHERE id = $4 AND version = $5 AND auth_version = $6"
				args := []driver.Value{int64(5), int64(8), req.At, int64(42), int64(4), int64(7)}
				if password {
					sql = "UPDATE auth SET password = $1, password_changed_at = $2, must_change_password = $3, version = $4, auth_version = $5, updated_at = $6 WHERE id = $7 AND version = $8 AND auth_version = $9"
					args = []driver.Value{"synthetic-encoded-hash", req.At, false, int64(5), int64(8), req.At, int64(42), int64(4), int64(7)}
				}
				mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, {Op: "exec", SQL: sql, Args: args, Result: result, Error: failure}, {Op: "rollback"}}}
				db := mock.Open()
				defer db.Close()
				tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				w := writer.NewCredentialInvalidationWriter(tx)
				var changed bool
				if password {
					changed, err = w.ChangePassword(ctx, domain.PasswordChange{CredentialInvalidation: req, PasswordHash: "synthetic-encoded-hash"})
				} else {
					changed, err = w.Invalidate(ctx, req)
				}
				if want == nil && (err != nil || changed != (kind == "one")) || want != nil && !errors.Is(err, want) {
					t.Fatal("credential outcome", changed, err)
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
}
func credentialInvalidationGuards(t *testing.T) {
	ctx := context.Background()
	mock := &mocks.SQL{Context: ctx, Steps: []mocks.SQLStep{{Op: "begin"}, {Op: "rollback"}}}
	db := mock.Open()
	defer db.Close()
	tx, err := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w := writer.NewCredentialInvalidationWriter(tx)
	base := domain.CredentialInvalidation{UserID: 42, ExpectedVersion: 4, ExpectedAuthVersion: 7, At: fixtures.ServiceTime}
	for _, mutate := range []func(*domain.CredentialInvalidation){func(r *domain.CredentialInvalidation) { r.UserID = 0 }, func(r *domain.CredentialInvalidation) { r.UserID = 2147483648 }, func(r *domain.CredentialInvalidation) { r.ExpectedVersion = 0 }, func(r *domain.CredentialInvalidation) { r.ExpectedAuthVersion = 0 }, func(r *domain.CredentialInvalidation) { r.ExpectedVersion = math.MaxInt64 }, func(r *domain.CredentialInvalidation) { r.ExpectedAuthVersion = math.MaxInt64 }, func(r *domain.CredentialInvalidation) { r.At = time.Time{} }} {
		r := base
		mutate(&r)
		if _, err := w.Invalidate(ctx, r); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("invalid generation reached SQL", err)
		}
	}
	for _, hash := range []string{"", "  ", strings.Repeat("x", 151)} {
		if _, err := w.ChangePassword(ctx, domain.PasswordChange{CredentialInvalidation: base, PasswordHash: hash}); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("invalid password hash reached SQL", err)
		}
	}
	if _, err := writer.NewCredentialInvalidationWriter(nil).Invalidate(ctx, base); !errors.Is(err, common.ErrInternalServerError) {
		t.Fatal("missing handle fallback")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := w.Invalidate(cancelled, base); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled write reached SQL")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = mock.Verify(); err != nil {
		t.Fatal(err)
	}
	req := domain.PasswordChange{CredentialInvalidation: base, PasswordHash: "synthetic-encoded-hash"}
	data, err := json.Marshal(req)
	if err != nil || strings.Contains(string(data), req.PasswordHash) {
		t.Fatal("credential serialized", err)
	}
	actor, err := domain.ActorForVerifiedUser(42)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []types.AuditAction{types.AuditSessionRefreshRotated, types.AuditRefreshReuseDetected, types.AuditSessionRevoked, types.AuditUserSessionsRevoked, types.AuditPasswordChanged} {
		record, err := (domain.AuditRequest{Actor: actor, Action: action, TargetType: types.AuditTargetSession, TargetID: fixtures.SessionID, Reason: types.AuditPasswordChange, After: &domain.SafeSnapshot{SessionID: fixtures.SessionID, Status: string(types.SessionRevoked)}, OccurredAt: fixtures.ServiceTime}).Record()
		if err != nil || record.Action != string(action) {
			t.Fatal("audit lifecycle serialization", err)
		}
		data, err = json.Marshal(record)
		if err != nil || strings.Contains(string(data), req.PasswordHash) {
			t.Fatal("secret in audit", err)
		}
	}
	for _, global := range []bool{false, true} {
		e := domain.EventEnvelope{EventID: fixtures.AccessTokenID, EventType: types.EventSessionRevoked, SchemaVersion: 1, Producer: "test-producer", AggregateType: types.AggregateSession, AggregateID: fixtures.SessionID, AggregateVersion: 2, OccurredAt: fixtures.ServiceTime, Data: domain.EventData{SessionID: fixtures.SessionID, ReasonCode: string(types.AuditLogout)}}
		if global {
			e.EventType = types.EventUserSessionsRevoked
			e.AggregateType = types.AggregateUser
			e.AggregateID = "42"
			e.AggregateVersion = 5
			e.Data = domain.EventData{UserID: 42, ReasonCode: string(types.AuditPasswordChange)}
		}
		record, err := e.Record()
		if err != nil || record.AggregateVersion != e.AggregateVersion || record.PublishedAt != nil || strings.Contains(record.Payload, req.PasswordHash) {
			t.Fatal("safe terminal/global outbox", err)
		}
	}
}
