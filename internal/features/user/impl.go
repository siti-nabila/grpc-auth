package user

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/orm/orm"
)

type (
	UserService interface {
		SearchUsers(req domain.UserListRequest) (orm.PageData[domain.UserSearchRow], error)
	}

	userService struct {
		ctx            context.Context
		userReader     domain.UserReader
		authReader     domain.AuthReader
		profileReader  domain.ProfileReader
		roleReader     domain.RoleReader
		userRoleReader domain.UserRoleReader
		appCfg         *config.AppConfig
	}
)

func NewUserService(ctx context.Context) *userService {
	return &userService{
		ctx:            ctx,
		userReader:     reader.NewUserReader(ctx),
		authReader:     reader.NewAuthReader(ctx),
		profileReader:  reader.NewProfileReader(ctx),
		roleReader:     reader.NewRoleReader(ctx),
		userRoleReader: reader.NewUserRoleReader(ctx),
		appCfg:         config.GetAppConfig(),
	}
}
