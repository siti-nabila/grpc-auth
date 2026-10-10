package authfeature

import (
	"errors"
	"strings"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	authdictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
)

func (a *authService) Register(request domain.AuthRequest) (token *string, err error) {
	defer func() {
		if recover() != nil {
			err = commondictionary.ErrInternalServerError
		}
		if err != nil {
			token = nil
		}
	}()
	if err = a.ctx.Err(); err != nil {
		return nil, err
	}
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if err = request.Validate(); err != nil {
		return nil, err
	}
	hashedPassword, err := a.register.HashPassword(request.Password)
	if err != nil {
		return nil, err
	}
	tx, err := a.register.Transactions.Begin(a.ctx)
	if err != nil {
		return nil, err
	}
	owner := common.NewTransactionOwner(a.ctx, tx)
	defer owner.Finish(&err)
	if err = common.AcquireLocks(owner, authdictionary.ErrResourceBusy, common.RegisterLockKeys(request.Email)...); err != nil {
		return nil, err
	}
	repos := a.register.Bind(tx)
	if _, err = repos.AuthReader.GetByEmail(request.Email); err == nil {
		return nil, authdictionary.ErrDataExists
	} else if !errors.Is(err, authdictionary.ErrNotFound) {
		return nil, err
	}
	role, err := repos.RoleReader.GetDefaultRole()
	if err != nil {
		return nil, err
	}
	req := &domain.AuthRequest{Email: request.Email, Password: hashedPassword}
	if err = repos.AuthWriter.Create(req); err != nil {
		return nil, err
	}
	if err = repos.ProfileWriter.Create(&domain.ProfileRequest{UserId: req.Id}); err != nil {
		return nil, err
	}
	if err = repos.UserRoleWriter.Create(&domain.UserRoleRequest{UserId: req.Id, RoleId: role.Id}); err != nil {
		return nil, err
	}
	token, err = a.register.GenerateToken(req.Id)
	if err != nil {
		return nil, err
	}
	if token == nil || *token == "" {
		return nil, commondictionary.ErrInternalServerError
	}
	if err = owner.Commit(); err != nil {
		return nil, err
	}
	return token, nil
}

func (a *authService) Login(request domain.AuthRequest) (*string, error) {
	request.Email = strings.ToLower(strings.TrimSpace(request.Email))
	if err := request.Validate(); err != nil {
		return nil, err
	}
	userData, err := a.login.Reader.GetByEmail(request.Email)
	if err != nil {
		return nil, err
	}

	valid, err := a.login.VerifyPassword(userData.Password, request.Password)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, authdictionary.ErrPasswordMismatch
	}

	token, err := a.login.GenerateToken(userData.Id)
	if err != nil {
		return nil, err
	}

	return token, nil
}

// The legacy consumer is retained until coordinated activation. Reads are
// sequential to avoid sharing errors and slices across goroutines. The prepared
// session consumer uses ReadUserDataSnapshot for authoritative snapshot reads.
func (a *authService) GetUserData() (userv1.UserData, error) {
	userSession, err := sessions.GetUserSession(a.ctx)
	if err != nil {
		return userv1.UserData{}, err
	}
	authData, err := a.authReader.GetById(userSession.UserId)
	if err != nil {
		return userv1.UserData{}, err
	}
	profileData, err := a.profileReader.GetByUserId(userSession.UserId)
	if err != nil {
		return userv1.UserData{}, err
	}
	roles, err := a.userRoleReader.GetRolesByUserId(userSession.UserId)
	if err != nil {
		return userv1.UserData{}, err
	}
	codes := make([]int32, 0, len(roles))
	names := make([]string, 0, len(roles))
	for _, role := range roles {
		codes = append(codes, int32(role.Code))
		names = append(names, role.Name)
	}
	return userv1.UserData{Id: authData.Id, Email: authData.Email, Fullname: profileData.Name, Address: profileData.Address, Phone: profileData.Phone, RoleCodes: codes, RoleNames: names}, nil
}
