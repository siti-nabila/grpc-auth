package domain

import (
	"context"
	"database/sql"
	"github.com/siti-nabila/orm/query"
)

// Transaction is the existing ORM adapter contract used by one command.
type Transaction interface {
	Commit() error
	Rollback() error
	TryLock(context.Context, string) (bool, error)
	UseModel(any) *query.QueryBuilder
	Create(any) error
	Update(any, ...map[string]any) error
}

type TransactionProvider interface {
	Begin(context.Context) (Transaction, error)
}

// ReadSnapshotProvider leaves lifecycle ownership in the feature. Mutation
// providers retain ReadCommitted; this path is only for consistent read RPCs.
type ReadSnapshotProvider interface {
	BeginSnapshot(context.Context) (Transaction, error)
}

// StatementWriter is a capability of the supplied primary transaction. The ORM
// has no delete builder; mapping replacement needs a parameterized DELETE on
// this same handle, without creating another pool or owning its lifecycle.
type StatementWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
