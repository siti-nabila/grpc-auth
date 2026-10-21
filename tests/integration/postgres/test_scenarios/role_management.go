//go:build postgres

package test_scenarios

import (
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"testing"
	"time"
)

func RoleManagement() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered role commands persist atomic outcomes exact replay noops scoped expiry and safe errors", Run: postgresRoleManagement},
		{Name: "every role write and deferred commit failure rolls back before independent audit", Run: postgresRoleFailures},
		{Name: "new catalog and mutation RPCs share existing read and admin quota", Run: postgresRoleQuota},
		{Name: "registered current permission and restricted session checks precede target disclosure", Run: postgresRoleAuthorization},
	}
}

type roleHarness struct {
	*managementHarness
	roles        rolev1.RoleServiceClient
	permissions  permissionv1.PermissionServiceClient
	applications applicationv1.ApplicationServiceClient
}

func roleMethods() []string {
	return []string{rolev1.RoleService_ListRoles_FullMethodName, rolev1.RoleService_GetRole_FullMethodName, rolev1.RoleService_CreateRole_FullMethodName, rolev1.RoleService_UpdateRole_FullMethodName, rolev1.RoleService_ChangeRoleStatus_FullMethodName, rolev1.RoleService_ReplaceRolePermissions_FullMethodName, permissionv1.PermissionService_ListPermissions_FullMethodName, applicationv1.ApplicationService_ListApplications_FullMethodName}
}
func newRoleHarness(t *testing.T, extra ...string) *roleHarness {
	t.Helper()
	m := newManagementHarness(t, append(roleMethods(), extra...)...)
	_, e := m.f.DB.ExecContext(m.ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.role.read','Read roles'),(1,'iam.role.create','Create roles'),(1,'iam.role.update','Update roles'),(1,'iam.role.permission.update','Replace permissions'),(1,'iam.permission.read','Read catalog');
 INSERT INTO role_permissions(role_id,permission_id) SELECT 2,id FROM permissions WHERE code LIKE 'iam.role.%' OR code='iam.permission.read';
 INSERT INTO applications(code,name) VALUES('sales','Sales');
 INSERT INTO role(role_name,role_code,application_id,code,version) VALUES('Editor',3,2,'sales.editor',7),('Overlap',4,2,'sales.overlap',1),('Historical',5,2,'sales.historical',1);
 SELECT setval('role_numeric_code_seq',100,false);
 INSERT INTO permissions(application_id,code,description) VALUES(2,'sales.order.read','Read orders'),(2,'sales.order.update','Update orders');
 INSERT INTO role_permissions(role_id,permission_id) SELECT 3,id FROM permissions WHERE code LIKE 'sales.%';
 INSERT INTO role_permissions(role_id,permission_id) SELECT 4,id FROM permissions WHERE code='sales.order.read';
 INSERT INTO user_role(user_id,role_id) VALUES(2,3),(2,4);
 INSERT INTO user_role(user_id,role_id,status,revoked_at,revoke_reason) VALUES(2,5,'REVOKED',CURRENT_TIMESTAMP,'Historical revocation');
 INSERT INTO user_application_access(user_id,application_id) VALUES(2,2);`)
	if e != nil {
		t.Fatal("owned role fixture", e)
	}
	return &roleHarness{m, rolev1.NewRoleServiceClient(m.conn), permissionv1.NewPermissionServiceClient(m.conn), applicationv1.NewApplicationServiceClient(m.conn)}
}
func roleCredentialState(t *testing.T, h *roleHarness) string {
	t.Helper()
	var s string
	e := h.f.DB.QueryRowContext(h.ctx, `SELECT jsonb_build_object(
 'auth',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM auth a),
 'profile',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM profile p),
 'assignments',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM user_role r),
 'access',(SELECT jsonb_agg(to_jsonb(a) ORDER BY user_id,application_id) FROM user_application_access a),
 'sessions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM sessions s),
 'refresh',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM refresh_tokens r))::text`).Scan(&s)
	if e != nil {
		t.Fatal("credential snapshot", e)
	}
	return s
}
func roleBusinessState(t *testing.T, h *roleHarness) string {
	t.Helper()
	var s string
	e := h.f.DB.QueryRowContext(h.ctx, `SELECT jsonb_build_object(
 'roles',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM role r),
 'mappings',(SELECT jsonb_agg(to_jsonb(p) ORDER BY role_id,permission_id) FROM role_permissions p),
 'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a WHERE action<>'SECURITY_ATTEMPT_FAILED'),
 'events',(SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM outbox_events o),
 'results',(SELECT jsonb_agg(to_jsonb(i) ORDER BY actor_id,method,route_scope,key) FROM idempotency_records i))::text`).Scan(&s)
	if e != nil {
		t.Fatal("role business snapshot", e)
	}
	return s + roleCredentialState(t, h)
}
func roleUpdate(id, version uint64, name string) *rolev1.UpdateRoleRequest {
	return &rolev1.UpdateRoleRequest{RoleId: id, ExpectedRoleVersion: managementVersion(version), Name: &name, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}}}
}
func roleStatus(version uint64, s string) *rolev1.ChangeRoleStatusRequest {
	return &rolev1.ChangeRoleStatusRequest{RoleId: 3, ExpectedRoleVersion: managementVersion(version), Status: &s, Reason: "Controlled review"}
}
func roleReplace(version uint64, codes ...string) *rolev1.ReplaceRolePermissionsRequest {
	return &rolev1.ReplaceRolePermissionsRequest{RoleId: 3, ExpectedRoleVersion: managementVersion(version), Permissions: &rolev1.PermissionSet{Codes: codes}, Reason: "Controlled review"}
}
func postgresRoleManagement(t *testing.T) {
	h := newRoleHarness(t)
	credentials := roleCredentialState(t, h)
	create := &rolev1.CreateRoleRequest{ApplicationCode: "sales", Code: "sales.created", Name: "  Created  ", PermissionCodes: []string{"sales.order.update", "sales.order.read", "sales.order.read"}}
	first, e := h.roles.CreateRole(h.actor("create"), create)
	if e != nil || first.Role.Code != "sales.created" || first.Role.RoleCode != 100 || first.Role.Version != 1 || first.Role.IsSystem || first.Role.Status != "ACTIVE" || len(first.Permissions) != 2 {
		t.Fatal("creation", e)
	}
	replay, e := h.roles.CreateRole(h.actor("create"), create)
	if e != nil || !proto.Equal(first, replay) {
		t.Fatal("lost response replay", e)
	}
	_, e = h.roles.CreateRole(h.actor("duplicate"), create)
	assertManagementCatalog(t, e, codes.AlreadyExists, roleerrors.ErrRoleCodeAlreadyExists.Key())
	changed := proto.Clone(create).(*rolev1.CreateRoleRequest)
	changed.Name = "Different"
	_, e = h.roles.CreateRole(h.actor("create"), changed)
	assertManagementCatalog(t, e, codes.Aborted, roleerrors.ErrIdempotencyConflict.Key())
	update := roleUpdate(3, 7, "Renamed")
	updated, e := h.roles.UpdateRole(h.actor("update"), update)
	if e != nil || !updated.Changed || updated.Role.Version != 8 || updated.Role.AffectedUserCount != 1 {
		t.Fatal("metadata update", e)
	}
	clear := ""
	cleared, e := h.roles.UpdateRole(h.actor(""), &rolev1.UpdateRoleRequest{RoleId: 3, ExpectedRoleVersion: managementVersion(8), Description: &clear, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}}})
	if e != nil || cleared.Changed || cleared.Role.Version != 8 || cleared.Role.Description != nil {
		t.Fatal("unkeyed NULL no-op", e)
	}
	disabled, e := h.roles.ChangeRoleStatus(h.actor("status"), roleStatus(8, "DISABLED"))
	if e != nil || !disabled.Changed || disabled.Version != 9 {
		t.Fatal("disable", e)
	}
	replayed, e := h.roles.UpdateRole(h.actor("update"), update)
	if e != nil || !proto.Equal(updated, replayed) {
		t.Fatal("historical safe response", e)
	}
	_, e = h.roles.UpdateRole(h.actor("stale"), update)
	assertManagementCatalog(t, e, codes.Aborted, roleerrors.ErrVersionConflict.Key())
	noOp, e := h.roles.ChangeRoleStatus(h.actor("noop"), roleStatus(9, "DISABLED"))
	if e != nil || noOp.Changed || noOp.Version != 9 {
		t.Fatal("status no-op", e)
	}
	replaced, e := h.roles.ReplaceRolePermissions(h.actor("replace"), roleReplace(9, "sales.order.read"))
	if e != nil || !replaced.Changed || replaced.Role.Version != 10 || len(replaced.Permissions) != 1 {
		t.Fatal("replace", e)
	}
	equivalent := roleReplace(9, "sales.order.read", "sales.order.read")
	r, e := h.roles.ReplaceRolePermissions(h.actor("replace"), equivalent)
	if e != nil || !proto.Equal(replaced, r) {
		t.Fatal("canonical set replay", e)
	}
	empty, e := h.roles.ReplaceRolePermissions(h.actor("empty"), roleReplace(10))
	if e != nil || !empty.Changed || empty.Role.Version != 11 || len(empty.Permissions) != 0 {
		t.Fatal("explicit empty clear", e)
	}
	if _, e = h.roles.ReplaceRolePermissions(h.actor("unknown"), roleReplace(11, "sales.order.missing")); status.Code(e) != codes.InvalidArgument {
		t.Fatal("unknown permission", e)
	}
	// Same key has independent method and target scope. Expired keys execute afresh.
	if _, e = h.roles.UpdateRole(h.actor("update"), roleUpdate(4, 1, "Scoped")); e != nil {
		t.Fatal("target scope", e)
	}
	if _, e = h.f.DB.ExecContext(h.ctx, `UPDATE idempotency_records SET created_at=$1,expires_at=$2 WHERE key='update' AND route_scope='role/3'`, h.now().Add(-25*time.Hour), h.now().Add(-time.Hour)); e != nil {
		t.Fatal(e)
	}
	if _, e = h.roles.UpdateRole(h.actor("update"), roleUpdate(3, 11, "After expiry")); e != nil {
		t.Fatal("expired replacement", e)
	}
	detail, e := h.roles.GetRole(h.actor(""), &rolev1.GetRoleRequest{RoleId: 3})
	if e != nil || detail.Role.Version != 12 || detail.Role.AffectedUserCount != 1 || len(detail.Permissions) != 0 {
		t.Fatal("safe current detail", e)
	}
	var audits, events, records int
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE target_type='role' AND action<>'SECURITY_ATTEMPT_FAILED'),(SELECT count(*) FROM outbox_events WHERE aggregate_type='role'),(SELECT count(*) FROM idempotency_records WHERE route_scope LIKE 'role/%' OR route_scope LIKE 'role-code/%')`).Scan(&audits, &events, &records)
	if e != nil || audits != 9 || events != 7 || records != 7 {
		t.Fatal("atomic outcome counts", audits, events, records, e)
	}
	if credentials != roleCredentialState(t, h) {
		t.Fatal("role mutation changed credentials or assignment history")
	}
	// Catalog visibility remains separate from editability and evaluation.
	if _, e = h.f.DB.ExecContext(h.ctx, "UPDATE role SET is_system=true,status='DISABLED' WHERE id=4; UPDATE applications SET status='DISABLED' WHERE id=2"); e != nil {
		t.Fatal(e)
	}
	protected, e := h.roles.GetRole(h.actor(""), &rolev1.GetRoleRequest{RoleId: 4})
	if e != nil || !protected.Role.IsSystem || protected.Role.Status != "DISABLED" || protected.Role.AffectedUserCount != 1 || len(protected.Permissions) != 1 {
		t.Fatal("disabled system role in disabled application must remain readable", e)
	}
	_, e = h.roles.GetRole(h.actor(""), &rolev1.GetRoleRequest{RoleId: 999})
	assertManagementCatalog(t, e, codes.NotFound, roleerrors.ErrRoleNotFound.Key())
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("role pool leak")
	}
}
func postgresRoleFailures(t *testing.T) {
	for _, command := range []string{"create", "update", "status", "replace"} {
		stages := []string{"role", "audit", "outbox", "result", "commit"}
		if command == "create" || command == "replace" {
			stages = append(stages, "mapping insert")
			if command == "replace" {
				stages = append(stages, "mapping soft-delete", "mapping restore")
			}
		}
		if command != "create" {
			stages = append(stages, "conditional")
		}
		for _, stage := range stages {
			t.Run(command+"/"+stage, func(t *testing.T) {
				h := newRoleHarness(t)
				if command == "replace" {
					prepareRoleMappingDiff(t, h)
				}
				before := roleBusinessState(t, h)
				table, op, condition := "role", "UPDATE", "TRUE"
				if command == "create" {
					op = "INSERT"
				}
				switch stage {
				case "mapping insert":
					table, op = "role_permissions", "INSERT"
				case "mapping soft-delete":
					table, op, condition = "role_permissions", "UPDATE", "NEW.deleted_at IS NOT NULL AND OLD.deleted_at IS NULL"
				case "mapping restore":
					table, op, condition = "role_permissions", "UPDATE", "NEW.deleted_at IS NULL AND OLD.deleted_at IS NOT NULL"
				case "audit":
					table, op, condition = "audit_logs", "INSERT", "NEW.action<>'SECURITY_ATTEMPT_FAILED'"
				case "outbox":
					table, op = "outbox_events", "INSERT"
				case "result", "commit":
					table, op = "idempotency_records", "INSERT"
				}
				body := "IF " + condition + " THEN RAISE EXCEPTION 'controlled role failure' USING ERRCODE='23514'; END IF; RETURN NEW;"
				trigger := "CREATE TRIGGER role_fault AFTER " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION role_fault()"
				if stage == "conditional" {
					body = "RETURN NULL;"
					trigger = "CREATE TRIGGER role_fault BEFORE UPDATE ON role FOR EACH ROW EXECUTE FUNCTION role_fault()"
				}
				if stage == "commit" {
					trigger = "CREATE CONSTRAINT TRIGGER role_fault AFTER INSERT ON idempotency_records DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION role_fault()"
				}
				if _, e := h.f.DB.ExecContext(h.ctx, "CREATE FUNCTION role_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN "+body+" END $$; "+trigger); e != nil {
					t.Fatal("fault setup", e)
				}
				var e error
				method := rolev1.RoleService_CreateRole_FullMethodName
				switch command {
				case "create":
					_, e = h.roles.CreateRole(h.actor("fault"), &rolev1.CreateRoleRequest{ApplicationCode: "sales", Code: "sales.fault", Name: "Fault", PermissionCodes: []string{"sales.order.read"}})
				case "update":
					method = rolev1.RoleService_UpdateRole_FullMethodName
					_, e = h.roles.UpdateRole(h.actor("fault"), roleUpdate(3, 7, "Fault"))
				case "status":
					method = rolev1.RoleService_ChangeRoleStatus_FullMethodName
					_, e = h.roles.ChangeRoleStatus(h.actor("fault"), roleStatus(7, "DISABLED"))
				case "replace":
					method = rolev1.RoleService_ReplaceRolePermissions_FullMethodName
					_, e = h.roles.ReplaceRolePermissions(h.actor("fault"), roleReplace(7, "sales.order.read", "sales.order.create"))
				}
				want := codes.Unavailable
				if stage == "conditional" {
					want = codes.Aborted
				}
				if status.Code(e) != want {
					t.Fatal("fault classification", e)
				}
				if before != roleBusinessState(t, h) {
					t.Fatal("partial role outcome after failed command")
				}
				assertManagementFailureMetrics(t, h.managementHarness, stage)

				var failures int
				e = h.f.DB.QueryRowContext(h.ctx, `SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND target_id=$1`, method).Scan(&failures)
				if e != nil || failures != 1 {
					t.Fatal("independent failure audit", failures, e)
				}
				if h.f.DB.Stats().InUse != 0 {
					t.Fatal("fault pool leak")
				}
			})
		}
	}
}
func postgresRoleQuota(t *testing.T) {
	h := newRoleHarness(t)
	ctx := shared.RPCContext(h.ctx, h.admin.Token)
	before := roleBusinessState(t, h)
	for i := 0; i < 5; i++ {
		var e error
		switch i {
		case 0:
			_, e = h.users.CreateUser(ctx, &userv1.CreateUserRequest{})
		case 1:
			_, e = h.roles.CreateRole(ctx, &rolev1.CreateRoleRequest{})
		case 2:
			_, e = h.roles.UpdateRole(ctx, &rolev1.UpdateRoleRequest{})
		case 3:
			_, e = h.roles.ChangeRoleStatus(ctx, &rolev1.ChangeRoleStatusRequest{})
		case 4:
			_, e = h.roles.ReplaceRolePermissions(ctx, &rolev1.ReplaceRolePermissionsRequest{})
		}
		if status.Code(e) != codes.InvalidArgument {
			t.Fatal("invalid admission", i, e)
		}
	}
	_, e := h.roles.CreateRole(ctx, &rolev1.CreateRoleRequest{})
	d, ok := grpcerror.Decode(e)
	if !ok || d.Code != codes.ResourceExhausted || d.RetryInfo == nil || d.RetryInfo.RetryDelay.AsDuration() != 2*time.Second {
		t.Fatal("shared mutation quota", e)
	}
	if before != roleBusinessState(t, h) {
		t.Fatal("rejected command persisted business state")
	}
	for i := 0; i < 20; i++ {
		switch i % 5 {
		case 0:
			_, e = h.users.GetUser(ctx, &userv1.GetUserRequest{Id: 2})
		case 1:
			_, e = h.roles.ListRoles(ctx, &rolev1.ListRolesRequest{})
		case 2:
			_, e = h.roles.GetRole(ctx, &rolev1.GetRoleRequest{RoleId: 3})
		case 3:
			_, e = h.permissions.ListPermissions(ctx, &permissionv1.ListPermissionsRequest{})
		case 4:
			_, e = h.applications.ListApplications(ctx, &applicationv1.ListApplicationsRequest{})
		}
		if e != nil {
			t.Fatal("shared reads", i, e)
		}
	}
	_, e = sessionv1.NewSessionServiceClient(h.conn).ListMySessions(ctx, &sessionv1.ListMySessionsRequest{})
	if status.Code(e) != codes.ResourceExhausted {
		t.Fatal("session read did not share bucket", e)
	}
}
func postgresRoleAuthorization(t *testing.T) {
	h := newRoleHarness(t)
	for _, method := range roleMethods() {
		var request, response proto.Message
		switch method {
		case rolev1.RoleService_ListRoles_FullMethodName:
			request, response = &rolev1.ListRolesRequest{}, &rolev1.ListRolesResponse{}
		case rolev1.RoleService_GetRole_FullMethodName:
			request, response = &rolev1.GetRoleRequest{RoleId: 999}, &rolev1.GetRoleResponse{}
		case rolev1.RoleService_CreateRole_FullMethodName:
			request, response = &rolev1.CreateRoleRequest{ApplicationCode: "sales", Code: "sales.denied", Name: "Denied"}, &rolev1.CreateRoleResponse{}
		case rolev1.RoleService_UpdateRole_FullMethodName:
			request, response = roleUpdate(999, 1, "Denied"), &rolev1.UpdateRoleResponse{}
		case rolev1.RoleService_ChangeRoleStatus_FullMethodName:
			r := roleStatus(1, "DISABLED")
			r.RoleId = 999
			request, response = r, &rolev1.ChangeRoleStatusResponse{}
		case rolev1.RoleService_ReplaceRolePermissions_FullMethodName:
			r := roleReplace(1)
			r.RoleId = 999
			request, response = r, &rolev1.ReplaceRolePermissionsResponse{}
		case permissionv1.PermissionService_ListPermissions_FullMethodName:
			request, response = &permissionv1.ListPermissionsRequest{}, &permissionv1.ListPermissionsResponse{}
		default:
			request, response = &applicationv1.ListApplicationsRequest{}, &applicationv1.ListApplicationsResponse{}
		}
		h.clock.Add(int64(2 * time.Second))
		ctx := shared.RPCContext(h.ctx, h.target.Token)
		ctx = metadata.AppendToOutgoingContext(ctx, "idempotency-key", "denied")
		if e := h.conn.Invoke(ctx, method, request, response); status.Code(e) != codes.PermissionDenied {
			t.Fatal("exact permission or target disclosure", method, e)
		}
	}
	// Mutation guards use localized role semantics and separate safe failure audit.
	_, e := h.roles.UpdateRole(h.actor("protected"), roleUpdate(1, 1, "Forbidden"))
	d, ok := grpcerror.Decode(e)
	if !ok || d.Code != codes.FailedPrecondition || d.Reason != roleerrors.ErrSystemRoleProtected.Key() || d.Message != roleerrors.ErrSystemRoleProtected.Message(locale.Indonesian) {
		t.Fatal("localized protected error", e)
	}
	if _, e = h.f.DB.ExecContext(h.ctx, "UPDATE auth SET must_change_password=true WHERE id=2"); e != nil {
		t.Fatal(e)
	}
	if _, e = h.roles.GetRole(shared.RPCContext(h.ctx, h.target.Token), &rolev1.GetRoleRequest{RoleId: 999}); status.Code(e) != codes.PermissionDenied {
		t.Fatal("restricted role disclosure", e)
	}
	// Old token remains accepted for its allowed restricted route.
	if _, e = h.users.Me(shared.RPCContext(h.ctx, h.target.Token), &emptypb.Empty{}); e != nil {
		t.Fatal("restricted Me compatibility", e)
	}
}
