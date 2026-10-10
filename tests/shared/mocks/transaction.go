package mocks

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/query"
)

type Transaction struct {
	Calls         []string
	Context       context.Context
	CommitError   error
	RollbackError error
	LockError     error
	Busy          bool
	Hook          func(string)
}

func (t *Transaction) Record(call string) {
	t.Calls = append(t.Calls, call)
	if t.Hook != nil {
		t.Hook(call)
	}
}
func (t *Transaction) Commit() error   { t.Record("commit"); return t.CommitError }
func (t *Transaction) Rollback() error { t.Record("rollback"); return t.RollbackError }
func (t *Transaction) TryLock(ctx context.Context, key string) (bool, error) {
	if ctx != t.Context {
		panic("unexpected lock context")
	}
	t.Record("lock:" + key)
	return !t.Busy, t.LockError
}
func (*Transaction) UseModel(any) *query.QueryBuilder    { panic("unexpected query on feature mock") }
func (*Transaction) Create(any) error                    { panic("unexpected create on feature mock") }
func (*Transaction) Update(any, ...map[string]any) error { panic("unexpected update on feature mock") }

type TransactionProvider struct {
	Tx    *Transaction
	Error error
	Calls int
}

func (p *TransactionProvider) Begin(ctx context.Context) (domain.Transaction, error) {
	p.Calls++
	if ctx != p.Tx.Context {
		panic("unexpected begin context")
	}
	p.Tx.Record("begin")
	if p.Error != nil {
		return nil, p.Error
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.Tx, nil
}
