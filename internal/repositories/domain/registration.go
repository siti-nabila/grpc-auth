package domain

import "github.com/siti-nabila/grpc-auth/pkg/types"

type PendingRegistration struct {
	ID                 uint64           `sql:"column:id;primaryKey"`
	Email              string           `sql:"column:email"`
	Password           string           `sql:"column:password"`
	Status             types.UserStatus `sql:"column:status"`
	Version            int64            `sql:"column:version"`
	AuthVersion        int64            `sql:"column:auth_version"`
	MustChangePassword bool             `sql:"column:must_change_password"`
}

func (PendingRegistration) TableName() string { return "auth" }

type RegistrationWriter interface {
	Create(*PendingRegistration) error
}
type RegistrationRoleReader interface{ DefaultMember() (uint64, error) }
type RegistrationMemberWriter interface {
	Assign(userID, roleID uint64) error
}

type RegistrationMember struct {
	ID     uint64                 `sql:"column:id;primaryKey"`
	UserID uint64                 `sql:"column:user_id"`
	RoleID uint64                 `sql:"column:role_id"`
	Status types.AssignmentStatus `sql:"column:status"`
}

func (RegistrationMember) TableName() string { return "user_role" }
