package handler

import (
	"context"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TestRPCHandler struct{}

func (*TestRPCHandler) TesRPC(context.Context, *emptypb.Empty) (*userv1.TestRPC, error) {
	return &userv1.TestRPC{Res: "WELCOME ANJING !!!!!!!"}, nil
}
