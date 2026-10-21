//go:build postgres

package test_scenarios

import (
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/writer"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"reflect"
	"testing"
	"time"
)

func AssignmentManagement() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered historical list atomic batch replay noops regrant and original JWT union", Run: postgresAssignments},
		{Name: "registered semantic errors entire invalid batches and current authority before replay", Run: postgresAssignmentErrors},
		{Name: "scoped method target expiry24h and response lost commit replay", Run: postgresAssignmentRetention},
		{Name: "real pair uniqueness constraints nullable legacy and conditional version rollback", Run: postgresAssignmentStorage},
		{Name: "all required writes deferred commit and failure audit errors preserve atomicity", Run: postgresAssignmentFailures},
		{Name: "C and baseline shared read mutation quotas count invalid commands", Run: postgresAssignmentQuota},
	}
}

func assignmentMethods() []string {
	return []string{userv1.UserService_ListUserRoles_FullMethodName, userv1.UserService_GrantUserRoles_FullMethodName, userv1.UserService_RevokeUserRoles_FullMethodName}
}
func newAssignmentHarness(t *testing.T, extra ...string) *roleHarness {
	t.Helper()
	h := newRoleHarness(t, append(assignmentMethods(), extra...)...)
	_, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.role.grant','Grant'),(1,'iam.user.role.revoke','Revoke');
 INSERT INTO role_permissions(role_id,permission_id) SELECT 2,id FROM permissions WHERE code IN ('iam.user.role.grant','iam.user.role.revoke');
 INSERT INTO role(role_name,role_code,application_id,code) VALUES('New editor',6,2,'sales.new'),('IAM reader',7,1,'iam.assignment_reader'),('Never assigned',8,2,'sales.unassigned');
 INSERT INTO role_permissions(role_id,permission_id) SELECT 7,id FROM permissions WHERE code='iam.user.read';
 DELETE FROM user_application_access WHERE user_id=2 AND application_id=1;
 UPDATE user_role SET granted_by=NULL,granted_at=NULL WHERE user_id=2;`)
	if e != nil {
		t.Fatal("owned assignment fixture", e)
	}
	return h
}
func assignmentGrant(user, version uint64, roles ...uint64) *userv1.GrantUserRolesRequest {
	return &userv1.GrantUserRolesRequest{UserId: user, RoleIds: roles, ExpectedUserVersion: managementVersion(version), Reason: " Controlled access review "}
}
func assignmentRevoke(user, version uint64, roles ...uint64) *userv1.RevokeUserRolesRequest {
	return &userv1.RevokeUserRolesRequest{UserId: user, RoleIds: roles, ExpectedUserVersion: managementVersion(version), Reason: " Controlled access review "}
}
func assignmentCredentialState(t *testing.T, h *roleHarness) string {
	t.Helper()
	var s string
	e := h.f.DB.QueryRowContext(h.ctx, `SELECT jsonb_build_object(
 'auth',(SELECT jsonb_agg(to_jsonb(a)-'version'-'updated_at' ORDER BY id) FROM auth a),
 'profiles',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM profile p),
 'roles',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM role r),
 'mapping',(SELECT jsonb_agg(to_jsonb(r) ORDER BY role_id,permission_id) FROM role_permissions r),
 'access',(SELECT jsonb_agg(to_jsonb(a) ORDER BY user_id,application_id) FROM user_application_access a),
 'sessions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM sessions s),
 'refresh',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM refresh_tokens r))::text`).Scan(&s)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func postgresAssignments(t *testing.T) {
	h := newAssignmentHarness(t)
	credentials := assignmentCredentialState(t, h)
	assertUnion := func(read, update []uint64) {
		t.Helper()
		got := roleEffective(t, h)
		if !reflect.DeepEqual(got["sales.order.read"], read) || !reflect.DeepEqual(got["sales.order.update"], update) {
			t.Fatal("current original-JWT union", got)
		}
		if credentials != assignmentCredentialState(t, h) {
			t.Fatal("credential/app/catalog state changed")
		}
	}
	assertUnion([]uint64{3, 4}, []uint64{3})
	list, e := h.users.ListUserRoles(h.actor(""), &userv1.ListUserRolesRequest{UserId: 2})
	if e != nil || list.Version != 1 || list.Meta.Total != 4 || len(list.Items) != 4 {
		t.Fatal("historical list", list, e)
	}
	for _, r := range list.Items {
		if r.GrantedBy != nil || r.GrantedAt != nil {
			t.Fatal("fabricated legacy provenance", r)
		}
	}
	var historicalID, editorID int64
	if e = h.f.DB.QueryRowContext(h.ctx, "SELECT id FROM user_role WHERE user_id=2 AND role_id=5").Scan(&historicalID); e != nil {
		t.Fatal(e)
	}
	if e = h.f.DB.QueryRowContext(h.ctx, "SELECT id FROM user_role WHERE user_id=2 AND role_id=3").Scan(&editorID); e != nil {
		t.Fatal(e)
	}
	grant := assignmentGrant(2, 1, 6, 3, 5, 6)
	first, e := h.users.GrantUserRoles(h.actor("grant"), grant)
	if e != nil || !first.Changed || first.Version != 2 || !reflect.DeepEqual(first.ChangedRoleIds, []uint64{5, 6}) {
		t.Fatal("batch grant", first, e)
	}
	var same, cleared bool
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT id=$1,granted_by=1 AND granted_at=$2 AND revoked_by IS NULL AND revoked_at IS NULL AND revoke_reason IS NULL FROM user_role WHERE user_id=2 AND role_id=5`, historicalID, h.now()).Scan(&same, &cleared)
	if e != nil || !same || !cleared {
		t.Fatal("reactivation identity/provenance", same, cleared, e)
	}
	// The saved result is the only response source after commit; input order/duplicates
	// and advanced target version cannot create another event or transition.
	retry := proto.Clone(grant).(*userv1.GrantUserRolesRequest)
	retry.RoleIds = []uint64{5, 6, 3}
	replay, e := h.users.GrantUserRoles(h.actor("grant"), retry)
	if e != nil || !proto.Equal(first, replay) {
		t.Fatal("lost-response replay", e)
	}
	assertUnion([]uint64{3, 4}, []uint64{3})
	revoked, e := h.users.RevokeUserRoles(h.actor("revoke"), assignmentRevoke(2, 2, 7, 5, 3))
	if e != nil || !revoked.Changed || revoked.Version != 3 || !reflect.DeepEqual(revoked.ChangedRoleIds, []uint64{3, 5}) {
		t.Fatal("mixed revoke", revoked, e)
	}
	assertUnion([]uint64{4}, nil)
	r, e := h.users.GrantUserRoles(h.actor("iam"), assignmentGrant(2, 3, 7))
	if e != nil || !r.Changed || r.Version != 4 {
		t.Fatal("IAM without app access", r, e)
	}
	var absent int
	if e = h.f.DB.QueryRowContext(h.ctx, "SELECT count(*) FROM user_application_access WHERE user_id=2 AND application_id=1").Scan(&absent); e != nil || absent != 0 {
		t.Fatal("IAM access created", e)
	}
	noopReq := assignmentRevoke(2, 4, 8, 3)
	noop, e := h.users.RevokeUserRoles(h.actor("noop"), noopReq)
	if e != nil || noop.Changed || noop.Version != 4 || len(noop.ChangedRoleIds) != 0 {
		t.Fatal("registered absent/revoked noop", noop, e)
	}
	var reason string
	var revokedBy uint64
	var revokedAt time.Time
	e = h.f.DB.QueryRowContext(h.ctx, "SELECT revoked_by,revoked_at,revoke_reason FROM user_role WHERE user_id=2 AND role_id=3").Scan(&revokedBy, &revokedAt, &reason)
	if e != nil || revokedBy != 1 || reason != "Controlled access review" {
		t.Fatal("revocation provenance", e)
	}
	noReplayBefore := managementBusinessState(t, h.managementHarness)
	noReplay, e := h.users.RevokeUserRoles(h.actor("noop"), noopReq)
	if e != nil || !proto.Equal(noop, noReplay) || noReplayBefore != managementBusinessState(t, h.managementHarness) {
		t.Fatal("noop replay effects", e)
	}
	r, e = h.users.GrantUserRoles(h.actor("regrant"), assignmentGrant(2, 4, 3))
	if e != nil || r.Version != 5 {
		t.Fatal("regrant", e)
	}
	var id int64
	e = h.f.DB.QueryRowContext(h.ctx, "SELECT id FROM user_role WHERE user_id=2 AND role_id=3 AND revoked_at IS NULL AND revoke_reason IS NULL AND revoked_by IS NULL").Scan(&id)
	if e != nil || id != editorID {
		t.Fatal("pair replaced", id, e)
	}
	assertUnion([]uint64{3, 4}, []uint64{3})
	if r, e := h.users.RevokeUserRoles(h.actor("overlap"), assignmentRevoke(2, 5, 4)); e != nil || r.Version != 6 {
		t.Fatal("overlap revoke", e)
	}
	assertUnion([]uint64{3}, []uint64{3})
	if r, e := h.users.GrantUserRoles(h.actor("grant-noop"), assignmentGrant(2, 6, 3)); e != nil || r.Changed || r.Version != 6 {
		t.Fatal("grant noop", e)
	}
	var audits, events, records, noops int
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT (SELECT count(*) FROM audit_logs WHERE action IN ('USER_ROLES_GRANTED','USER_ROLES_REVOKED')),(SELECT count(*) FROM outbox_events WHERE event_type IN ('iam.user.roles_granted.v1','iam.user.roles_revoked.v1') AND published_at IS NULL),(SELECT count(*) FROM idempotency_records WHERE method IN ('/user.v1.UserService/GrantUserRoles','/user.v1.UserService/RevokeUserRoles')),(SELECT count(*) FROM audit_logs WHERE action='NO_OP')`).Scan(&audits, &events, &records, &noops)
	if e != nil || audits != 5 || events != 5 || records != 7 || noops != 2 {
		t.Fatal("atomic persistence counts", audits, events, records, noops, e)
	}
	var safe bool
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT bool_and(NOT (payload->'data' ? 'reason') AND NOT (payload->'data' ? 'password')) FROM outbox_events WHERE event_type IN ('iam.user.roles_granted.v1','iam.user.roles_revoked.v1')`).Scan(&safe)
	if e != nil || !safe {
		t.Fatal("event safety", e)
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("assignment resource leak")
	}
}

