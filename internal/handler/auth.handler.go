package handler

import (
	"context"
	"encoding/json"

	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pb/user"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	if errs := request.Validate(); errs != nil || len(errs) != 0 {
		errJson, _ := json.Marshal(errs)
		return nil, status.Errorf(codes.InvalidArgument, string(errJson))
	}

	token, err := feat.Register(request)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
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
	if errs := request.Validate(); errs != nil || len(errs) != 0 {
		errJson, _ := json.Marshal(errs)
		return nil, status.Errorf(codes.InvalidArgument, string(errJson))
	}

	feat := authfeature.NewAuthService(ctx)
	token, err := feat.Login(request)
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
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
