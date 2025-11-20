package authfeature

import (
	"context"

	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
)

type (
	AuthService interface {
		Register(req domain.AuthRequest) (err error)
	}

	authService struct {
		authRepo domain.AuthWriter
	}
)

func NewAuthService(ctx context.Context) AuthService {
	return &authService{
		authRepo: writer.NewAuthWriter(ctx),
	}
}

func (a *authService) Register(request domain.AuthRequest) (err error) {
	tx, err := a.authRepo.Begin()
	if err != nil {
		return err
	}
	a.authRepo.UseTransaction(tx)

	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	req := &domain.AuthRequest{
		Email:    request.Email,
		Password: request.Password,
	}
	return a.authRepo.RegisterTx(req)
}
