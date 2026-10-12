package domain

import (
	"encoding/json"
	"regexp"
	"strconv"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

var canonicalUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var reasonCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

var eventAggregates = map[types.EventType]types.AggregateType{
	types.EventUserCreated:                  types.AggregateUser,
	types.EventUserProfileUpdated:           types.AggregateUser,
	types.EventUserStatusChanged:            types.AggregateUser,
	types.EventUserRolesGranted:             types.AggregateUser,
	types.EventUserRolesRevoked:             types.AggregateUser,
	types.EventUserApplicationAccessGranted: types.AggregateUser,
	types.EventUserApplicationAccessRevoked: types.AggregateUser,
	types.EventRoleCreated:                  types.AggregateRole,
	types.EventRoleUpdated:                  types.AggregateRole,
	types.EventRolePermissionsChanged:       types.AggregateRole,
	types.EventSessionRevoked:               types.AggregateSession,
	types.EventUserSessionsRevoked:          types.AggregateUser,
}

type EventData struct {
	UserID          uint64                `json:"user_id,omitempty"`
	RoleID          uint64                `json:"role_id,omitempty"`
	SessionID       string                `json:"session_id,omitempty"`
	ApplicationID   uint64                `json:"application_id,omitempty"`
	ApplicationCode types.ApplicationCode `json:"application_code,omitempty"`
	Status          string                `json:"status,omitempty"`
	ChangedRoleIDs  []uint64              `json:"changed_role_ids,omitempty"`
	PermissionCodes []string              `json:"permission_codes,omitempty"`
	ReasonCode      string                `json:"reason_code,omitempty"`
	SessionsRevoked *bool                 `json:"sessions_revoked,omitempty"`
}

func (d *EventData) UnmarshalJSON(data []byte) error {
	type fields EventData
	var value fields
	if err := decodeSafeJSON(data, &value); err != nil {
		return err
	}
	*d = EventData(value)
	return nil
}

type EventEnvelope struct {
	EventID          string              `json:"event_id"`
	EventType        types.EventType     `json:"event_type"`
	SchemaVersion    int                 `json:"schema_version"`
	OccurredAt       time.Time           `json:"occurred_at"`
	Producer         string              `json:"producer"`
	AggregateType    types.AggregateType `json:"aggregate_type"`
	AggregateID      string              `json:"aggregate_id"`
	AggregateVersion int64               `json:"aggregate_version"`
	ActorUserID      *uint64             `json:"actor_user_id,omitempty"`
	CorrelationID    string              `json:"correlation_id"`
	CausationID      string              `json:"causation_id"`
	Traceparent      string              `json:"traceparent,omitempty"`
	Data             EventData           `json:"data"`
}

type OutboxWriter interface{ Create(EventEnvelope) error }

func (e *EventEnvelope) UnmarshalJSON(data []byte) error {
	type fields EventEnvelope
	var value fields
	if err := decodeSafeJSON(data, &value); err != nil {
		return err
	}
	*e = EventEnvelope(value)
	return nil
}

type OutboxRecord struct {
	EventID          string     `sql:"column:event_id;primaryKey"`
	EventType        string     `sql:"column:event_type"`
	SchemaVersion    int        `sql:"column:schema_version"`
	AggregateType    string     `sql:"column:aggregate_type"`
	AggregateID      string     `sql:"column:aggregate_id"`
	AggregateVersion int64      `sql:"column:aggregate_version"`
	Payload          string     `sql:"column:payload"`
	CreatedAt        time.Time  `sql:"column:created_at"`
	PublishedAt      *time.Time `sql:"column:published_at"`
	Attempts         int        `sql:"column:attempts"`
	NextAttemptAt    time.Time  `sql:"column:next_attempt_at"`
}

func (OutboxRecord) TableName() string { return "outbox_events" }

func (e EventEnvelope) Record() (*OutboxRecord, error) {
	aggregate, ok := eventAggregates[e.EventType]
	if !ok || aggregate != e.AggregateType || !canonicalUUID.MatchString(e.EventID) || e.SchemaVersion != 1 || e.AggregateVersion < 1 || e.OccurredAt.IsZero() || !boundedText(e.Producer, 100, true) || !boundedText(e.AggregateID, 100, true) {
		return nil, common.ErrBadRequest
	}
	if aggregate == types.AggregateSession {
		if !canonicalUUID.MatchString(e.AggregateID) || e.AggregateVersion != 2 {
			return nil, common.ErrBadRequest
		}
	} else {
		id, err := strconv.ParseUint(e.AggregateID, 10, 64)
		if err != nil || id == 0 || strconv.FormatUint(id, 10) != e.AggregateID {
			return nil, common.ErrBadRequest
		}
	}
	if e.ActorUserID != nil && *e.ActorUserID == 0 {
		return nil, common.ErrBadRequest
	}
	if err := e.Data.validate(); err != nil {
		return nil, err
	}
	if (e.EventType == types.EventUserRolesGranted || e.EventType == types.EventUserRolesRevoked) && len(e.Data.ChangedRoleIDs) > 20 {
		return nil, common.ErrBadRequest
	}
	e.OccurredAt = e.OccurredAt.UTC()
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, common.ErrBadRequest
	}
	return &OutboxRecord{EventID: e.EventID, EventType: string(e.EventType), SchemaVersion: e.SchemaVersion, AggregateType: string(e.AggregateType), AggregateID: e.AggregateID, AggregateVersion: e.AggregateVersion, Payload: string(payload), CreatedAt: e.OccurredAt, NextAttemptAt: e.OccurredAt}, nil
}

func (d EventData) validate() error {
	if !boundedText(d.Status, 16, false) || !boundedText(string(d.ApplicationCode), 50, false) || (d.SessionID != "" && !canonicalUUID.MatchString(d.SessionID)) || (d.ReasonCode != "" && (!boundedText(d.ReasonCode, 100, false) || !reasonCodePattern.MatchString(d.ReasonCode))) {
		return common.ErrBadRequest
	}
	for _, id := range d.ChangedRoleIDs {
		if id == 0 {
			return common.ErrBadRequest
		}
	}
	for _, code := range d.PermissionCodes {
		if !boundedText(code, 150, true) {
			return common.ErrBadRequest
		}
	}
	return nil
}
