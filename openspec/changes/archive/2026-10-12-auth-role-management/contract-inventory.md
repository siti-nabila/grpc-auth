# Contract B inventory

Baseline: `github.com/siti-nabila/api-contracts` commit `7297589746962a3d994cf850a623502562017db0`, branch `feat/user-err`, clean tree at inspection. Parent workspace links this checkout; grpc-auth currently pins this published baseline.

## Existing methods (19)

| Full method | Input | Output |
| --- | --- | --- |
| /auth.v1.AuthService/RefreshSession | .auth.v1.RefreshSessionRequest | .user.v1.UserTokenResponse |
| /auth.v1.AuthService/Logout | .auth.v1.LogoutRequest | .auth.v1.LogoutResponse |
| /auth.v1.AuthService/ChangePassword | .auth.v1.ChangePasswordRequest | .auth.v1.ChangePasswordResponse |
| /profile.v1.ProfileService/UpdateProfile | .profile.v1.ProfileRequest | .profile.v1.ProfileResponse |
| /session.v1.SessionService/ListMySessions | .session.v1.ListMySessionsRequest | .session.v1.ListSessionsResponse |
| /session.v1.SessionService/RevokeMySession | .session.v1.RevokeMySessionRequest | .session.v1.RevokeSessionResponse |
| /session.v1.SessionService/RevokeAllMySessions | .google.protobuf.Empty | .session.v1.RevokeAllMySessionsResponse |
| /session.v1.SessionService/ListUserSessions | .session.v1.ListUserSessionsRequest | .session.v1.ListSessionsResponse |
| /session.v1.SessionService/RevokeUserSession | .session.v1.RevokeUserSessionRequest | .session.v1.RevokeSessionResponse |
| /session.v1.SessionService/RevokeAllUserSessions | .session.v1.RevokeAllUserSessionsRequest | .session.v1.RevokeAllUserSessionsResponse |
| /user.v1.UserService/Register | .user.v1.AuthRequest | .user.v1.UserTokenResponse |
| /user.v1.UserService/Login | .user.v1.AuthRequest | .user.v1.UserTokenResponse |
| /user.v1.UserService/Me | .google.protobuf.Empty | .user.v1.UserData |
| /user.v1.UserService/ListUsers | .user.v1.ListUsersRequest | .user.v1.ListUsersResponse |
| /user.v1.UserService/GetUser | .user.v1.GetUserRequest | .user.v1.GetUserResponse |
| /user.v1.UserService/CreateUser | .user.v1.CreateUserRequest | .user.v1.CreateUserResponse |
| /user.v1.UserService/UpdateUser | .user.v1.UpdateUserRequest | .user.v1.UpdateUserResponse |
| /user.v1.UserService/ChangeUserStatus | .user.v1.ChangeUserStatusRequest | .user.v1.ChangeUserStatusResponse |
| /user.v1.UserService/TesRPC | .google.protobuf.Empty | .user.v1.TestRPC |

All existing RPCs are unary. No role/permission/application package or planned method exists in the baseline. The new files allocate tags only inside new messages, so no old tag is reused. Exact baseline FileDescriptorProto snapshots are captured before edits for regression tests.

## Existing public fields

