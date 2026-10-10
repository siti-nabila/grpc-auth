package domain

var (
	MemberRole RoleCode = 1
	AdminRole  RoleCode = 2
)

type (
	RoleCode   uint64
	RoleWriter interface {
		Create(*RoleRequest) error
	}

	RoleReader interface {
		GetDefaultRole() (Role, error)
	}

	RoleRequest struct {
		Id   uint64 `sql:"column:id;primaryKey" json:"id"`
		Name string `sql:"column:role_name" json:"role_name"`
	}
	Role struct {
		Id          uint64  `sql:"column:id;primaryKey" json:"id"`
		Name        string  `sql:"column:role_name" json:"role_name"`
		Code        uint64  `sql:"column:role_code" json:"role_code"`
		Description *string `sql:"column:role_description" json:"role_description"`
	}

	RoleResponse struct {
		Id   uint64 `sql:"column:id;primaryKey" json:"id"`
		Name string `sql:"column:role_name" json:"role_name"`
		Code uint64 `sql:"column:role_code" json:"role_code"`
	}
)

func (Role) TableName() string {
	return "role"
}
