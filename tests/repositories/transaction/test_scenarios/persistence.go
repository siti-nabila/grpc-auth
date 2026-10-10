package test_scenarios

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func Persistence() []testutils.Scenario {
	return []testutils.Scenario{{Name: "safe persistence rejects credentials absent actors and schema overflow", Run: safePersistence}, {Name: "catalog envelope preserves user role batch compound and terminal session events", Run: eventCatalog}, {Name: "audit and outbox writes use supplied transaction context and preserve failures", Run: persistenceSQL}}
}

func safePersistence(t *testing.T) {
	for _, target := range []any{&domain.SafeSnapshot{}, &domain.EventData{}, &domain.SafeCommandResult{}} {
		for _, key := range []string{"password", "password_hash", "access_token", "refresh_token", "authorization", "raw_request"} {
			if err := json.Unmarshal([]byte(`{"`+key+`":"sensitive"}`), target); err == nil {
				t.Fatal("credential accepted", key)
			}
		}
	}
	for _, change := range []func(*domain.AuditRequest){func(r *domain.AuditRequest) { r.Actor = nil }, func(r *domain.AuditRequest) { r.Action = types.AuditAction(strings.Repeat("a", 101)) }, func(r *domain.AuditRequest) { r.Reason = types.AuditReason(strings.Repeat("a", 501)) }, func(r *domain.AuditRequest) { r.TraceID = strings.Repeat("a", 101) }, func(r *domain.AuditRequest) { r.After = &domain.SafeSnapshot{SessionID: "opaque-token"} }, func(r *domain.AuditRequest) { r.After = &domain.SafeSnapshot{ReasonCode: "free reason"} }} {
		r := fixtures.Audit()
		change(&r)
		if _, err := r.Record(); !errors.Is(err, common.ErrBadRequest) {
			t.Fatal("unsafe audit accepted")
		}
	}
	if _, err := domain.ActorForVerifiedUser(0); err == nil {
		t.Fatal("zero actor accepted")
	}
	if _, err := domain.ActorForVerifiedService(" "); err == nil {
		t.Fatal("empty service accepted")
	}
	service, _ := domain.ActorForVerifiedService("verified-fixture-caller")
	a := fixtures.Audit()
	a.Actor = service
	a.Before = nil
	a.After = nil
	r, err := a.Record()
	if err != nil || r.ActorUserID != nil || r.ActorService == nil || r.SafeBefore != nil || r.SafeAfter != nil || r.Reason != nil {
		t.Fatal("service actor or nullable fields incorrect")
	}
	for _, err := range []error{writer.NewAuditWriter(nil).Create(fixtures.Audit()), writer.NewOutboxWriter(nil).Create(fixtures.Event()), writer.NewIdempotencyStore(nil).Save(fixtures.Scope(), []byte("digest"), domain.SafeCommandResult{}, fixtures.PersistenceTime)} {
		if !errors.Is(err, common.ErrInternalServerError) {
			t.Fatal("nil transaction fell back")
		}
	}
}