| Message.field | Tag | Type | Label | Presence |
| --- | --- | --- | --- | --- |
| auth.v1.RefreshSessionRequest.refresh_token | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| auth.v1.LogoutRequest.refresh_token | 1 | TYPE_STRING | LABEL_OPTIONAL | True |
| auth.v1.LogoutResponse.changed | 1 | TYPE_BOOL | LABEL_OPTIONAL | False |
| auth.v1.ChangePasswordRequest.current_password | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| auth.v1.ChangePasswordRequest.new_password | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| auth.v1.ChangePasswordResponse.changed | 1 | TYPE_BOOL | LABEL_OPTIONAL | False |
| auth.v1.ChangePasswordResponse.login_required | 2 | TYPE_BOOL | LABEL_OPTIONAL | False |
| paginator.v1.PageQuery.page | 1 | TYPE_INT32 | LABEL_OPTIONAL | False |
| paginator.v1.PageQuery.limit | 2 | TYPE_INT32 | LABEL_OPTIONAL | False |
| paginator.v1.PageQuery.fields | 3 | TYPE_STRING | LABEL_REPEATED | False |
| paginator.v1.PageQuery.sort | 4 | .paginator.v1.Sort | LABEL_REPEATED | False |
| paginator.v1.PageQuery.search | 5 | .paginator.v1.Search | LABEL_OPTIONAL | False |
| paginator.v1.PageQuery.last_id | 6 | TYPE_STRING | LABEL_OPTIONAL | False |
| paginator.v1.PageMeta.page | 1 | TYPE_INT32 | LABEL_OPTIONAL | False |
| paginator.v1.PageMeta.limit | 2 | TYPE_INT32 | LABEL_OPTIONAL | False |
| paginator.v1.PageMeta.total | 3 | TYPE_INT64 | LABEL_OPTIONAL | False |
| paginator.v1.PageMeta.total_pages | 4 | TYPE_INT32 | LABEL_OPTIONAL | False |
| paginator.v1.PageMeta.has_next | 5 | TYPE_BOOL | LABEL_OPTIONAL | False |
| paginator.v1.PageMeta.has_prev | 6 | TYPE_BOOL | LABEL_OPTIONAL | False |
| paginator.v1.Sort.field | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| paginator.v1.Sort.desc | 2 | TYPE_BOOL | LABEL_OPTIONAL | False |
| paginator.v1.Search.fields | 1 | TYPE_STRING | LABEL_REPEATED | False |
| paginator.v1.Search.keyword | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| paginator.v1.Search.mode | 3 | .paginator.v1.SearchMode | LABEL_OPTIONAL | False |
| profile.v1.Profile.id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| profile.v1.Profile.user_id | 2 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| profile.v1.Profile.name | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| profile.v1.Profile.address | 4 | TYPE_STRING | LABEL_OPTIONAL | False |
| profile.v1.Profile.phone | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| profile.v1.ProfileRequest.name | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| profile.v1.ProfileRequest.address | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| profile.v1.ProfileRequest.phone | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| profile.v1.ProfileRequest.field_mask | 4 | .google.protobuf.FieldMask | LABEL_OPTIONAL | False |
| profile.v1.ProfileResponse.profile | 1 | .profile.v1.Profile | LABEL_OPTIONAL | False |
| profile.v1.ProfileResponse.version | 2 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| profile.v1.ProfileResponse.changed | 3 | TYPE_BOOL | LABEL_OPTIONAL | False |
| session.v1.Session.id | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.Session.user_id | 2 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| session.v1.Session.status | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.Session.created_at | 4 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| session.v1.Session.last_seen_at | 5 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| session.v1.Session.expires_at | 6 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| session.v1.Session.revoked_at | 7 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| session.v1.Session.device_label | 8 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.Session.user_agent | 9 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.Session.ip_address | 10 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.Session.is_current | 11 | TYPE_BOOL | LABEL_OPTIONAL | False |
| session.v1.ListMySessionsRequest.query | 1 | .paginator.v1.PageQuery | LABEL_OPTIONAL | False |
| session.v1.ListUserSessionsRequest.user_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| session.v1.ListUserSessionsRequest.query | 2 | .paginator.v1.PageQuery | LABEL_OPTIONAL | False |
| session.v1.ListSessionsResponse.items | 1 | .session.v1.Session | LABEL_REPEATED | False |
| session.v1.ListSessionsResponse.meta | 2 | .paginator.v1.PageMeta | LABEL_OPTIONAL | False |
| session.v1.ListSessionsResponse.next_cursor | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.RevokeMySessionRequest.session_id | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.RevokeUserSessionRequest.user_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| session.v1.RevokeUserSessionRequest.session_id | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| session.v1.RevokeSessionResponse.changed | 1 | TYPE_BOOL | LABEL_OPTIONAL | False |
| session.v1.RevokeAllMySessionsResponse.changed | 1 | TYPE_BOOL | LABEL_OPTIONAL | False |
| session.v1.RevokeAllMySessionsResponse.login_required | 2 | TYPE_BOOL | LABEL_OPTIONAL | False |
| session.v1.RevokeAllUserSessionsRequest.user_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| session.v1.RevokeAllUserSessionsRequest.expected_user_version | 2 | TYPE_UINT64 | LABEL_OPTIONAL | True |
| session.v1.RevokeAllUserSessionsResponse.changed | 1 | TYPE_BOOL | LABEL_OPTIONAL | False |
| session.v1.RevokeAllUserSessionsResponse.version | 2 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.User.id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.User.name | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.User.email | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.User.password | 4 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.User.dob | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.User.phone_no | 6 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.User.address | 7 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.User.status | 8 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.User.version | 9 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.User.created_at | 10 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.User.must_change_password | 11 | TYPE_BOOL | LABEL_OPTIONAL | False |
| user.v1.User.roles | 12 | .user.v1.UserRoleAssignment | LABEL_REPEATED | False |
| user.v1.User.application_access | 13 | .user.v1.UserApplicationAccess | LABEL_REPEATED | False |
| user.v1.UserResponse.id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UserResponse.email | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserResponse.created_at | 3 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.UserData.id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UserData.email | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserData.fullname | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserData.address | 4 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserData.phone | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserData.role_codes | 6 | TYPE_INT32 | LABEL_REPEATED | False |
| user.v1.UserData.role_names | 7 | TYPE_STRING | LABEL_REPEATED | False |
| user.v1.UserData.created_at | 8 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.UserData.status | 9 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserData.version | 10 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UserData.roles | 11 | .user.v1.UserRoleAssignment | LABEL_REPEATED | False |
| user.v1.UserData.application_access | 12 | .user.v1.UserApplicationAccess | LABEL_REPEATED | False |
| user.v1.UserData.must_change_password | 13 | TYPE_BOOL | LABEL_OPTIONAL | False |
| user.v1.UserRoleAssignment.role_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UserRoleAssignment.code | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserRoleAssignment.name | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserRoleAssignment.status | 4 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserRoleAssignment.role_status | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserApplicationAccess.application_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UserApplicationAccess.code | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserApplicationAccess.name | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserApplicationAccess.status | 4 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserApplicationAccess.application_status | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserDataOld.user | 1 | .user.v1.UserResponse | LABEL_OPTIONAL | False |
| user.v1.UserDataOld.profile | 2 | .profile.v1.Profile | LABEL_OPTIONAL | False |
| user.v1.AuthRequest.email | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.AuthRequest.password | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.token | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.registration_id | 2 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.registration_status | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.refresh_token | 4 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.session_id | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.access_expires_at | 6 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.session_expires_at | 7 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.UserTokenResponse.must_change_password | 8 | TYPE_BOOL | LABEL_OPTIONAL | False |
| user.v1.CreateUserRequest.name | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.CreateUserRequest.email | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.CreateUserRequest.password | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.CreateUserRequest.status | 4 | TYPE_STRING | LABEL_OPTIONAL | True |
| user.v1.CreateUserRequest.address | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.CreateUserRequest.phone | 6 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.CreateUserResponse.user | 1 | .user.v1.User | LABEL_OPTIONAL | False |
| user.v1.GetUserRequest.id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.GetUserResponse.user | 1 | .user.v1.User | LABEL_OPTIONAL | False |
| user.v1.TestRPC.res | 1 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.ListUsersRequest.query | 1 | .paginator.v1.PageQuery | LABEL_OPTIONAL | False |
| user.v1.ListUsersRequest.filter | 2 | .user.v1.UserFilter | LABEL_OPTIONAL | False |
| user.v1.UserListItem.id | 1 | TYPE_INT32 | LABEL_OPTIONAL | False |
| user.v1.UserListItem.email | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserListItem.name | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserListItem.address | 4 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserListItem.phone | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserListItem.status | 6 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserListItem.version | 7 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UserListItem.roles | 8 | .user.v1.UserRoleAssignment | LABEL_REPEATED | False |
| user.v1.UserListItem.application_access | 9 | .user.v1.UserApplicationAccess | LABEL_REPEATED | False |
| user.v1.UserListItem.created_at | 10 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.ListUsersResponse.items | 1 | .user.v1.UserListItem | LABEL_REPEATED | False |
| user.v1.ListUsersResponse.total | 2 | TYPE_INT32 | LABEL_OPTIONAL | False |
| user.v1.ListUsersResponse.page | 3 | TYPE_INT32 | LABEL_OPTIONAL | False |
| user.v1.ListUsersResponse.limit | 4 | TYPE_INT32 | LABEL_OPTIONAL | False |
| user.v1.ListUsersResponse.total_pages | 5 | TYPE_INT32 | LABEL_OPTIONAL | False |
| user.v1.ListUsersResponse.has_next | 6 | TYPE_BOOL | LABEL_OPTIONAL | False |
| user.v1.ListUsersResponse.has_prev | 7 | TYPE_BOOL | LABEL_OPTIONAL | False |
| user.v1.ListUsersResponse.next_cursor | 8 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UserFilter.created_from | 1 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.UserFilter.created_to | 2 | .google.protobuf.Timestamp | LABEL_OPTIONAL | False |
| user.v1.UserFilter.role_codes | 3 | TYPE_UINT64 | LABEL_REPEATED | False |
| user.v1.UserFilter.statuses | 4 | TYPE_STRING | LABEL_REPEATED | False |
| user.v1.UserFilter.application_code | 5 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.UpdateUserRequest.user_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.UpdateUserRequest.expected_user_version | 2 | TYPE_UINT64 | LABEL_OPTIONAL | True |
| user.v1.UpdateUserRequest.name | 3 | TYPE_STRING | LABEL_OPTIONAL | True |
| user.v1.UpdateUserRequest.address | 4 | TYPE_STRING | LABEL_OPTIONAL | True |
| user.v1.UpdateUserRequest.phone | 5 | TYPE_STRING | LABEL_OPTIONAL | True |
| user.v1.UpdateUserRequest.field_mask | 6 | .google.protobuf.FieldMask | LABEL_OPTIONAL | False |
| user.v1.UpdateUserResponse.user | 1 | .user.v1.User | LABEL_OPTIONAL | False |
| user.v1.UpdateUserResponse.changed | 2 | TYPE_BOOL | LABEL_OPTIONAL | False |
| user.v1.ChangeUserStatusRequest.user_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.ChangeUserStatusRequest.status | 2 | TYPE_STRING | LABEL_OPTIONAL | True |
| user.v1.ChangeUserStatusRequest.reason | 3 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.ChangeUserStatusRequest.expected_user_version | 4 | TYPE_UINT64 | LABEL_OPTIONAL | True |
| user.v1.ChangeUserStatusResponse.user_id | 1 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.ChangeUserStatusResponse.status | 2 | TYPE_STRING | LABEL_OPTIONAL | False |
| user.v1.ChangeUserStatusResponse.version | 3 | TYPE_UINT64 | LABEL_OPTIONAL | False |
| user.v1.ChangeUserStatusResponse.changed | 4 | TYPE_BOOL | LABEL_OPTIONAL | False |

