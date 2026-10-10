package configs

import (
	"context"

	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type sessionUserServer struct {
	userv1.UnimplementedUserServiceServer
	login    *handler.SessionLoginHandler
	register *handler.PendingRegistrationHandler
	reads    *handler.SessionReadHandler
}

func (s *sessionUserServer) Login(ctx context.Context, req *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	return s.login.Login(ctx, req)
}
func (s *sessionUserServer) Register(ctx context.Context, req *userv1.AuthRequest) (*userv1.UserTokenResponse, error) {
	return s.register.Register(ctx, req)
}
func (s *sessionUserServer) Me(ctx context.Context, req *emptypb.Empty) (*userv1.UserData, error) {
	return s.reads.Me(ctx, req)
}
func (s *sessionUserServer) ListUsers(ctx context.Context, req *userv1.ListUsersRequest) (*userv1.ListUsersResponse, error) {
	return s.reads.ListUsers(ctx, req)
}

func (s *sessionUserServer) TesRPC(ctx context.Context, req *emptypb.Empty) (*userv1.TestRPC, error) {
	return (&handler.UserHandler{}).TesRPC(ctx, req)
}

type sessionProfileServer struct {
	profilev1.UnimplementedProfileServiceServer
	handler *handler.SessionProfileHandler
}

func (s *sessionProfileServer) UpdateProfile(ctx context.Context, req *profilev1.ProfileRequest) (*profilev1.ProfileResponse, error) {
	return s.handler.UpdateProfile(ctx, req)
}

func registerFoundation(s *grpc.Server, users *sessionUserServer, profile *sessionProfileServer) {
	userv1.RegisterUserServiceServer(s, users)
	profilev1.RegisterProfileServiceServer(s, profile)
}
