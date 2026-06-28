package authfeature

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pb/user"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

type (
	AuthService interface {
		Register(req domain.AuthRequest) (token *string, err error)
		Login(req domain.AuthRequest) (token *string, err error)
		GetUserData() (res user.UserData, err error)
	}

	authService struct {
		ctx            context.Context
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

func NewAuthService(ctx context.Context) AuthService {
	return &authService{
		ctx:            ctx,
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

func (a *authService) GenerateAuthToken(userId uint64) (*string, error) {
	jwtReq := jwt.JwtRequest{
		UserId: userId,
		Issuer: a.appCfg.ApplicationName,
		Secret: a.appCfg.JWT.SecretKey,
	}
	token, err := jwt.GenerateJWTToken(jwtReq)
	if err != nil {
		return nil, err
	}
	return &token, nil

}