func postgresAssignmentErrors(t *testing.T) {
	cases := []struct {
		name, setup string
		code        codes.Code
		reason      string
	}{
		{"unknown role", "", codes.NotFound, roleerrors.ErrRoleNotFound.Key()},
		{"disabled role", "UPDATE role SET status='DISABLED' WHERE id=6", codes.FailedPrecondition, roleerrors.ErrRoleDisabled.Key()},
		{"disabled app", "UPDATE applications SET status='DISABLED' WHERE id=2", codes.FailedPrecondition, applicationerrors.ErrApplicationInactive.Key()},
		{"missing access", "DELETE FROM user_application_access WHERE user_id=2 AND application_id=2", codes.FailedPrecondition, applicationerrors.ErrApplicationAccessRequired.Key()},
		{"revoked access", "UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled fixture revocation' WHERE user_id=2 AND application_id=2", codes.FailedPrecondition, applicationerrors.ErrApplicationAccessRequired.Key()},
		{"inactive target", "UPDATE auth SET status='SUSPENDED' WHERE id=2", codes.FailedPrecondition, usererrors.ErrGrantTargetInactive.Key()},
		{"self admin", "", codes.PermissionDenied, usererrors.ErrSelfAdminRoleRevokeForbidden.Key()},
		{"inactive actor", "INSERT INTO user_role(user_id,role_id) VALUES(2,2);UPDATE auth SET status='SUSPENDED' WHERE id=1", codes.Unauthenticated, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newAssignmentHarness(t)
			if c.setup != "" {
				if _, e := h.f.DB.ExecContext(h.ctx, c.setup); e != nil {
					t.Fatal(e)
				}
			}
			before := managementBusinessState(t, h.managementHarness)
			var e error
			switch c.name {
			case "self admin":
				_, e = h.users.RevokeUserRoles(h.actor("deny"), assignmentRevoke(1, 1, 2))
			case "unknown role":
				_, e = h.users.GrantUserRoles(h.actor("deny"), assignmentGrant(2, 1, 6, 999))
			default:
				_, e = h.users.GrantUserRoles(h.actor("deny"), assignmentGrant(2, 1, 5, 6))
			}
			if c.reason != "" {
				assertManagementCatalog(t, e, c.code, c.reason)
			} else if status.Code(e) != c.code {
				t.Fatal(e)
			}
			if managementBusinessState(t, h.managementHarness) != before {
				t.Fatal("invalid batch partial write")
			}
		})
	}
	// Revoke accepts every account status and disabled role/application without access.
	for _, state := range []string{"PENDING", "SUSPENDED", "DEACTIVATED"} {
		t.Run("cleanup "+state, func(t *testing.T) {
			h := newAssignmentHarness(t)
			if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE auth SET status=$1 WHERE id=2;`, state); e != nil {
				t.Fatal(e)
			}
			if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE role SET status='DISABLED' WHERE id=3;UPDATE applications SET status='DISABLED' WHERE id=2;DELETE FROM user_application_access WHERE user_id=2`); e != nil {
				t.Fatal(e)
			}
			r, e := h.users.RevokeUserRoles(h.actor("cleanup"), assignmentRevoke(2, 1, 3, 8))
			if e != nil || r.Version != 2 || !reflect.DeepEqual(r.ChangedRoleIds, []uint64{3}) {
				t.Fatal(r, e)
			}
			page, e := h.users.ListUserRoles(h.actor(""), &userv1.ListUserRolesRequest{UserId: 2})
			if e != nil || page.Version != 2 || page.Meta.Total != 4 {
				t.Fatal(page, e)
			}
		})
	}
	h := newAssignmentHarness(t)
	req := assignmentGrant(2, 1, 6)
	if _, e := h.users.GrantUserRoles(h.actor("retained"), req); e != nil {
		t.Fatal(e)
	}
	if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.role.grant')`); e != nil {
		t.Fatal(e)
	}
	before := managementBusinessState(t, h.managementHarness)
	if _, e := h.users.GrantUserRoles(h.actor("retained"), req); status.Code(e) != codes.PermissionDenied {
		t.Fatal("replay stale authority", e)
	}
	if before != managementBusinessState(t, h.managementHarness) {
		t.Fatal("denied replay effects")
	}
}

