package common

import (
	"context"
	"database/sql"
	"errors"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	normalizeerr "github.com/siti-nabila/orm/pkg/normalize_err"
)

func (o *TransactionOwner) TryLock(key string) (bool, error) {
	if o.closed {
		return false, sql.ErrTxDone
	}
	if err := o.ctx.Err(); err != nil {
		return false, err
	}
	acquired, err := o.tx.TryLock(o.ctx, key)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false, err
	}
	var dbErr *normalizeerr.DBError
	if errors.As(err, &dbErr) {
		return false, errors.Join(commondictionary.ErrDependencyUnavailable, err)
	}
	return acquired, err
}

func (o *TransactionOwner) Commit() error {
	if o.closed {
		return sql.ErrTxDone
	}
	if err := o.ctx.Err(); err != nil {
		return err
	}
	// A failed commit can have an unknown outcome. Never retry the mutation.
	err := o.tx.Commit()
	o.closed = true
	if err != nil {
		cleanupErr := o.tx.Rollback()
		if !errors.Is(cleanupErr, sql.ErrTxDone) {
			err = errors.Join(err, cleanupErr)
		}
	}
	return err
}

func (o *TransactionOwner) Rollback() error {
	if o.closed {
		return nil
	}
	o.closed = true
	err := o.tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}

// Finish must be deferred directly by the feature. It never commits.
func (o *TransactionOwner) Finish(err *error) {
	if recover() != nil {
		*err = commondictionary.ErrInternalServerError
	}
	if cleanupErr := o.Rollback(); cleanupErr != nil {
		*err = errors.Join(*err, cleanupErr)
	}
}
