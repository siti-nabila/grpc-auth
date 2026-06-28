package user

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
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
		authWriter     domain.AuthWriter
		authReader     domain.AuthReader
		profileWriter  domain.ProfileWriter
		profileReader  domain.ProfileReader
		roleReader     domain.RoleReader
		userRoleWriter domain.UserRoleWriter
		userRoleReader domain.UserRoleReader
		appCfg         *config.AppConfig
	}
)

func NewUserService(ctx context.Context) *userService {
	return &userService{
		ctx:            ctx,
		userReader:     reader.NewUserReader(ctx),
		authWriter:     writer.NewAuthWriter(ctx),
		authReader:     reader.NewAuthReader(ctx),
		profileWriter:  writer.NewProfileWriter(ctx),
		profileReader:  reader.NewProfileReader(ctx),
		roleReader:     reader.NewRoleReader(ctx),
		userRoleWriter: writer.NewUserRoleWriter(ctx),
		userRoleReader: reader.NewUserRoleReader(ctx),
		appCfg:         config.GetAppConfig(),
	}
}