func eventCatalog(t *testing.T) {
	for _, name := range []string{"iam.user.created.v1", "iam.user.profile_updated.v1", "iam.user.status_changed.v1", "iam.user.roles_granted.v1", "iam.user.roles_revoked.v1", "iam.user.application_access_granted.v1", "iam.user.application_access_revoked.v1", "iam.role.created.v1", "iam.role.updated.v1", "iam.role.permissions_changed.v1", "iam.session.revoked.v1", "iam.user.sessions_revoked.v1"} {
		e := fixtures.Event()
		e.EventType = types.EventType(name)
		if strings.HasPrefix(name, "iam.role.") {
			e.AggregateType = "role"
			e.Data = domain.EventData{RoleID: 1, PermissionCodes: []string{"iam.user.read"}}
		}
		if strings.Contains(name, "roles_") || strings.Contains(name, "application_access_") {
			e.Data.ChangedRoleIDs = []uint64{2, 3}
			e.Data.ApplicationID = 1
			e.Data.ApplicationCode = types.ApplicationCLM
		}
		if name == "iam.session.revoked.v1" {
			e.AggregateType = "session"
			e.AggregateID = fixtures.SessionID
			e.Data.SessionID = fixtures.SessionID
			e.Data.ReasonCode = "REVOKED"
		}
		r, err := e.Record()
		if err != nil {
			t.Fatal(name, err)
		}
		if r.PublishedAt != nil || r.Attempts != 0 || !r.NextAttemptAt.Equal(fixtures.PersistenceTime) {
			t.Fatal("event not pending")
		}
		var decoded domain.EventEnvelope
		if err = json.Unmarshal([]byte(r.Payload), &decoded); err != nil || !reflect.DeepEqual(decoded, e) {
			t.Fatal("envelope metadata or batch changed")
		}
		var fields map[string]any
		_ = json.Unmarshal([]byte(r.Payload), &fields)
		for _, key := range []string{"event_id", "event_type", "schema_version", "occurred_at", "producer", "aggregate_type", "aggregate_id", "aggregate_version", "actor_user_id", "correlation_id", "causation_id", "traceparent", "data"} {
			if _, ok := fields[key]; !ok {
				t.Fatal("missing envelope field", key)
			}
		}
		if fields["event_type"] != name || fields["aggregate_type"] != string(e.AggregateType) {
			t.Fatal("typed event changed wire values")
		}
		if e.Data.ApplicationCode != "" {
			data, ok := fields["data"].(map[string]any)
			if !ok || data["application_code"] != "clm" {
				t.Fatal("typed application code changed wire value")
			}
		}
		if len(fields) != 13 {
			t.Fatal("unexpected wire metadata")
		}
	}
	for _, change := range []func(*domain.EventEnvelope){func(e *domain.EventEnvelope) { e.EventType = "iam.session.rotated.v1" }, func(e *domain.EventEnvelope) { e.SchemaVersion = 2 }, func(e *domain.EventEnvelope) { e.AggregateType = "role" }, func(e *domain.EventEnvelope) { e.EventID = "opaque-token" }, func(e *domain.EventEnvelope) { e.Data.ReasonCode = "free reason" }, func(e *domain.EventEnvelope) {
		e.EventType = "iam.session.revoked.v1"
		e.AggregateType = "session"
		e.AggregateID = fixtures.SessionID
		e.AggregateVersion = 3
	}} {
		e := fixtures.Event()
		change(&e)
		if _, err := e.Record(); err == nil {
			t.Fatal("invalid event accepted")
		}
	}
}

func persistenceSQL(t *testing.T) {
	for _, kind := range []string{"audit", "outbox"} {
		for _, fails := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: " failure", false: " success"}[fails], func(t *testing.T) {
				ctx := context.WithValue(context.Background(), struct{}{}, "request")
				a, _ := fixtures.Audit().Record()
				e, _ := fixtures.Event().Record()
				step := mocks.SQLStep{Op: "query", SQL: "INSERT INTO audit_logs", Args: []driver.Value{int64(1), nil, a.Action, a.TargetType, a.TargetID, *a.SafeBefore, *a.SafeAfter, nil, *a.RequestID, *a.TraceID, a.OccurredAt}, Columns: []string{"id"}, Rows: [][]driver.Value{{int64(1)}}}
				if kind == "outbox" {
					step = mocks.SQLStep{Op: "query", SQL: "INSERT INTO outbox_events", Args: []driver.Value{e.EventID, e.EventType, int64(e.SchemaVersion), e.AggregateType, e.AggregateID, e.AggregateVersion, e.Payload, e.CreatedAt, nil, int64(0), e.NextAttemptAt}, Columns: []string{"event_id"}, Rows: [][]driver.Value{{e.EventID}}}
				}
				cause := errors.New("controlled persistence failure")
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
				if kind == "audit" {
					err = writer.NewAuditWriter(tx).Create(fixtures.Audit())
				} else {
					err = writer.NewOutboxWriter(tx).Create(fixtures.Event())
				}
				if fails && !errors.Is(err, cause) {
					t.Fatal("write cause lost", err)
				}
				if !fails && err != nil {
					t.Fatal(err)
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
