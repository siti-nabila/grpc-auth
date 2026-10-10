package writer

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

type registrationWriter struct{ tx domain.Transaction }
type registrationMemberWriter struct{ tx domain.Transaction }

func NewRegistrationWriter(tx domain.Transaction) domain.RegistrationWriter {
	return &registrationWriter{tx: tx}
}
func NewRegistrationMemberWriter(tx domain.Transaction) domain.RegistrationMemberWriter {
	return &registrationMemberWriter{tx: tx}
}

func (w *registrationWriter) Create(req *domain.PendingRegistration) error {
	if w.tx == nil {
		return common.ErrDependencyUnavailable
	}
	if req == nil || req.ID != 0 || req.Email == "" || req.Password == "" || req.Status != types.UserPending || req.Version != 1 || req.AuthVersion != 1 || req.MustChangePassword {
		return common.ErrBadRequest
	}
	return helpers.HandleDBError(w.tx.Create(req))
}

func (w *registrationMemberWriter) Assign(userID, roleID uint64) error {
	if w.tx == nil {
		return common.ErrDependencyUnavailable
	}
	if userID == 0 || roleID == 0 {
		return common.ErrBadRequest
	}
	return helpers.HandleDBError(w.tx.Create(&domain.RegistrationMember{UserID: userID, RoleID: roleID, Status: types.AssignmentActive}))
}
