package common

import "github.com/siti-nabila/orm/orm"

func DeferTransaction(tx *orm.SqlTransactionAdapter, err *error) {
	if tx == nil {
		return
	}

	if *err != nil {
		_ = tx.Rollback()
		return
	}

	if commitErr := tx.Commit(); commitErr != nil {
		*err = commitErr
	}
}
