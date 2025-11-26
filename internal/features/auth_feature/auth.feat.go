package authfeature

import (
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
)

func (a *authService) Register(request domain.AuthRequest) (*string, error) {
	tx, err := a.authWriter.Begin()
	if err != nil {
		return nil, err
	}
	a.authWriter.UseTransaction(tx)

	defer deferTransaction(tx, &err)

	passCfg := DefaultArgon2Config()
	hashedPassword, err := passCfg.HashPassword(request.Password)
	if err != nil {
		return nil, err
	}

	req := &domain.AuthRequest{
		Email:    request.Email,
		Password: hashedPassword,
	}
	err = a.authWriter.RegisterTx(req)
	if err != nil {
		return nil, err
	}
	token, err := a.GenerateAuthToken(req.Id)
	if err != nil {
		return nil, err
	}

	return token, nil
}

func (a *authService) Login(request domain.AuthRequest) (*string, error) {
	userData, err := a.authReader.GetByEmail(request.Email)
	if err != nil {
		return nil, err
	}

	passCfg := DefaultArgon2Config()
	valid, err := passCfg.VerifyPassword(userData.Password, request.Password)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, dictionary.ErrPasswordMismatch
	}

	token, err := a.GenerateAuthToken(userData.Id)
	if err != nil {
		return nil, err
	}

	return token, nil
}
