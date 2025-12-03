package handler

import (
	"context"

	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pb/user"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"

	"google.golang.org/protobuf/types/known/emptypb"
)

type (
	UserHandler struct {
		user.UnimplementedUserServiceServer
	}
)

func (u *UserHandler) Register(ctx context.Context, in *user.AuthRequest) (*user.UserTokenResponse, error) {
	var (
		feat = authfeature.NewAuthService(ctx)
	)
	request := domain.AuthRequest{
		Email:    in.Email,
		Password: in.Password,
	}
	if err := request.Validate(); err != nil {
		return nil, helpers.HandleError(err)
	}

	token, err := feat.Register(request)
	if err != nil {
		return nil, helpers.HandleError(err)
	}

	return &user.UserTokenResponse{
		Token: *token,
	}, nil
}
func (u *UserHandler) Login(ctx context.Context, in *user.AuthRequest) (*user.UserTokenResponse, error) {
	request := domain.AuthRequest{
		Email:    in.Email,
		Password: in.Password,
	}
	if err := request.Validate(); err != nil {
		return nil, helpers.HandleError(err)
	}

	feat := authfeature.NewAuthService(ctx)
	token, err := feat.Login(request)
	if err != nil {
		return nil, helpers.HandleError(err)
	}

	return &user.UserTokenResponse{
		Token: *token,
	}, nil

}
func (u *UserHandler) TesRPC(context.Context, *emptypb.Empty) (*user.TestRPC, error) {
	return &user.TestRPC{
		Res: "WELCOME ANJING !!!!!!!",
	}, nil
}
