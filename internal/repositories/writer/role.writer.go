package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type RoleWriter struct{ tx domain.Transaction }

func NewRoleWriterInTransaction(tx domain.Transaction) domain.RoleWriter { return &RoleWriter{tx: tx} }
func (p *RoleWriter) Model() domain.RoleRequest                          { return domain.RoleRequest{} }
func (p *RoleWriter) Create(req *domain.RoleRequest) error {
	if p.tx == nil {
		return common.ErrInternalServerError
	}
	return helpers.HandleDBError(p.tx.Create(req))
}
