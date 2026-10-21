package user

import (
	"context"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/orm/orm"
)

type UserService interface {
	SearchUsers(domain.UserListRequest) (orm.PageData[domain.UserSearchRow], error)
}

type userService struct {
	ctx        context.Context
	userReader domain.UserReader
}
