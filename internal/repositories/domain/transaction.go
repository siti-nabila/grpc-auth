package domain

import (
	"context"
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
