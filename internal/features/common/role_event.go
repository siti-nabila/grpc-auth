package common

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"strconv"
	"time"
)

func RoleEvent(ctx context.Context, id string, event types.EventType, role domain.RoleSummary, actor uint64, codes []string, now time.Time) domain.EventEnvelope {
	correlation, traceparent := eventCorrelation(ctx, id)
	return domain.EventEnvelope{EventID: id, EventType: event, SchemaVersion: 1, OccurredAt: now.UTC(), Producer: "grpc-auth", AggregateType: types.AggregateRole, AggregateID: strconv.FormatUint(role.ID, 10), AggregateVersion: role.Version, ActorUserID: &actor, CorrelationID: correlation, CausationID: correlation, Traceparent: traceparent, Data: domain.EventData{RoleID: role.ID, ApplicationID: role.ApplicationID, ApplicationCode: role.ApplicationCode, Status: string(role.Status), PermissionCodes: domain.NormalizePermissionCodes(codes)}}
}
