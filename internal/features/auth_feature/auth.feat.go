package authfeature

import (
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pb/user"
	"github.com/siti-nabila/grpc-auth/pkg/dictionary"
	"github.com/siti-nabila/orm/orm"
	"golang.org/x/sync/errgroup"
)

func (a *authService) Register(request domain.AuthRequest) (token *string, err error) {
	var (
		tx *orm.SqlTransactionAdapter
	)
	defer func() {
		if tx != nil {
			common.DeferTransaction(tx, &err)
		}

		if r := recover(); r != nil {
			// Handle the panic and return an error
			err = dictionary.ErrInternalServerError
		}
	}()
	if _, err := a.authReader.GetByEmail(request.Email); err == nil {
		return nil, dictionary.ErrDataExists
	} else if err != dictionary.ErrNotFound {
		return nil, err
	}

	tx, err = a.authWriter.Begin()
	if err != nil {
		return nil, err
	}
	a.profileWriter.UseTransaction(tx)
	a.userRoleWriter.UseTransaction(tx)

	passCfg := DefaultArgon2Config()
	hashedPassword, err := passCfg.HashPassword(request.Password)
	if err != nil {
		return nil, err
	}

	role, err := a.roleReader.GetDefaultRole()
	if err != nil {
		return nil, err
	}
	req := &domain.AuthRequest{
		Email:    request.Email,
		Password: hashedPassword,
	}
	err = a.authWriter.Create(req)
	if err != nil {
		return nil, err
	}

	token, err = a.GenerateAuthToken(req.Id)
	if err != nil {
		return nil, err
	}

	g, _ := errgroup.WithContext(a.ctx)
	g.Go(func() error {
		profileRequest := &domain.ProfileRequest{
			UserId: req.Id,
		}
		err = a.profileWriter.Create(profileRequest)
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		userRoleRequest := &domain.UserRoleRequest{
			UserId: req.Id,
			RoleId: role.Id,
		}
		err = a.userRoleWriter.Create(userRoleRequest)
		if err != nil {
			return err
		}
		return nil
	})

	if err = g.Wait(); err != nil {
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

func (a *authService) GetUserData() (res user.UserData, err error) {
	var (
		authData    domain.Auth
		profileData domain.Profile
		RolesData   []domain.Role
	)
	roleIds := make([]uint64, 0)
	roleNames := make([]string, 0)
	userSession, err := sessions.GetUserSession(a.ctx)
	if err != nil {
		return user.UserData{}, err
	}

	g, _ := errgroup.WithContext(a.ctx)
	g.Go(func() error {
		authData, err = a.authReader.GetById(userSession.UserId)
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		profileData, err = a.profileReader.GetByUserId(userSession.UserId)
		if err != nil {
			return err
		}
		return nil
	})
	g.Go(func() error {
		RolesData, err = a.userRoleReader.GetRolesByUserId(userSession.UserId)
		if err != nil {
			return err
		}
		for _, role := range RolesData {
			roleIds = append(roleIds, role.Id)
			roleNames = append(roleNames, role.Name)
		}
		return nil
	})

	if err = g.Wait(); err != nil {
		return user.UserData{}, err
	}
	// protoProfile := profileData.ToProfileResponse()
	// protoAuth := authData.ToUserDataResponse()

	return user.UserData{
		Id:        authData.Id,
		Email:     authData.Email,
		Fullname:  profileData.Name,
		Address:   profileData.Address,
		Phone:     profileData.Phone,
		RoleIds:   roleIds,
		RoleNames: roleNames,
	}, nil
}
