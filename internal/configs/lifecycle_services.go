package configs

import (
	"context"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/grpc-auth/internal/handler"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type sessionAuthServer struct {
	authv1.UnimplementedAuthServiceServer
	refresh  *handler.RefreshHandler
	logout   *handler.LogoutHandler
	password *handler.ChangePasswordHandler
}

func (s *sessionAuthServer) RefreshSession(ctx context.Context, req *authv1.RefreshSessionRequest) (*userv1.UserTokenResponse, error) {
	return s.refresh.RefreshSession(ctx, req)
}
func (s *sessionAuthServer) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	return s.logout.Logout(ctx, req)
}
func (s *sessionAuthServer) ChangePassword(ctx context.Context, req *authv1.ChangePasswordRequest) (*authv1.ChangePasswordResponse, error) {
	return s.password.ChangePassword(ctx, req)
}

type sessionManagementServer struct {
	sessionv1.UnimplementedSessionServiceServer
	self  *handler.SelfSessionHandler
	lists *handler.ListSessionsHandler
	admin *handler.AdminSessionsHandler
}

func (s *sessionManagementServer) ListMySessions(ctx context.Context, req *sessionv1.ListMySessionsRequest) (*sessionv1.ListSessionsResponse, error) {
	return s.lists.ListMySessions(ctx, req)
}
func (s *sessionManagementServer) ListUserSessions(ctx context.Context, req *sessionv1.ListUserSessionsRequest) (*sessionv1.ListSessionsResponse, error) {
	return s.lists.ListUserSessions(ctx, req)
}
func (s *sessionManagementServer) RevokeMySession(ctx context.Context, req *sessionv1.RevokeMySessionRequest) (*sessionv1.RevokeSessionResponse, error) {
	return s.self.RevokeMySession(ctx, req)
}
func (s *sessionManagementServer) RevokeAllMySessions(ctx context.Context, req *emptypb.Empty) (*sessionv1.RevokeAllMySessionsResponse, error) {
	return s.self.RevokeAllMySessions(ctx, req)
}
func (s *sessionManagementServer) RevokeUserSession(ctx context.Context, req *sessionv1.RevokeUserSessionRequest) (*sessionv1.RevokeSessionResponse, error) {
	return s.admin.RevokeUserSession(ctx, req)
}
func (s *sessionManagementServer) RevokeAllUserSessions(ctx context.Context, req *sessionv1.RevokeAllUserSessionsRequest) (*sessionv1.RevokeAllUserSessionsResponse, error) {
	return s.admin.RevokeAllUserSessions(ctx, req)
}
func registerLifecycle(s *grpc.Server, auth *sessionAuthServer, sessions *sessionManagementServer) {
	authv1.RegisterAuthServiceServer(s, auth)
	sessionv1.RegisterSessionServiceServer(s, sessions)
}
