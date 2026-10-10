package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type UserRoleWriter struct{ tx domain.Transaction }

func NewUserRoleWriterInTransaction(tx domain.Transaction) domain.UserRoleWriter {
	return &UserRoleWriter{tx: tx}
}

func (p *UserRoleWriter) Create(req *domain.UserRoleRequest) error {
	if p.tx == nil {
		return common.ErrInternalServerError
	}
	err := p.tx.Create(req)
	if er := helpers.HandleDBError(err); er != nil {
		return er
	}

	return nil
}

func (p *UserRoleWriter) Model() domain.UserRoleRequest { return domain.UserRoleRequest{} }
