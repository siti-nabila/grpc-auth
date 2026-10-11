# Contract inventory and publication

Inspected api-contracts AGENTS, clean feat/user-err baseline 668cbd2, source
proto/catalog, generated descriptors, existing tests, and grpc-auth consumers
(register.service.go, foundation.go, handler/user.handler.go).

| Contract | Before | Additive result |
| --- | --- | --- |
| UserService | Register/Login/Me/ListUsers/TesRPC | Existing signatures preserved; GetUser/CreateUser/UpdateUser/ChangeUserStatus added |
| User | id1/name2/email3/password4/dob5/phone_no6 | All names/types/numbers retained; address7/status8/version9/created_at10/must_change_password11/roles12/application_access13 |
| CreateUserRequest | name1/email2/password3 string | optional status4, address5/phone6 string |
| CreateUserResponse/GetUserResponse | User user1 | Same wrapper type and number |
| GetUserRequest | uint64 id1 | Unchanged |
| UpdateUserRequest | Absent | user_id1, optional expected_user_version2/name3/address4/phone5, FieldMask6 |
| UpdateUserResponse | Absent | User user1, bool changed2 |
| ChangeUserStatusRequest | Absent | user_id1, optional status2, reason3, optional expected_user_version4 |
| ChangeUserStatusResponse | Absent | user_id1/status2/version3/changed4 |

Expected versions use uint64; backend requires present positive INTEGER-compatible
target IDs and positive int64-compatible versions. Legacy User.password is retained
but never populated by management mapping. Test fixtures verify safe serialization;
backend response safety remains a separate required task.

Errors reused: auth.not_found=NF/NotFound/404, auth.already_exists=EX/AlreadyExists/409,
auth validation catalog, user.version_conflict=VC, resource_busy=RB,
idempotency_conflict=IC (Aborted/409). New user errors:
invalid_status_transition=IST/FailedPrecondition/409,
self_inactivation_forbidden=SIF/PermissionDenied/403,
last_admin_required=LAR/FailedPrecondition/409. Locales en/id and id-ID/fr fallback
are tested with redacted wrapped error identity/decoder metadata; no legacy remap.

## Actual verification

- make proto: PASS. Second run: all 13 generated Go file hashes identical.
- GOCACHE=/tmp/auth-management-go-cache go test ./...: PASS in api-contracts.
- GOCACHE=/tmp/auth-management-go-cache go vet ./...: PASS in api-contracts.
- git diff --check: PASS; generated changes confined to three user/v1 files.
- Descriptor tests compare each old RPC signature independently, allowing additions;
  old field descriptors, wrapper identity and additive metadata legacy wire hop PASS.
- No runtime auth/REST/DB code or credentials were added to contract repository.
- Approved normal commit/push feat/user-err: 7297589746962a3d994cf850a623502562017db0.
- Published module: v0.0.0-20261011113651-729758974696.
- Checksum: h1:MZ1/0dEXvGZIHfWPlGYFOPfDg55lOnxtEYR/HiPr92Q=.
- GOWORK=off go mod download -json <module>@7297589: PASS, exact origin commit.
- grpc-auth dependency updated with GOWORK=off GOPROXY=off GOSUMDB=off go get
  to this already verified downloaded module. No replace/module-cache edits.
- Full backend initial suites found old inventory assertion15 versus descriptor19;
  the boundary regression now covers all19 anonymous and all13 new-method forbidden
  caller paths. This is not proof that management handlers are implemented.

Backend lifecycle/management completion and DEP-01 remain open.
