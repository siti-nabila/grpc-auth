package reader

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
)

type transactionAuthReader struct{ tx domain.Transaction }
type transactionProfileReader struct{ tx domain.Transaction }
type transactionRoleReader struct{ tx domain.Transaction }

func NewAuthReaderInTransaction(tx domain.Transaction) domain.AuthReader {
	return &transactionAuthReader{tx: tx}
}
func NewProfileReaderInTransaction(tx domain.Transaction) domain.ProfileReader {
	return &transactionProfileReader{tx: tx}
}
func NewRoleReaderInTransaction(tx domain.Transaction) domain.RoleReader {
	return &transactionRoleReader{tx: tx}
}

func (r *transactionAuthReader) GetByEmail(email string) (result domain.Auth, err error) {
	if r.tx == nil {
		return result, common.ErrInternalServerError
	}
	err = r.tx.UseModel(domain.Auth{}).Where("email = ?", email).Limit(1).Scan(&result)
	return result, helpers.HandleDBError(err)
}

func (r *transactionAuthReader) GetById(id uint64) (result domain.Auth, err error) {
	if r.tx == nil {
		return result, common.ErrInternalServerError
	}
	err = r.tx.UseModel(domain.Auth{}).Where("id = ?", id).Limit(1).Scan(&result)
	return result, helpers.HandleDBError(err)
}

func (r *transactionProfileReader) GetByUserId(id uint64) (result domain.Profile, err error) {
	if r.tx == nil {
		return result, common.ErrInternalServerError
	}
	err = r.tx.UseModel(domain.Profile{}).Where("user_id = ?", id).First(&result)
	return result, helpers.HandleDBError(err)
}

func (r *transactionRoleReader) GetDefaultRole() (result domain.Role, err error) {
	if r.tx == nil {
		return result, common.ErrInternalServerError
	}
	err = r.tx.UseModel(domain.Role{}).Where("role_code = ?", domain.MemberRole).Limit(1).Scan(&result)
	return result, helpers.HandleDBError(err)
}
