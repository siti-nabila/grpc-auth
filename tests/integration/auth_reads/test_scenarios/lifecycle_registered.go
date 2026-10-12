package test_scenarios

import (
	"bytes"
	"context"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"testing"
	"time"
)

func RegisteredLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "all twenty seven registered RPCs enforce TLS service caller and exact allowlists before persistence", Run: registeredLifecycleBoundary}, {Name: "nine lifecycle credential failures preserve localized catalog and independent safe service audit", Run: registeredLifecycleFailure}, {Name: "all eight role catalog methods reject caller namespace before session or target reads", Run: registeredRoleNamespace}}
}

type lifecycleRPC struct {
	method            string
	request, response proto.Message
}

func lifecycleRPCs() []lifecycleRPC {
	return []lifecycleRPC{
		{authv1.AuthService_RefreshSession_FullMethodName, &authv1.RefreshSessionRequest{RefreshToken: "malformed"}, &userv1.UserTokenResponse{}},
		{authv1.AuthService_Logout_FullMethodName, &authv1.LogoutRequest{}, &authv1.LogoutResponse{}},
		{authv1.AuthService_ChangePassword_FullMethodName, &authv1.ChangePasswordRequest{}, &authv1.ChangePasswordResponse{}},
		{sessionv1.SessionService_ListMySessions_FullMethodName, &sessionv1.ListMySessionsRequest{}, &sessionv1.ListSessionsResponse{}},
		{sessionv1.SessionService_RevokeMySession_FullMethodName, &sessionv1.RevokeMySessionRequest{SessionId: shared.SessionID}, &sessionv1.RevokeSessionResponse{}},
		{sessionv1.SessionService_RevokeAllMySessions_FullMethodName, &emptypb.Empty{}, &sessionv1.RevokeAllMySessionsResponse{}},
		{sessionv1.SessionService_ListUserSessions_FullMethodName, &sessionv1.ListUserSessionsRequest{UserId: 1}, &sessionv1.ListSessionsResponse{}},
		{sessionv1.SessionService_RevokeUserSession_FullMethodName, &sessionv1.RevokeUserSessionRequest{UserId: 1, SessionId: shared.SessionID}, &sessionv1.RevokeSessionResponse{}},
		{sessionv1.SessionService_RevokeAllUserSessions_FullMethodName, &sessionv1.RevokeAllUserSessionsRequest{UserId: 1}, &sessionv1.RevokeAllUserSessionsResponse{}},
	}
}
func registeredLifecycleBoundary(t *testing.T) {
	mock := &mocks.SQL{}
	db := mock.Open()
	t.Cleanup(func() { db.Close() })
	conn, foundation := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)))
	inventory := grpc.NewServer()
	foundation.Pipeline.Register(inventory)
	defer inventory.Stop()
	methods := 0
	for _, service := range inventory.GetServiceInfo() {
		methods += len(service.Methods)
	}
	if len(inventory.GetServiceInfo()) != 7 || methods != 27 {
		t.Fatal("incomplete default RPC inventory", methods)
	}
	all := append(lifecycleRPCs(), []lifecycleRPC{
		{userv1.UserService_Login_FullMethodName, &userv1.AuthRequest{}, &userv1.UserTokenResponse{}},
		{userv1.UserService_Register_FullMethodName, &userv1.AuthRequest{}, &userv1.UserTokenResponse{}},
		{userv1.UserService_Me_FullMethodName, &emptypb.Empty{}, &userv1.UserData{}},
		{userv1.UserService_TesRPC_FullMethodName, &emptypb.Empty{}, &userv1.TestRPC{}},
		{userv1.UserService_ListUsers_FullMethodName, &userv1.ListUsersRequest{}, &userv1.ListUsersResponse{}},
		{profilev1.ProfileService_UpdateProfile_FullMethodName, &profilev1.ProfileRequest{}, &profilev1.ProfileResponse{}},
	}...)
	all = append(all, managementRPCs()...)
	all = append(all, roleRPCs()...)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, call := range all {
		if e := conn.Invoke(ctx, call.method, call.request, call.response); status.Code(e) != codes.Unauthenticated {
			t.Fatal("anonymous boundary", call.method, e)
		}
	}
	for _, call := range append(append(lifecycleRPCs(), managementRPCs()...), roleRPCs()...) {
		e := conn.Invoke(shared.RPCContext(ctx, ""), call.method, call.request, call.response)
		if status.Code(e) != codes.PermissionDenied {
			t.Fatal("new RPC bypassed caller allowlist", call.method, e)
		}
	}
	if foundation.Login.Limiter.Counts().Entries != 0 {
		t.Fatal("unverified/forbidden caller allocated quota")
	}
	if e := mock.Verify(); e != nil {
		t.Fatal(e)
	}
}

