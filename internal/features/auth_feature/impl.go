package authfeature

import (
	"context"
	"database/sql"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/config"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

type (
	AuthService interface {
		Register(req domain.AuthRequest) (token *string, err error)
		Login(req domain.AuthRequest) (token *string, err error)
	}

	authService struct {
		authWriter domain.AuthWriter
		authReader domain.AuthReader
		appCfg     *config.AppConfig
	}
)

func NewAuthService(ctx context.Context) AuthService {
	return &authService{
		authWriter: writer.NewAuthWriter(ctx),
		authReader: reader.NewAuthReader(ctx),
		appCfg:     config.GetAppConfig(),
	}
}

func deferTransaction(tx *sql.Tx, err *error) {
	if *err != nil {
		tx.Rollback()
	} else {
		tx.Commit()
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
