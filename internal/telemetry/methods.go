package telemetry

// MethodNames is the fixed registered descriptor inventory; callers receive a copy.
func MethodNames() [MethodCount]string {
	return [MethodCount]string{
		"/application.v1.ApplicationService/GrantApplicationAccess",
		"/application.v1.ApplicationService/ListApplications",
		"/application.v1.ApplicationService/RevokeApplicationAccess",
		"/audit.v1.AuditService/ListAuditLogs",
		"/auth.v1.AuthService/ChangePassword",
		"/auth.v1.AuthService/Logout",
		"/auth.v1.AuthService/RefreshSession",
		"/authorization.v1.AuthorizationService/EvaluateAccess",
		"/permission.v1.PermissionService/ListPermissions",
		"/profile.v1.ProfileService/UpdateProfile",
		"/role.v1.RoleService/ChangeRoleStatus",
		"/role.v1.RoleService/CreateRole",
		"/role.v1.RoleService/GetRole",
		"/role.v1.RoleService/ListRoles",
		"/role.v1.RoleService/ReplaceRolePermissions",
		"/role.v1.RoleService/UpdateRole",
		"/session.v1.SessionService/ListMySessions",
		"/session.v1.SessionService/ListUserSessions",
		"/session.v1.SessionService/RevokeAllMySessions",
		"/session.v1.SessionService/RevokeAllUserSessions",
		"/session.v1.SessionService/RevokeMySession",
		"/session.v1.SessionService/RevokeUserSession",
		"/user.v1.UserService/ChangeUserStatus",
		"/user.v1.UserService/CreateUser",
		"/user.v1.UserService/GetMyEffectivePermissions",
		"/user.v1.UserService/GetUser",
		"/user.v1.UserService/GetUserEffectivePermissions",
		"/user.v1.UserService/GrantUserRoles",
		"/user.v1.UserService/ListUserApplicationAccess",
		"/user.v1.UserService/ListUserRoles",
		"/user.v1.UserService/ListUsers",
		"/user.v1.UserService/Login",
		"/user.v1.UserService/Me",
		"/user.v1.UserService/Register",
		"/user.v1.UserService/RevokeUserRoles",
		"/user.v1.UserService/TesRPC",
		"/user.v1.UserService/UpdateUser",
		"unknown",
	}
}

func MethodIndex(method string) int {
	for i, m := range MethodNames() {
		if method == m {
			return i
		}
	}
	return MethodCount - 1
}
