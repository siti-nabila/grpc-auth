package authfeature

import (
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/jwt"
)

func (a *authService) Register(request domain.AuthRequest) (token string, err error) {
	tx, err := a.authRepo.Begin()
	if err != nil {
		return "", err
	}
	a.authRepo.UseTransaction(tx)

	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()
	passCfg := DefaultArgon2Config()
	hashedPassword, err := passCfg.HashPassword(request.Password)
	if err != nil {
		return "", err
	}

	req := &domain.AuthRequest{
		Email:    request.Email,
		Password: hashedPassword,
	}
	err = a.authRepo.RegisterTx(req)
	if err != nil {
		return "", err
	}
	jwtReq := jwt.JwtRequest{
		UserId: req.Id,
		Issuer: a.appCfg.ApplicationName,
		Secret: a.appCfg.JWT.SecretKey,
	}
	token, err = jwt.GenerateJWTToken(jwtReq)
	if err != nil {
		return "", err
	}

	return token, nil
}
