package domain

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type SessionSortField string

const (
	SessionSortCreatedAt  SessionSortField = "created_at"
	SessionSortLastSeenAt SessionSortField = "last_seen_at"
	SessionSortExpiresAt  SessionSortField = "expires_at"
)

// SessionSummary is a safe read model without credential/hash/counter columns.
type SessionSummary struct {
	ID          string              `sql:"column:id"`
	UserID      uint64              `sql:"column:user_id"`
	Status      types.SessionStatus `sql:"column:status"`
	CreatedAt   time.Time           `sql:"column:created_at"`
	LastSeenAt  sql.NullTime        `sql:"column:last_seen_at"`
	ExpiresAt   time.Time           `sql:"column:expires_at"`
	RevokedAt   sql.NullTime        `sql:"column:revoked_at"`
	DeviceLabel sql.NullString      `sql:"column:device_label"`
	UserAgent   sql.NullString      `sql:"column:user_agent"`
	IPAddress   sql.NullString      `sql:"column:ip_address"`
}

func (SessionSummary) TableName() string { return "sessions" }

type SessionListCursor struct {
	At sql.NullTime
	ID string
}

func ValidateSessionID(id string) error {
	if !canonicalUUID.MatchString(id) {
		return common.ErrBadRequest
	}
	return nil
}

type SessionListQuery struct {
	UserID uint64
	Field  SessionSortField
	Desc   bool
	Limit  int
	After  *SessionListCursor
}

func (q SessionListQuery) Validate() error {
	if q.UserID == 0 || q.UserID > 2147483647 || q.Limit < 1 || q.Limit > 100 {
		return common.ErrBadRequest
	}
	switch q.Field {
	case SessionSortCreatedAt, SessionSortLastSeenAt, SessionSortExpiresAt:
	default:
		return common.ErrBadRequest
	}
	if q.After != nil && (!canonicalUUID.MatchString(q.After.ID) || (q.After.At.Valid && q.After.At.Time.IsZero()) || (!q.After.At.Valid && q.Field != SessionSortLastSeenAt)) {
		return common.ErrBadRequest
	}
	return nil
}

type SessionListPage struct {
	Items   []SessionSummary
	Total   int64
	HasNext bool
}
type SessionLifecycleReader interface {
	GetByID(context.Context, string) (SessionSummary, error)
	ListIDs(context.Context, uint64) ([]string, error)
	ListPage(context.Context, SessionListQuery) (SessionListPage, error)
}

type SessionRevocation struct {
	UserID, ActorID uint64
	SessionID       string
	At              time.Time
	Reason          types.AuditReason
}

func (r SessionRevocation) Validate() error {
	if r.UserID == 0 || r.UserID > 2147483647 || r.ActorID == 0 || r.ActorID > 2147483647 || r.At.IsZero() || !boundedText(string(r.Reason), 500, true) || utf8.RuneCountInString(strings.TrimSpace(string(r.Reason))) < 3 {
		return common.ErrBadRequest
	}
	if r.SessionID != "" && !canonicalUUID.MatchString(r.SessionID) {
		return common.ErrBadRequest
	}
	return nil
}

type SessionRevocationWriter interface {
	RevokeSession(context.Context, SessionRevocation) (bool, error)
	RevokeUserSessions(context.Context, SessionRevocation) (int64, error)
}
type RefreshFamilyRevocation struct {
	SessionID, FamilyID string
	At                  time.Time
}

func (r RefreshFamilyRevocation) Validate() error {
	if !canonicalUUID.MatchString(r.SessionID) || !canonicalUUID.MatchString(r.FamilyID) || r.At.IsZero() {
		return common.ErrBadRequest
	}
	return nil
}

type RefreshRevocationWriter interface {
	RevokeSessionRefresh(context.Context, string, time.Time) (int64, error)
	RevokeFamily(context.Context, RefreshFamilyRevocation) (int64, error)
	RevokeUserRefresh(context.Context, uint64, time.Time) (int64, error)
}
