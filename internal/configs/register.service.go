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
	login       *handler.SessionLoginHandler
	register    *handler.PendingRegistrationHandler
	reads       *handler.SessionReadHandler
	management  *handler.UserManagementHandler
	assignments *handler.UserRoleAssignmentHandler
	access      *handler.ApplicationAccessHandler
}

func (s *sessionUserServer) GetMyEffectivePermissions(ctx context.Context, r *userv1.GetMyEffectivePermissionsRequest) (*userv1.GetEffectivePermissionsResponse, error) {
	return s.reads.GetMyEffectivePermissions(ctx, r)
}

func (s *sessionUserServer) GetUserEffectivePermissions(ctx context.Context, r *userv1.GetUserEffectivePermissionsRequest) (*userv1.GetEffectivePermissionsResponse, error) {
	return s.reads.GetUserEffectivePermissions(ctx, r)
}

func (s *sessionUserServer) ListUserApplicationAccess(ctx context.Context, r *userv1.ListUserApplicationAccessRequest) (*userv1.ListUserApplicationAccessResponse, error) {
	return s.access.ListUserApplicationAccess(ctx, r)
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

func (s *sessionUserServer) GetUser(ctx context.Context, req *userv1.GetUserRequest) (*userv1.GetUserResponse, error) {
	return s.reads.GetUser(ctx, req)
}

func (s *sessionUserServer) TesRPC(ctx context.Context, req *emptypb.Empty) (*userv1.TestRPC, error) {
	return (&handler.TestRPCHandler{}).TesRPC(ctx, req)
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

func (s *sessionUserServer) CreateUser(ctx context.Context, r *userv1.CreateUserRequest) (*userv1.CreateUserResponse, error) {
	return s.management.CreateUser(ctx, r)
}
func (s *sessionUserServer) UpdateUser(ctx context.Context, r *userv1.UpdateUserRequest) (*userv1.UpdateUserResponse, error) {
	return s.management.UpdateUser(ctx, r)
}
func (s *sessionUserServer) ChangeUserStatus(ctx context.Context, r *userv1.ChangeUserStatusRequest) (*userv1.ChangeUserStatusResponse, error) {
	return s.management.ChangeUserStatus(ctx, r)
}

func (s *sessionUserServer) ListUserRoles(ctx context.Context, r *userv1.ListUserRolesRequest) (*userv1.ListUserRolesResponse, error) {
	return s.assignments.ListUserRoles(ctx, r)
}
func (s *sessionUserServer) GrantUserRoles(ctx context.Context, r *userv1.GrantUserRolesRequest) (*userv1.GrantUserRolesResponse, error) {
	return s.assignments.GrantUserRoles(ctx, r)
}
func (s *sessionUserServer) RevokeUserRoles(ctx context.Context, r *userv1.RevokeUserRolesRequest) (*userv1.RevokeUserRolesResponse, error) {
	return s.assignments.RevokeUserRoles(ctx, r)
}
