package configs

import (
	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"google.golang.org/grpc"
)

func RegisterAll(s *grpc.Server) {
	userv1.RegisterUserServiceServer(s, &handler.UserHandler{})
	profilev1.RegisterProfileServiceServer(s, &handler.ProfileHandler{})

}
