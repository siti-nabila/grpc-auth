package handler

import (
	"context"
	"strings"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"

	"google.golang.org/protobuf/types/known/emptypb"
)

type (
	UserHandler struct {
		userv1.UnimplementedUserServiceServer
		NewAuthService func(context.Context) authfeature.AuthService
	}
)

func (u *UserHandler) Register(ctx context.Context, in *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	request := domain.AuthRequest{
		Email:    strings.ToLower(strings.TrimSpace(in.Email)),
		Password: in.Password,
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	feat := u.authService(ctx)

	token, err := feat.Register(request)
	if err != nil {
		return nil, err
	}

	return &userv1.UserTokenResponse{
		Token: *token,
	}, nil
}
func (u *UserHandler) Login(ctx context.Context, in *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	if in == nil {
		return nil, common.ErrBadRequest
	}
	request := domain.AuthRequest{
		Email:    strings.ToLower(strings.TrimSpace(in.Email)),
		Password: in.Password,
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}

	feat := u.authService(ctx)
	token, err := feat.Login(request)
	if err != nil {
		return nil, err
	}

	return &userv1.UserTokenResponse{
		Token: *token,
	}, nil

}

func (u *UserHandler) Me(ctx context.Context, in *emptypb.Empty) (*userv1.UserData, error) {
	feat := u.authService(ctx)
	data, err := feat.GetUserData()
	if err != nil {
		return nil, err
	}
	return &data, nil
}

func (u *UserHandler) authService(ctx context.Context) authfeature.AuthService {
	if u.NewAuthService != nil {
		return u.NewAuthService(ctx)
	}
	return authfeature.NewAuthService(ctx)
}

func (u *UserHandler) TesRPC(context.Context, *emptypb.Empty) (*userv1.TestRPC, error) {
	return &userv1.TestRPC{
		Res: "WELCOME ANJING !!!!!!!",
	}, nil
}
