package common

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

// TransactionOwner is request-local; the feature chooses when to commit.
type TransactionOwner struct {
	ctx    context.Context
	tx     domain.Transaction
	closed bool
}

func NewTransactionOwner(ctx context.Context, tx domain.Transaction) *TransactionOwner {
	return &TransactionOwner{ctx: ctx, tx: tx}
}
