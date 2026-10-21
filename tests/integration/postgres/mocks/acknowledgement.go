//go:build postgres

package mocks

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

// CommitAcknowledgement loses one acknowledgement only after the real commit.
type CommitAcknowledgement struct {
	Inner    domain.TransactionProvider
	Failure  error
	FailNext bool
	Commits  int
}

func (p *CommitAcknowledgement) Begin(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.Inner.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &acknowledgementTransaction{Transaction: tx, owner: p}, nil
}

type acknowledgementTransaction struct {
	domain.Transaction
	owner *CommitAcknowledgement
}

func (tx *acknowledgementTransaction) Commit() error {
	tx.owner.Commits++
	if err := tx.Transaction.Commit(); err != nil {
		return err
	}
	if tx.owner.FailNext {
		tx.owner.FailNext = false
		return tx.owner.Failure
	}
	return nil
}
