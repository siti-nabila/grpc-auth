package common

import (
	"context"
	"regexp"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"google.golang.org/grpc/metadata"
)

var traceparentPattern = regexp.MustCompile(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)

// UserEvent keeps command payloads confined to catalog IDs and status. Caller
// metadata supplies correlation only and never supplies the actor identity.
func UserEvent(ctx context.Context, id string, event types.EventType, userID uint64, version int64, actor *uint64, status types.UserStatus, now time.Time) domain.EventEnvelope {
	correlation, traceparent := eventCorrelation(ctx, id)
	return domain.EventEnvelope{EventID: id, EventType: event, SchemaVersion: 1, OccurredAt: now.UTC(), Producer: "grpc-auth", AggregateType: types.AggregateUser, AggregateID: strconv.FormatUint(userID, 10), AggregateVersion: version, ActorUserID: actor, CorrelationID: correlation, CausationID: correlation, Traceparent: traceparent, Data: domain.EventData{UserID: userID, Status: string(status)}}
}

func eventCorrelation(ctx context.Context, id string) (string, string) {
	md, _ := metadata.FromIncomingContext(ctx)
	correlation := id
	if values := md.Get("x-request-id"); len(values) == 1 && values[0] != "" && len(values[0]) <= 100 && utf8.ValidString(values[0]) {
		correlation = values[0]
	}
	traceparent := ""
	if values := md.Get("traceparent"); len(values) == 1 && traceparentPattern.MatchString(values[0]) {
		traceparent = values[0]
	}
	return correlation, traceparent
}
