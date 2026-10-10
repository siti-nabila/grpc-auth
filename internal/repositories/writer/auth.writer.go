package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type AuthWriter struct{ tx domain.Transaction }

func NewAuthWriterInTransaction(tx domain.Transaction) domain.AuthWriter { return &AuthWriter{tx: tx} }

func (a *AuthWriter) Create(req *domain.AuthRequest) error {
	if a.tx == nil {
		return common.ErrInternalServerError
	}
	err := a.tx.Create(req)
	if er := helpers.HandleDBError(err); er != nil {
		return er
	}

	return nil
}
