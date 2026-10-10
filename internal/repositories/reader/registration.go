package reader

import (
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
)

const memberRoleCode = "iam.member"

type registrationRoleReader struct{ tx domain.Transaction }

func NewRegistrationRoleReader(tx domain.Transaction) domain.RegistrationRoleReader {
	return &registrationRoleReader{tx: tx}
}

func (r *registrationRoleReader) DefaultMember() (uint64, error) {
	if r.tx == nil {
		return 0, common.ErrDependencyUnavailable
	}
	var role domain.Role
	err := r.tx.UseModel(domain.Role{}).Select("role.id").Join("applications app", "app.id = role.application_id").
		Where("role.role_code = ?", domain.MemberRole).Where("role.code = ?", memberRoleCode).
		Where("role.status = ?", types.RoleActive).Where("role.is_system = ?", true).
		Where("app.code = ?", types.ApplicationIAM).Where("app.status = ?", types.ApplicationActive).
		Where("NOT EXISTS (SELECT 1 FROM role_permissions rp WHERE rp.role_id = role.id)").Limit(1).Scan(&role)
	if err != nil {
		return 0, helpers.HandleDBError(err)
	}
	return role.Id, nil
}
