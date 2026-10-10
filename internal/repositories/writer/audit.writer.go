package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type auditWriter struct{ tx domain.Transaction }

func NewAuditWriter(tx domain.Transaction) domain.AuditWriter { return &auditWriter{tx: tx} }
func (w *auditWriter) Create(request domain.AuditRequest) error {
	if w.tx == nil {
		return common.ErrInternalServerError
	}
	record, err := request.Record()
	if err != nil {
		return err
	}
	return helpers.HandleDBError(w.tx.Create(record))
}
