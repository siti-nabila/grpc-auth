package mocks

import (
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
)

type CommittedAcknowledgement struct {
	domain.Transaction
	Calls     []string
	Committed bool
	Failure   error
}

func (t *CommittedAcknowledgement) Commit() error {
	t.Calls = append(t.Calls, "commit")
	t.Committed = true
	return t.Failure
}

func (t *CommittedAcknowledgement) Rollback() error {
	t.Calls = append(t.Calls, "rollback")
	return sql.ErrTxDone
}
