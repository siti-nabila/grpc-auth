package domain

import "github.com/siti-nabila/grpc-auth/pkg/types"

// UserIdentity contains only fields permitted in a self-service response.
type UserIdentity struct {
	ID                 uint64           `sql:"column:id;primaryKey"`
	Email              string           `sql:"column:email"`
	Status             types.UserStatus `sql:"column:status"`
	Version            int64            `sql:"column:version"`
	MustChangePassword bool             `sql:"column:must_change_password"`
}

func (UserIdentity) TableName() string { return "auth" }
