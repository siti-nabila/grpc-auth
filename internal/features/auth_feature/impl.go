package authfeature

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/config"
)

type (
	AuthService interface {
		Register(req domain.AuthRequest) (token string, err error)
	}

	authService struct {
		authRepo domain.AuthWriter
		appCfg   *config.AppConfig
	}
)

func NewAuthService(ctx context.Context) AuthService {
	return &authService{
		authRepo: writer.NewAuthWriter(ctx),
		appCfg:   config.GetAppConfig(),
	}
}
