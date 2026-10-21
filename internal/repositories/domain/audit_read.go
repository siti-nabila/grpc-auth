package domain

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/orm/orm"
)

const AuditLogMethod = "/audit.v1.AuditService/ListAuditLogs"

type AuditLogQuery struct {
	Page, Limit                                int
	LastID                                     string
	Unsupported                                bool
	ActorUserID                                *uint64
	ActorService, Action, TargetType, TargetID string
	OccurredFrom, OccurredBefore               *time.Time
}

type AuditLogReader interface {
	List(context.Context, AuditLogQuery) (orm.PageData[AuditLogItem], error)
}

type AuditLogItem struct {
	ID                           uint64
	ActorUserID                  *uint64
	ActorService                 *string
	Action, TargetType, TargetID string
	Before, After                *SafeSnapshot
	Reason, RequestID, TraceID   *string
	OccurredAt                   time.Time
}

func (q AuditLogQuery) Normalized() (AuditLogQuery, error) {
	if q.Unsupported || q.Page < 0 || q.Page > math.MaxInt32 || q.Limit < 0 || q.Limit > 100 || len(q.LastID) > 4096 {
		return q, common.ErrBadRequest
	}
	if q.Page == 0 {
		q.Page = 1
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if int64(q.Page-1) > math.MaxInt64/int64(q.Limit) {
		return q, common.ErrBadRequest
	}
	if q.ActorUserID != nil {
		id := *q.ActorUserID
		if ValidateManagementTarget(id) != nil {
			return q, common.ErrBadRequest
		}
		q.ActorUserID = &id
	}
	for _, f := range []struct {
		value *string
		max   int
	}{{&q.ActorService, 100}, {&q.Action, 100}, {&q.TargetType, 50}, {&q.TargetID, 100}} {
		*f.value = strings.TrimSpace(*f.value)
		if !auditQueryText(*f.value, f.max) {
			return q, common.ErrBadRequest
		}
	}
	if q.TargetID != "" && q.TargetType == "" {
		return q, common.ErrBadRequest
	}
	for _, f := range []**time.Time{&q.OccurredFrom, &q.OccurredBefore} {
		if *f != nil {
			at := (**f).UTC()
			if at.Year() < 1 || at.Year() > 9999 {
				return q, common.ErrBadRequest
			}
			*f = &at
		}
	}
	if q.OccurredFrom != nil && q.OccurredBefore != nil && !q.OccurredFrom.Before(*q.OccurredBefore) {
		return q, common.ErrBadRequest
	}
	if _, err := q.Cursor(); err != nil {
		return q, err
	}
	return q, nil
}

func auditQueryText(s string, max int) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= max && !strings.ContainsFunc(s, unicode.IsControl)
}

// SafeItem validates historical storage without looking up a live actor/target.
func (r AuditRecord) SafeItem() (AuditLogItem, error) {
	var item AuditLogItem
	if r.ID <= 0 || r.OccurredAt.IsZero() || r.OccurredAt.Year() < 1 || r.OccurredAt.Year() > 9999 || r.ActorUserID == nil && r.ActorService == nil || r.ActorUserID != nil && ValidateManagementTarget(*r.ActorUserID) != nil {
		return item, common.ErrDependencyUnavailable
	}
	for _, f := range []struct {
		value    *string
		max      int
		required bool
	}{
		{r.ActorService, 100, true}, {&r.Action, 100, true}, {&r.TargetType, 50, true}, {&r.TargetID, 100, true},
		{r.Reason, 500, false}, {r.RequestID, 100, false}, {r.TraceID, 100, false},
	} {
		if f.value != nil && !boundedText(*f.value, f.max, f.required) {
			return item, common.ErrDependencyUnavailable
		}
	}
	before, err := auditSnapshot(r.SafeBefore)
	if err != nil {
		return item, err
	}
	after, err := auditSnapshot(r.SafeAfter)
	if err != nil {
		return item, err
	}
	return AuditLogItem{ID: uint64(r.ID), ActorUserID: r.ActorUserID, ActorService: r.ActorService, Action: r.Action, TargetType: r.TargetType, TargetID: r.TargetID, Before: before, After: after, Reason: r.Reason, RequestID: r.RequestID, TraceID: r.TraceID, OccurredAt: r.OccurredAt.UTC()}, nil
}

func auditSnapshot(raw *string) (*SafeSnapshot, error) {
	if raw == nil {
		return nil, nil
	}
	b := bytes.TrimSpace([]byte(*raw))
	if len(b) < 2 || b[0] != '{' || b[len(b)-1] != '}' {
		return nil, common.ErrDependencyUnavailable
	}
	var snapshot SafeSnapshot
	if json.Unmarshal(b, &snapshot) != nil {
		return nil, common.ErrDependencyUnavailable
	}
	return &snapshot, nil
}
