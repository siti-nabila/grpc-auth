package configs

import (
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/pb/profile"
	"google.golang.org/grpc"
)

func RegisterAll(s *grpc.Server) {
	userv1.RegisterUserServiceServer(s, &handler.UserHandler{})
	profile.RegisterProfileServiceServer(s, &handler.ProfileHandler{})

}
