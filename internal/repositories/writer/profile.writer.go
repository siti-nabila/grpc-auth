package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type ProfileWriter struct{ tx domain.Transaction }

func NewProfileWriterInTransaction(tx domain.Transaction) domain.ProfileWriter {
	return &ProfileWriter{tx: tx}
}

func (p *ProfileWriter) Create(req *domain.ProfileRequest) error {
	if p.tx == nil {
		return common.ErrInternalServerError
	}
	err := p.tx.Create(req)
	if er := helpers.HandleDBError(err); er != nil {
		return er
	}

	return nil
}

func (p *ProfileWriter) Update(req *domain.UpdateProfileRequest) error {
	if p.tx == nil {
		return common.ErrInternalServerError
	}
	err := p.tx.Update(req)
	if er := helpers.HandleDBError(err); er != nil {
		return er
	}

	return nil
}

func (p *ProfileWriter) Model() domain.ProfileRequest { return domain.ProfileRequest{} }

func (p *ProfileWriter) Patch(req map[string]any) error {
	if p.tx == nil {
		return common.ErrInternalServerError
	}
	return helpers.HandleDBError(p.tx.Update(p.Model(), req))
}
