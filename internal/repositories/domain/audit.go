package domain

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// AuditActor is created only after the consumer has authenticated the caller.
// These constructors validate storage identity, not caller authentication.
type AuditActor struct {
	userID  *uint64
	service *string
}

func ActorForVerifiedUser(id uint64) (*AuditActor, error) {
	if id == 0 {
		return nil, common.ErrBadRequest
	}
	return &AuditActor{userID: &id}, nil
}

func ActorForVerifiedService(id string) (*AuditActor, error) {
	if !boundedText(id, 100, true) {
		return nil, common.ErrBadRequest
	}
	return &AuditActor{service: &id}, nil
}

type AuditRequest struct {
	Actor      *AuditActor
	Action     types.AuditAction
	TargetType types.AuditTarget
	TargetID   string
	Before     *SafeSnapshot
	After      *SafeSnapshot
	Reason     types.AuditReason
	RequestID  string
	TraceID    string
	OccurredAt time.Time
}

type AuditWriter interface{ Create(AuditRequest) error }

// AuditRecord is the database DTO; callers supply AuditRequest instead.
type AuditRecord struct {
	ID           int64     `sql:"column:id;primaryKey"`
	ActorUserID  *uint64   `sql:"column:actor_user_id"`
	ActorService *string   `sql:"column:actor_service"`
	Action       string    `sql:"column:action"`
	TargetType   string    `sql:"column:target_type"`
	TargetID     string    `sql:"column:target_id"`
	SafeBefore   *string   `sql:"column:safe_before"`
	SafeAfter    *string   `sql:"column:safe_after"`
	Reason       *string   `sql:"column:reason"`
	RequestID    *string   `sql:"column:request_id"`
	TraceID      *string   `sql:"column:trace_id"`
	OccurredAt   time.Time `sql:"column:occurred_at"`
}

func (AuditRecord) TableName() string { return "audit_logs" }

func (r AuditRequest) Record() (*AuditRecord, error) {
	if r.Actor == nil || (r.Actor.userID == nil && r.Actor.service == nil) || !boundedText(string(r.Action), 100, true) || !boundedText(string(r.TargetType), 50, true) || !boundedText(r.TargetID, 100, true) || !boundedText(string(r.Reason), 500, false) || !boundedText(r.RequestID, 100, false) || !boundedText(r.TraceID, 100, false) || r.OccurredAt.IsZero() {
		return nil, common.ErrBadRequest
	}
	before, err := encodeSnapshot(r.Before)
	if err != nil {
		return nil, err
	}
	after, err := encodeSnapshot(r.After)
	if err != nil {
		return nil, err
	}
	return &AuditRecord{ActorUserID: r.Actor.userID, ActorService: r.Actor.service, Action: string(r.Action), TargetType: string(r.TargetType), TargetID: r.TargetID, SafeBefore: before, SafeAfter: after, Reason: optionalText(string(r.Reason)), RequestID: optionalText(r.RequestID), TraceID: optionalText(r.TraceID), OccurredAt: r.OccurredAt.UTC()}, nil
}

func encodeSnapshot(snapshot *SafeSnapshot) (*string, error) {
	if snapshot == nil {
		return nil, nil
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return nil, common.ErrBadRequest
	}
	value := string(data)
	return &value, nil
}

func boundedText(value string, max int, required bool) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= max && (!required || strings.TrimSpace(value) != "")
}
func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
