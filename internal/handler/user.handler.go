package handler

import (
	"context"

	"github.com/siti-nabila/grpc-auth/pb/user"
	"google.golang.org/protobuf/types/known/emptypb"
)

type (
	UserHandler struct {
		user.UnimplementedUserServiceServer
	}
)

func (u *UserHandler) Register(context.Context, *user.AuthRequest) (*user.UserTokenResponse, error) {
	return nil, nil
}
func (u *UserHandler) Login(context.Context, *user.AuthRequest) (*user.UserTokenResponse, error) {
	return nil, nil
}
func (u *UserHandler) TesRPC(context.Context, *emptypb.Empty) (*user.TestRPC, error) {
	return &user.TestRPC{
		Res: "WELCOME ANJING !!!!!!!",
	}, nil
}
