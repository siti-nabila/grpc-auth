package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type initialRefreshWriter struct{ tx domain.Transaction }

func NewInitialRefreshWriter(tx domain.Transaction) domain.InitialRefreshWriter {
	return &initialRefreshWriter{tx: tx}
}

func (w *initialRefreshWriter) Create(refresh domain.InitialRefresh) error {
	if w.tx == nil {
		return common.ErrInternalServerError
	}
	if err := refresh.Validate(); err != nil {
		return err
	}
	return helpers.HandleDBError(w.tx.Create(&refresh))
}
