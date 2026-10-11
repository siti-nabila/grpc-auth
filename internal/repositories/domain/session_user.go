package domain

import (
	"context"
	"database/sql"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

// SessionUserState projects only the identity/status/counters needed by admin
// invalidation, without reading password or other credential material.
type SessionUserState struct {
	ID          uint64           `sql:"column:id"`
	Status      types.UserStatus `sql:"column:status"`
	Version     int64            `sql:"column:version"`
	AuthVersion int64            `sql:"column:auth_version" json:"-"`
	DeletedAt   sql.NullTime     `sql:"column:deleted_at"`
}

func (SessionUserState) TableName() string { return "auth" }

type SessionUserReader interface {
	GetByID(context.Context, uint64) (SessionUserState, error)
}