## Source fingerprints

| Proto source | SHA-256 |
| --- | --- |
| proto/auth/v1/auth.payload.proto | 606010569d1056d00abae6971bad86fcfcb1df1b74f931e5dd6ba896a1801570 |
| proto/auth/v1/auth.service.proto | 3eb98c2b738e11629fcd629a7c6bf6bff0667c281246f24ca05cd8f415920f59 |
| proto/paginator/v1/paginator.payload.proto | e800f5e2e719da238066e7d69c809e73ff979feadb2819ebb4b1e203b6f78719 |
| proto/profile/v1/profile.payload.proto | 360fbe660c3d74fc989fb28873e639a90d443d535f93da4cee8255777802c014 |
| proto/profile/v1/profile.service.proto | 8390e38a4c05a80bfb843c8ec4bbab66dd9390e543e4d3fe7c3feaff4c2f4e77 |
| proto/session/v1/session.payload.proto | a27d777a806d9eeedc8b41708de9b8596d3f23cc926c8d4d2d1a4ef16f5def25 |
| proto/session/v1/session.service.proto | d2ae713fe9539cd780d3273832450bb13473aa7e999f02b4c6958f81dffa3e22 |
| proto/user/v1/user.payload.proto | 1730461d4c2c12f262ced381bcd68f8ef687d4142905a26ccf237c9e4c3f2cc8 |
| proto/user/v1/user.service.proto | fdb46800bd7c0cce9f6eb9adeb4d95aabf7a190f8159e44c64e2f195b9a94851 |

