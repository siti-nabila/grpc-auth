package domain

import (
	"github.com/siti-nabila/orm/orm"
)

type (
	UserRoleWriter interface {
		UseTransaction(tx *orm.SqlTransactionAdapter)
		Begin() (*orm.SqlTransactionAdapter, error)

		Create(*UserRoleRequest) error
	}

	UserRoleReader interface {
		GetRolesByUserId(id uint64) ([]Role, error)
	}

	UserRoleRequest struct {
		Id     uint64 `sql:"column:id;primaryKey" json:"id"`
		UserId uint64 `sql:"column:user_id" json:"user_id"`
		RoleId uint64 `sql:"column:role_id" json:"role_id"`
	}
	UserRole struct {
		Id     uint64 `sql:"column:id;primaryKey" json:"id"`
		UserId uint64 `sql:"column:user_id" json:"user_id"`
		RoleId uint64 `sql:"column:role_id" json:"role_id"`
	}

	UserRoleResponse struct {
		Id     uint64 `sql:"column:id;primaryKey" json:"id"`
		UserId uint64 `sql:"column:user_id" json:"user_id"`
		RoleId uint64 `sql:"column:role_id" json:"role_id"`
	}
)

func (UserRoleRequest) TableName() string {
	return "user_role"
}

func (UserRole) TableName() string {
	return "user_role"
}
