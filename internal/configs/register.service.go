package configs

import (
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"github.com/siti-nabila/grpc-auth/pb/user"
	"google.golang.org/grpc"
)

func RegisterAll(s *grpc.Server) {
	user.RegisterUserServiceServer(s, &handler.UserHandler{})

}
