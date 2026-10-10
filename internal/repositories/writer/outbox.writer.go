package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type outboxWriter struct{ tx domain.Transaction }

func NewOutboxWriter(tx domain.Transaction) domain.OutboxWriter { return &outboxWriter{tx: tx} }
func (w *outboxWriter) Create(event domain.EventEnvelope) error {
	if w.tx == nil {
		return common.ErrInternalServerError
	}
	record, err := event.Record()
	if err != nil {
		return err
	}
	return helpers.HandleDBError(w.tx.Create(record))
}
