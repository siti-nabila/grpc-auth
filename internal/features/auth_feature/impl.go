package authfeature

import (
	"context"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

type (
	AuthService interface {
		Register(req domain.AuthRequest) (token *string, err error)
		Login(req domain.AuthRequest) (token *string, err error)
		GetUserData() (res userv1.UserData, err error)
	}

	authService struct {
		ctx            context.Context
		authReader     domain.AuthReader
		profileReader  domain.ProfileReader
		roleReader     domain.RoleReader
		userRoleReader domain.UserRoleReader
		appCfg         *config.AppConfig
		register       RegisterDependencies
		login          LoginDependencies
	}
)

func NewAuthService(ctx context.Context) AuthService {
	passCfg := DefaultArgon2Config()
	deps := RegisterDependencies{
		Transactions: database.NewTransactionProvider(database.DBGetNativePool(database.UserDbSource), database.GetDialect(database.UserDbSource), database.GetORMConfig()),
		Bind:         BindRegisterRepositories,
		HashPassword: passCfg.HashPassword,
	}
	service := &authService{
		ctx:            ctx,
		authReader:     reader.NewAuthReader(ctx),
		profileReader:  reader.NewProfileReader(ctx),
		roleReader:     reader.NewRoleReader(ctx),
		userRoleReader: reader.NewUserRoleReader(ctx),
		appCfg:         config.GetAppConfig(),
		register:       deps,
	}
	service.register.GenerateToken = service.GenerateAuthToken
	service.login = LoginDependencies{Reader: service.authReader, VerifyPassword: passCfg.VerifyPassword, GenerateToken: service.GenerateAuthToken}
	return service
}

type RegisterRepositories struct {
	AuthReader     domain.AuthReader
	RoleReader     domain.RoleReader
	AuthWriter     domain.AuthWriter
	ProfileWriter  domain.ProfileWriter
	UserRoleWriter domain.UserRoleWriter
}

type RegisterDependencies struct {
	Transactions  domain.TransactionProvider
	Bind          func(domain.Transaction) RegisterRepositories
	HashPassword  func(string) (string, error)
	GenerateToken func(uint64) (*string, error)
}

type RegisterService interface {
	Register(domain.AuthRequest) (*string, error)
}

type LoginService interface {
	Login(domain.AuthRequest) (*string, error)
}
type LoginDependencies struct {
	Reader         domain.AuthReader
	VerifyPassword func(string, string) (bool, error)
	GenerateToken  func(uint64) (*string, error)
}

func NewLoginServiceWithDependencies(ctx context.Context, deps LoginDependencies) LoginService {
	return &authService{ctx: ctx, login: deps}
}

func NewRegisterServiceWithDependencies(ctx context.Context, deps RegisterDependencies) RegisterService {
	return &authService{ctx: ctx, register: deps}
}

func BindRegisterRepositories(tx domain.Transaction) RegisterRepositories {
	return RegisterRepositories{
		AuthReader: reader.NewAuthReaderInTransaction(tx), RoleReader: reader.NewRoleReaderInTransaction(tx),
		AuthWriter: writer.NewAuthWriterInTransaction(tx), ProfileWriter: writer.NewProfileWriterInTransaction(tx), UserRoleWriter: writer.NewUserRoleWriterInTransaction(tx),
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
