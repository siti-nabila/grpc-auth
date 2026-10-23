package domain

import "github.com/siti-nabila/grpc-auth/pkg/types"

// SelfRole is safe metadata independent of whether the role maps any permission.
// Readers apply active application/access gates; the feature scopes namespaces.
type SelfRole struct {
	ID              uint64                 `sql:"column:role_id"`
	Code            string                 `sql:"column:code"`
	Name            string                 `sql:"column:name"`
	Status          types.AssignmentStatus `sql:"column:status"`
	RoleStatus      types.RoleStatus       `sql:"column:role_status"`
	ApplicationCode types.ApplicationCode  `sql:"column:application_code"`
}

func (SelfRole) TableName() string { return "user_role ur" }

type SelfApplicationAccess struct {
	ID                uint64                        `sql:"column:application_id"`
	Code              types.ApplicationCode         `sql:"column:code"`
	Name              string                        `sql:"column:name"`
	Status            types.ApplicationAccessStatus `sql:"column:status"`
	ApplicationStatus types.ApplicationStatus       `sql:"column:application_status"`
}

func (SelfApplicationAccess) TableName() string { return "user_application_access access" }
