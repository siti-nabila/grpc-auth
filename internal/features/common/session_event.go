package common

import (
	"context"
	"time"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

func SessionRevokedEvent(ctx context.Context, id string, userID, actorID uint64, sessionID string, reason types.AuditReason, now time.Time) domain.EventEnvelope {
	e := UserEvent(ctx, id, types.EventSessionRevoked, userID, 2, &actorID, types.UserActive, now)
	e.AggregateType = types.AggregateSession
	e.AggregateID = sessionID
	e.Data = domain.EventData{UserID: userID, SessionID: sessionID, Status: string(types.SessionRevoked), ReasonCode: string(reason)}
	return e
}
