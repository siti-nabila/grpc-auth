package mocks

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/query"
)

type PermissionSnapshot struct {
	Provider domain.ReadSnapshotProvider
	PanicAt  int
	CancelAt int
	Cancel   context.CancelFunc
}

func (s PermissionSnapshot) BeginSnapshot(ctx context.Context) (domain.Transaction, error) {
	tx, err := s.Provider.BeginSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	return &permissionTransaction{Transaction: tx, snapshot: s}, nil
}

type permissionTransaction struct {
	domain.Transaction
	snapshot PermissionSnapshot
	queries  int
}

func (t *permissionTransaction) UseModel(model any) *query.QueryBuilder {
	t.queries++
	if t.queries == t.snapshot.PanicAt {
		panic("controlled permission query panic")
	}
	if t.queries == t.snapshot.CancelAt {
		t.snapshot.Cancel()
	}
	return t.Transaction.UseModel(model)
}
