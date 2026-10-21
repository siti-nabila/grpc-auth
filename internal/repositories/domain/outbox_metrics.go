package domain

import (
	"context"
	"database/sql"
)

type PendingOutbox struct {
	Count  int64        `sql:"column:pending_count"`
	Oldest sql.NullTime `sql:"column:oldest_created_at"`
}

type OutboxMetricsReader interface {
	Pending(context.Context) (PendingOutbox, error)
}
