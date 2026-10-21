package domain

import (
	"context"
	"database/sql"
	"time"

	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// LoginAccount is credential-only input, never a public response or audit DTO.
type LoginAccount struct {
	ID                 uint64           `sql:"column:id;primaryKey"`
	Email              string           `sql:"column:email"`
	Password           string           `sql:"column:password" json:"-"`
	Status             types.UserStatus `sql:"column:status"`
	AuthVersion        int64            `sql:"column:auth_version"`
	Version            int64            `sql:"column:version"`
	MustChangePassword bool             `sql:"column:must_change_password"`
	DeletedAt          sql.NullTime     `sql:"column:deleted_at"`
}

func (LoginAccount) TableName() string { return "auth" }

type LoginAccountReader interface {
	GetByEmail(context.Context, string) (LoginAccount, error)
	GetByID(context.Context, uint64) (LoginAccount, error)
}

type InitialRefresh struct {
	ID         string         `sql:"column:id;primaryKey"`
	SessionID  string         `sql:"column:session_id"`
	TokenHash  []byte         `sql:"column:token_hash" json:"-"`
	FamilyID   string         `sql:"column:family_id"`
	IssuedAt   time.Time      `sql:"column:issued_at"`
	ExpiresAt  time.Time      `sql:"column:expires_at"`
	ConsumedAt sql.NullTime   `sql:"column:consumed_at"`
	RevokedAt  sql.NullTime   `sql:"column:revoked_at"`
	ReplacedBy sql.NullString `sql:"column:replaced_by"`
}

func (InitialRefresh) TableName() string { return "refresh_tokens" }

type InitialRefreshWriter interface{ Create(InitialRefresh) error }

func (r InitialRefresh) Validate() error {
	if !canonicalUUID.MatchString(r.ID) || !canonicalUUID.MatchString(r.SessionID) || !canonicalUUID.MatchString(r.FamilyID) || len(r.TokenHash) != 32 || r.IssuedAt.IsZero() || !r.ExpiresAt.After(r.IssuedAt) || r.ExpiresAt.Sub(r.IssuedAt) > 7*24*time.Hour || r.ConsumedAt.Valid || r.RevokedAt.Valid || r.ReplacedBy.Valid {
		return common.ErrBadRequest
	}
	return nil
}