func roleRPCs() []lifecycleRPC {
	return []lifecycleRPC{
		{rolev1.RoleService_ListRoles_FullMethodName, &rolev1.ListRolesRequest{}, &rolev1.ListRolesResponse{}},
		{rolev1.RoleService_GetRole_FullMethodName, &rolev1.GetRoleRequest{}, &rolev1.GetRoleResponse{}},
		{rolev1.RoleService_CreateRole_FullMethodName, &rolev1.CreateRoleRequest{}, &rolev1.CreateRoleResponse{}},
		{rolev1.RoleService_UpdateRole_FullMethodName, &rolev1.UpdateRoleRequest{}, &rolev1.UpdateRoleResponse{}},
		{rolev1.RoleService_ChangeRoleStatus_FullMethodName, &rolev1.ChangeRoleStatusRequest{}, &rolev1.ChangeRoleStatusResponse{}},
		{rolev1.RoleService_ReplaceRolePermissions_FullMethodName, &rolev1.ReplaceRolePermissionsRequest{}, &rolev1.ReplaceRolePermissionsResponse{}},
		{permissionv1.PermissionService_ListPermissions_FullMethodName, &permissionv1.ListPermissionsRequest{}, &permissionv1.ListPermissionsResponse{}},
		{applicationv1.ApplicationService_ListApplications_FullMethodName, &applicationv1.ListApplicationsRequest{}, &applicationv1.ListApplicationsResponse{}},
	}
}
func registeredLifecycleFailure(t *testing.T) {
	steps := []mocks.SQLStep{}
	for _, call := range lifecycleRPCs() {
		steps = append(steps, registeredAttempt(call.method, "AUTH_INVALID_SESSION")...)
	}
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	t.Cleanup(func() { db.Close() })
	conn, foundation := shared.FoundationServer(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), shared.LifecycleMethods()...)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, call := range lifecycleRPCs() {
		e := conn.Invoke(shared.RPCContext(ctx, ""), call.method, call.request, call.response)
		decoded, ok := grpcerror.Decode(e)
		if !ok || decoded.Message != auth.ErrInvalidSession.Message(locale.Indonesian) || status.Code(e) != codes.Unauthenticated {
			t.Fatal("credential failure mapping", call.method, e)
		}
	}
	if foundation.Login.Limiter.Counts().Entries != 1 || foundation.Login.AuditFailures() != 0 {
		t.Fatal("refresh unknown bucket/audit outcome")
	}
	if e := mock.Verify(); e != nil {
		t.Fatal(e)
	}
}

func managementRPCs() []lifecycleRPC {
	return []lifecycleRPC{
		{userv1.UserService_GetUser_FullMethodName, &userv1.GetUserRequest{}, &userv1.GetUserResponse{}},
		{userv1.UserService_CreateUser_FullMethodName, &userv1.CreateUserRequest{}, &userv1.CreateUserResponse{}},
		{userv1.UserService_UpdateUser_FullMethodName, &userv1.UpdateUserRequest{}, &userv1.UpdateUserResponse{}},
		{userv1.UserService_ChangeUserStatus_FullMethodName, &userv1.ChangeUserStatusRequest{}, &userv1.ChangeUserStatusResponse{}},
	}
}

func registeredRoleNamespace(t *testing.T) {
	steps := []mocks.SQLStep{}
	service := shared.ServiceConfig()
	service.Callers[0].Namespaces = []string{"clm"}
	for _, call := range roleRPCs() {
		steps = append(steps, registeredAttempt(call.method, "COMMON_FORBIDDEN")...)
		service.Callers[0].Methods = append(service.Callers[0].Methods, call.method)
	}
	mock := &mocks.SQL{Steps: steps}
	db := mock.Open()
	t.Cleanup(func() { db.Close() })
	conn, foundation := shared.FoundationServerWithService(t, db, func() time.Time { return shared.ServiceTime }, bytes.NewReader(make([]byte, 2048)), service)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, call := range roleRPCs() {
		if e := conn.Invoke(shared.RPCContext(ctx, ""), call.method, call.request, call.response); status.Code(e) != codes.PermissionDenied {
			t.Fatal("namespace bypass", call.method, e)
		}
		for _, tokens := range [][]string{{"invalid"}, {shared.ServiceToken(1), shared.ServiceToken(1)}} {
			bad := metadata.NewOutgoingContext(ctx, metadata.MD{"x-service-token": tokens})
			if e := conn.Invoke(bad, call.method, call.request, call.response); status.Code(e) != codes.Unauthenticated {
				t.Fatal("invalid service credential", call.method, e)
			}
		}
	}
	if e := mock.Verify(); e != nil {
		t.Fatal(e)
	}
	if foundation.Login.Limiter.Counts().Entries != 0 || foundation.Login.AuditFailures() != 0 {
		t.Fatal("denied namespace allocated quota or failed audit")
	}
}
