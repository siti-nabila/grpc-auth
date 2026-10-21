package common

import (
	"context"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

// RecordSecurityAttempt owns a separate transaction after the command outcome.
func RecordSecurityAttempt(ctx context.Context, provider domain.TransactionProvider, request domain.AuditRequest) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if provider == nil {
		return common.ErrDependencyUnavailable
	}
	tx, err := provider.Begin(ctx)
	if err != nil {
		return DependencyError(err)
	}
	owner := NewTransactionOwner(ctx, tx)
	defer owner.Finish(&err)
	if err = writer.NewAuditWriter(tx).Create(request); err != nil {
		return DependencyError(err)
	}
	return DependencyError(owner.Commit())
}
