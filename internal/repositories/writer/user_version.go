package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type userVersionWriter struct{ tx domain.Transaction }

func NewUserVersionWriter(tx domain.Transaction) domain.UserVersionWriter {
	return &userVersionWriter{tx: tx}
}

func (w *userVersionWriter) SetVersion(userID uint64, version int64) error {
	if w.tx == nil {
		return common.ErrDependencyUnavailable
	}
	if userID == 0 || version < 1 {
		return common.ErrBadRequest
	}
	return helpers.HandleDBError(w.tx.Update(&domain.UserVersionUpdate{ID: userID, Version: version}))
}
