package domain

import (
	"context"
	"database/sql"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/orm/query"
)

// QueryReader is shared by primary adapters and feature-owned transaction handles.
// It cannot begin/end transactions or acquire locks.
type QueryReader interface{ UseModel(any) *query.QueryBuilder }

type SessionStateReader interface {
	GetSessionState(context.Context, string) (SessionState, error)
}
type SessionWriter interface{ Create(Session) error }

type Session struct {
	ID           string              `sql:"column:id;primaryKey"`
	UserID       uint64              `sql:"column:user_id"`
	Status       types.SessionStatus `sql:"column:status"`
	CreatedAt    time.Time           `sql:"column:created_at"`
	LastSeenAt   time.Time           `sql:"column:last_seen_at"`
	ExpiresAt    time.Time           `sql:"column:expires_at"`
	RevokedAt    sql.NullTime        `sql:"column:revoked_at"`
	RevokedBy    sql.NullInt64       `sql:"column:revoked_by"`
	RevokeReason sql.NullString      `sql:"column:revoke_reason"`
	DeviceLabel  sql.NullString      `sql:"column:device_label"`
	UserAgent    sql.NullString      `sql:"column:user_agent"`
	IPAddress    sql.NullString      `sql:"column:ip_address"`
	AuthVersion  int64               `sql:"column:auth_version"`
}

func (Session) TableName() string { return "sessions" }

// SessionState reads identity and session together without credential columns.
type SessionState struct {
	UserID             uint64              `sql:"column:user_id"`
	UserStatus         types.UserStatus    `sql:"column:user_status"`
	AuthVersion        int64               `sql:"column:auth_version"`
	UserVersion        int64               `sql:"column:user_version"`
	MustChangePassword bool                `sql:"column:must_change_password"`
	DeletedAt          sql.NullTime        `sql:"column:deleted_at"`
	SessionID          string              `sql:"column:session_id"`
	SessionUserID      uint64              `sql:"column:session_user_id"`
	SessionStatus      types.SessionStatus `sql:"column:session_status"`
	CreatedAt          time.Time           `sql:"column:created_at"`
	ExpiresAt          time.Time           `sql:"column:expires_at"`
	RevokedAt          sql.NullTime        `sql:"column:revoked_at"`
	SessionAuthVersion sql.NullInt64       `sql:"column:session_auth_version"`
}

func (SessionState) TableName() string { return "sessions s" }

func (s Session) Validate() error {
	if !canonicalUUID.MatchString(s.ID) || s.UserID == 0 || s.UserID > 2147483647 || s.Status != types.SessionActive || s.AuthVersion < 1 {
		return common.ErrBadRequest
	}
	if s.CreatedAt.IsZero() || !s.ExpiresAt.After(s.CreatedAt) || s.ExpiresAt.Sub(s.CreatedAt) > 7*24*time.Hour {
		return common.ErrBadRequest
	}
	if s.LastSeenAt.Before(s.CreatedAt) || s.LastSeenAt.After(s.ExpiresAt) || s.RevokedAt.Valid || s.RevokedBy.Valid || s.RevokeReason.Valid {
		return common.ErrBadRequest
	}
	for _, field := range []struct {
		value sql.NullString
		max   int
	}{{s.DeviceLabel, 200}, {s.UserAgent, 1000}, {s.IPAddress, 45}} {
		if field.value.Valid && !boundedText(field.value.String, field.max, false) {
			return common.ErrBadRequest
		}
	}
	return nil
}
