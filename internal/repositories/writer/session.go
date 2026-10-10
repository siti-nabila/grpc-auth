package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type sessionWriter struct{ tx domain.Transaction }

func NewSessionWriter(tx domain.Transaction) domain.SessionWriter { return &sessionWriter{tx: tx} }

func (w *sessionWriter) Create(session domain.Session) error {
	if w.tx == nil {
		return common.ErrInternalServerError
	}
	if err := session.Validate(); err != nil {
		return err
	}
	return helpers.HandleDBError(w.tx.Create(&session))
}