func postgresAssignmentRetention(t *testing.T) {
	h := newAssignmentHarness(t)
	req := assignmentGrant(2, 1, 7)
	first, e := h.users.GrantUserRoles(h.actor("shared"), req)
	if e != nil {
		t.Fatal(e)
	}
	again, e := h.users.GrantUserRoles(h.actor("shared"), req)
	if e != nil || !proto.Equal(first, again) {
		t.Fatal(e)
	}
	different := proto.Clone(req).(*userv1.GrantUserRolesRequest)
	different.Reason = "Different reason"
	_, e = h.users.GrantUserRoles(h.actor("shared"), different)
	assertManagementCatalog(t, e, codes.Aborted, usererrors.ErrIdempotencyConflict.Key())
	if _, e = h.users.GrantUserRoles(h.actor("shared"), assignmentGrant(1, 1, 7)); e != nil {
		t.Fatal("different target scope", e)
	}
	if _, e = h.users.RevokeUserRoles(h.actor("shared"), assignmentRevoke(2, 2, 7)); e != nil {
		t.Fatal("different method scope", e)
	}
	if _, e = h.f.DB.ExecContext(h.ctx, `UPDATE idempotency_records SET created_at=$1,expires_at=$2 WHERE method=$3 AND route_scope='user/2' AND key='shared'`, h.now().Add(-25*time.Hour), h.now().Add(-time.Hour), userv1.UserService_GrantUserRoles_FullMethodName); e != nil {
		t.Fatal(e)
	}
	_, e = h.users.GrantUserRoles(h.actor("shared"), req)
	assertManagementCatalog(t, e, codes.Aborted, usererrors.ErrVersionConflict.Key())
	req.ExpectedUserVersion = managementVersion(3)
	fresh, e := h.users.GrantUserRoles(h.actor("shared"), req)
	if e != nil || fresh.Version != 4 {
		t.Fatal("expired new command", e)
	}
	var valid bool
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT created_at=$1 AND expires_at=$2 FROM idempotency_records WHERE method=$3 AND route_scope='user/2' AND key='shared'`, h.now(), h.now().Add(24*time.Hour), userv1.UserService_GrantUserRoles_FullMethodName).Scan(&valid)
	if e != nil || !valid {
		t.Fatal("24h retention", e)
	}
}

func postgresAssignmentStorage(t *testing.T) {
	h := newAssignmentHarness(t)
	before := managementBusinessState(t, h.managementHarness)
	provider := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
	tx, e := provider.Begin(h.ctx)
	if e != nil {
		t.Fatal(e)
	}
	w := writer.NewUserRoleAssignmentWriter(tx)
	ok, e := w.AdvanceVersion(h.ctx, 2, 999, h.now())
	if e != nil || ok {
		t.Fatal("conditional version", ok, e)
	}
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	tx, e = provider.Begin(h.ctx)
	if e != nil {
		t.Fatal(e)
	}
	w = writer.NewUserRoleAssignmentWriter(tx)
	r := domain.UserRoleAssignmentItem{RoleID: 6}
	if ok, e = w.Activate(h.ctx, 2, r, 1, h.now()); e != nil || !ok {
		t.Fatal(e)
	}
	if _, e = w.Activate(h.ctx, 2, r, 1, h.now()); e == nil {
		t.Fatal("pair uniqueness absent")
	}
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	for _, statement := range []string{`INSERT INTO user_role(user_id,role_id,status) VALUES(2,6,'INVALID')`, `INSERT INTO user_role(user_id,role_id,status) VALUES(2,6,'REVOKED')`, `INSERT INTO user_role(user_id,role_id,status,revoked_at,revoke_reason) VALUES(2,6,'REVOKED',CURRENT_TIMESTAMP,'x')`, `INSERT INTO user_role(user_id,role_id,granted_by) VALUES(2,6,999)`, `INSERT INTO user_role(user_id,role_id,revoked_by) VALUES(2,6,999)`} {
		if _, e = h.f.DB.ExecContext(h.ctx, statement); e == nil {
			t.Fatal("missing constraint")
		}
	}
	if before != managementBusinessState(t, h.managementHarness) {
		t.Fatal("raw batch rollback failed")
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("storage pool leak")
	}
}

func postgresAssignmentFailures(t *testing.T) {
	for _, command := range []string{"grant", "revoke"} {
		stages := []string{"assignment", "later assignment", "version", "conditional", "audit", "outbox", "result", "commit", "audit failure"}
		if command == "grant" {
			stages = append(stages, "reactivate")
		}
		for _, stage := range stages {
			t.Run(command+"/"+stage, func(t *testing.T) {
				h := newAssignmentHarness(t)
				before := managementBusinessState(t, h.managementHarness)
				table, op, condition := "user_role", "INSERT", "TRUE"
				if command == "revoke" {
					op = "UPDATE"
				}
				switch stage {
				case "reactivate":
					op = "UPDATE"
					condition = "NEW.role_id=5"
				case "later assignment":
					condition = "NEW.role_id=6"
					if command == "revoke" {
						condition = "NEW.role_id=4"
					}
				case "version", "conditional":
					table, op = "auth", "UPDATE"
				case "audit", "audit failure":
					table, op = "audit_logs", "INSERT"
					if stage == "audit" {
						condition = "NEW.action<>'SECURITY_ATTEMPT_FAILED'"
					}
				case "outbox":
					table, op = "outbox_events", "INSERT"
				case "result", "commit":
					table, op = "idempotency_records", "INSERT"
				}
				body := "IF " + condition + " THEN RAISE EXCEPTION 'controlled fixture failure' USING ERRCODE='23514'; END IF; RETURN NEW;"
				trigger := "CREATE TRIGGER assignment_fault AFTER " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION assignment_fault()"
				if stage == "conditional" {
					body = "RETURN NULL;"
					trigger = "CREATE TRIGGER assignment_fault BEFORE UPDATE ON auth FOR EACH ROW EXECUTE FUNCTION assignment_fault()"
				}
				if stage == "commit" {
					trigger = "CREATE CONSTRAINT TRIGGER assignment_fault AFTER INSERT ON idempotency_records DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION assignment_fault()"
				}
				if _, e := h.f.DB.ExecContext(h.ctx, "CREATE FUNCTION assignment_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN "+body+" END $$; "+trigger); e != nil {
					t.Fatal(e)
				}
				var e error
				if command == "grant" {
					_, e = h.users.GrantUserRoles(h.actor("fault"), assignmentGrant(2, 1, 5, 6))
				} else {
					_, e = h.users.RevokeUserRoles(h.actor("fault"), assignmentRevoke(2, 1, 3, 4))
				}
				want := codes.Unavailable
				if stage == "conditional" {
					want = codes.Aborted
				}
				if status.Code(e) != want {
					t.Fatal("fault mapping", e)
				}
				if before != managementBusinessState(t, h.managementHarness) {
					t.Fatal("partial business writes", stage)
				}
				assertManagementFailureMetrics(t, h.managementHarness, stage)

				var failures int
				if e = h.f.DB.QueryRowContext(h.ctx, "SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED'").Scan(&failures); e != nil || failures != map[bool]int{true: 0, false: 1}[stage == "audit failure"] {
					t.Fatal("independent failure audit", failures, e)
				}
				if h.f.DB.Stats().InUse != 0 {
					t.Fatal("fault pool leak")
				}
			})
		}
	}
}

func postgresAssignmentQuota(t *testing.T) {
	h := newAssignmentHarness(t)
	ctx := shared.RPCContext(h.ctx, h.admin.Token)
	before := managementBusinessState(t, h.managementHarness)
	for i := 0; i < 5; i++ {
		var e error
		if i%2 == 0 {
			_, e = h.users.GrantUserRoles(ctx, &userv1.GrantUserRolesRequest{})
		} else {
			_, e = h.users.RevokeUserRoles(ctx, &userv1.RevokeUserRolesRequest{})
		}
		if status.Code(e) != codes.InvalidArgument {
			t.Fatal(e)
		}
	}
	if _, e := h.users.UpdateUser(ctx, &userv1.UpdateUserRequest{}); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("shared mutation quota", e)
	}
	for i := 0; i < 20; i++ {
		if _, e := h.users.ListUserRoles(ctx, &userv1.ListUserRolesRequest{UserId: 2}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := h.users.GetUser(ctx, &userv1.GetUserRequest{Id: 2}); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("shared read quota", e)
	}
	if before != managementBusinessState(t, h.managementHarness) {
		t.Fatal("quota business effects")
	}
}
