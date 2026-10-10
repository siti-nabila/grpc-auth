package fixtures

import (
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"time"
)

var PersistenceTime = time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

const EventID = "10000000-0000-4000-8000-000000000001"
const SessionID = "20000000-0000-4000-8000-000000000001"

func Audit() domain.AuditRequest {
	actor, _ := domain.ActorForVerifiedUser(1)
	return domain.AuditRequest{Actor: actor, Action: "USER_PROFILE_UPDATED", TargetType: "user", TargetID: "1", Before: &domain.SafeSnapshot{Version: 1}, After: &domain.SafeSnapshot{Version: 2}, RequestID: "request-fixture", TraceID: "trace-fixture", OccurredAt: PersistenceTime}
}
func Event() domain.EventEnvelope {
	actor := uint64(1)
	return domain.EventEnvelope{EventID: EventID, EventType: "iam.user.profile_updated.v1", SchemaVersion: 1, OccurredAt: PersistenceTime, Producer: "grpc-auth", AggregateType: "user", AggregateID: "1", AggregateVersion: 2, ActorUserID: &actor, CorrelationID: "request-fixture", CausationID: "command-fixture", Traceparent: "00-00000000000000000000000000000001-0000000000000001-01", Data: domain.EventData{UserID: 1}}
}
func Scope() domain.IdempotencyScope {
	return domain.IdempotencyScope{ActorID: 1, Method: "/user.v1.UserService/UpdateUser", Target: "user/1", Key: "fixture-key"}
}