## Contract B field allocation

| Message | Field number/type yang direncanakan |
| --- | --- |
| RoleSummary | id1 uint64, code2 string, name3 string, description4 optional string, application_id5 uint64, application_code6 string, status7 string, is_system8 bool, version9 uint64, role_code10 int32, created_at11 Timestamp, updated_at12 Timestamp, affected_user_count13 uint64 |
| PermissionItem | id1 uint64, application_id2 uint64, application_code3 string, code4 string, description5 string |
| ApplicationItem | id1 uint64, code2 string, name3 string, status4 string, is_system5 bool |
| RoleFilter | application_code1 string, statuses2 repeated string |
| ApplicationFilter | statuses1 repeated string |
| ListRolesRequest | query1 PageQuery, filter2 RoleFilter |
| ListPermissionsRequest | query1 PageQuery, application_code2 string |
| ListApplicationsRequest | query1 PageQuery, filter2 ApplicationFilter |
| Each list response | items1 repeated relevant item, meta2 PageMeta, next_cursor3 string |
| GetRoleRequest / GetRoleResponse | role_id1 uint64 / role1 RoleSummary, permissions2 repeated PermissionItem |
| CreateRoleRequest | application_code1 string, code2 string, name3 string, description4 optional string, permission_codes5 repeated string |
| CreateRoleResponse | role1 RoleSummary, permissions2 repeated PermissionItem |
| UpdateRoleRequest | role_id1 uint64, expected_role_version2 optional uint64, name3 optional string, description4 optional string, field_mask5 FieldMask |
| UpdateRoleResponse | role1 RoleSummary, changed2 bool |
| ChangeRoleStatusRequest | role_id1 uint64, status2 optional string, reason3 string, expected_role_version4 optional uint64 |
| ChangeRoleStatusResponse | role_id1 uint64, status2 string, version3 uint64, changed4 bool |
| PermissionSet | codes1 repeated string |
| ReplaceRolePermissionsRequest | role_id1 uint64, permissions2 PermissionSet, reason3 string, expected_role_version4 optional uint64 |
| ReplaceRolePermissionsResponse | role1 RoleSummary, permissions2 repeated PermissionItem, changed3 bool |



## Error semantics

New qualified role errors: role_not_found→NOT_FOUND, role_code_already_exists→ALREADY_EXISTS, system_role_protected→FAILED_PRECONDITION, invalid_permission_set→INVALID_ARGUMENT, version_conflict/idempotency_conflict/resource_busy→ABORTED. Application errors: application_not_found→NOT_FOUND, application_inactive/system_application_protected→FAILED_PRECONDITION. Common validation, forbidden, dependency, deadline and cancellation semantics are reused. Existing auth/user/common mappings remain unchanged. Every new definition supplies en/id, regional base fallback and English fallback. Wrapping preserves source identity/cause; wire encoding deliberately contains only the safe catalog definition.

## Verification and publication

Task 1.1: baseline source/descriptors inspected; 19 existing methods, eight planned additions absent, new tag allocation collision-free. Compatibility tests, generation stability and contract verification PASS; publication PASS; see evidence. Backend implementation remains NOT_STARTED. User subsequently authorized commit/push on feat/role-management; published commit/version/origin/checksums are recorded in evidence.

## Verified generated hashes

Two `make proto` runs produced identical hashes for all22 files (13 existing, nine new). Existing generated files are unchanged in Git.

| Generated file | SHA-256 |
| --- | --- |
| pb/application/v1/application.payload.pb.go | 0aaa93bbdc659d731551219c0adcfb1fe16d170d72b152d1fc6bc7c30bfb5516 |
| pb/application/v1/application.service.pb.go | a6a425bdc7ebb807f9e798ad7d3508189151c205f304eead27d0d13d25e0c3e9 |
| pb/application/v1/application.service_grpc.pb.go | 0e01a272c4cf0af65aee43a06acb55e386f420aedb4941fe9f24d607d6e05500 |
| pb/auth/v1/auth.payload.pb.go | ffc61b656d92169886c0b8d8476ce3efdb5a3ef34b6b4aec42118ca8b4b49ff8 |
| pb/auth/v1/auth.service.pb.go | 519660058ba7c52c1825b58fa86579964a8171dd16e962a624084eec53421917 |
| pb/auth/v1/auth.service_grpc.pb.go | 24ef3844b9432d2e0bbc8ea40c3865d7e29291fc924c15e6af02966b1471ab8b |
| pb/paginator/v1/paginator.payload.pb.go | 424dbdcfeb2ff90c58a1a09961e4bfc7ecade7ae515d4cec747bc6f7865074f4 |
| pb/permission/v1/permission.payload.pb.go | 92028dcadaac43c1582cbeafad4a090dd6c9dea398cd0ce02c8526e7af41ecec |
| pb/permission/v1/permission.service.pb.go | d3162a02fb0cca3ce62e2a5cfdb8fccc75244a8ff0f223e16fe08891f1c30dce |
| pb/permission/v1/permission.service_grpc.pb.go | d02dbec4ef34d295d3219680b4b1385109301e56f71ad897251a0b77611d42be |
| pb/profile/v1/profile.payload.pb.go | 3cb2ce3d462ab579e287bcc38def6a5b7b5feab43a977dcc904a0c4057403d7b |
| pb/profile/v1/profile.service.pb.go | ae2be80097ab2665e99322dcb987749aec0aec1d05d11aaa418ef2dc8c3e3aee |
| pb/profile/v1/profile.service_grpc.pb.go | 335dfc62148e7f622b0e8b7906a64d87ff19b1b20d36655e4cb2cfd62ee2cf8e |
| pb/role/v1/role.payload.pb.go | 45f80cbf62838966aa929d7cc6bf3cb64d96dafcc53c0988c17957e3585f7e10 |
| pb/role/v1/role.service.pb.go | 2fc8fe6166e37de5819ebaf2cede69df12b30c1e9c0c9d1099c7f53ce4168529 |
| pb/role/v1/role.service_grpc.pb.go | 4220a4c4a11eefe2ce6d0c7aedc747507e10f6655407c3a94120cb2cf3b802a9 |
| pb/session/v1/session.payload.pb.go | 024e0fcbb57f14dcfd1ba6effc3b85c109dbd6536e790e0c53ceb85b0a6f91db |
| pb/session/v1/session.service.pb.go | ca31dbe0612d4518494c612c7b4e0b407d21aa4c4e46e1225f2b8f5c7d67bbac |
| pb/session/v1/session.service_grpc.pb.go | bf6a685fea81e7e894b54961e3b833e975c7c8f7c30c083a44c6f657d1a708f1 |
| pb/user/v1/user.payload.pb.go | 026cf92f062be50c2189e03649c1999e98a2bd90b56b1457c9bb641a035b4fad |
| pb/user/v1/user.service.pb.go | 6ff4e4201f3b30c535d0b0eacff2f1e778a7d1652af21bac510fd354989931a3 |
| pb/user/v1/user.service_grpc.pb.go | 364a5404f257bfb146e0100d4942a841f24763850e3498260bd8a87cf44b5659 |

See [evidence](evidence.md) for commands, environment, actual timings, source manifest and publication blocker.
