//go:build postgres

package test_scenarios

import (
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/features/application"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"reflect"
	"testing"
	"time"
)

func ApplicationManagement() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered additive subset access-only complete revoke history provenance credentials and replay", Run: postgresApplicationManagement},
		{Name: "registered AND system status eligibility and no disclosure atomic errors", Run: postgresApplicationErrors},
		{Name: "required writes conditional deferred commit and independent failure audit rollback", Run: postgresApplicationFailures},
		{Name: "owned access constraints and composite identity", Run: postgresApplicationSchema},
		{Name: "independent user application method scopes and shared invalid command quotas", Run: postgresApplicationScopes},
		{Name: "revoked access provenance retained during role-only cleanup", Run: postgresApplicationRevokedCleanup},
	}
}
func applicationMethods() []string {
	return []string{userv1.UserService_ListUserApplicationAccess_FullMethodName, applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName, applicationv1.ApplicationService_RevokeApplicationAccess_FullMethodName}
}
func newApplicationHarness(t *testing.T) *roleHarness {
	t.Helper()
	h := newAssignmentHarness(t, applicationMethods()...)
	_, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.application.grant','Grant access'),(1,'iam.user.application.revoke','Revoke access'); INSERT INTO role_permissions(role_id,permission_id) SELECT 2,id FROM permissions WHERE code IN ('iam.user.application.grant','iam.user.application.revoke');`)
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func applicationDeps(h *roleHarness) application.Dependencies {
	p := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
	return application.Dependencies{Transactions: p, Snapshots: p, Bind: application.BindRepositories, Now: h.now, EventID: eventIDs()}
}
func applicationService(t *testing.T, d application.Dependencies) application.Service {
	t.Helper()
	s, e := application.NewService(d)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func applicationGrant(version uint64, roles ...uint64) *applicationv1.GrantApplicationAccessRequest {
	return &applicationv1.GrantApplicationAccessRequest{UserId: 2, ApplicationCode: " Sales ", RoleIds: roles, ExpectedUserVersion: managementVersion(version), Reason: " Controlled access review "}
}
func applicationRevoke(version uint64) *applicationv1.RevokeApplicationAccessRequest {
	return &applicationv1.RevokeApplicationAccessRequest{UserId: 2, ApplicationCode: "sales", ExpectedUserVersion: managementVersion(version), Reason: " Controlled access review "}
}

func accessCredentialState(t *testing.T, h *roleHarness) string {
	t.Helper()
	var s string
	e := h.f.DB.QueryRowContext(h.ctx, `SELECT jsonb_build_object('auth',(SELECT jsonb_agg(to_jsonb(a)-'version'-'updated_at' ORDER BY id) FROM auth a),'sessions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM sessions s),'refresh',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM refresh_tokens r),'other_assignments',(SELECT jsonb_agg(to_jsonb(ur) ORDER BY ur.id) FROM user_role ur JOIN role r ON r.id=ur.role_id WHERE ur.user_id<>2 OR r.application_id<>2),'other_access',(SELECT jsonb_agg(to_jsonb(ua) ORDER BY user_id,application_id) FROM user_application_access ua WHERE user_id<>2 OR application_id<>2))::text`).Scan(&s)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func postgresApplicationManagement(t *testing.T) {
	h := newApplicationHarness(t)
	credentials := accessCredentialState(t, h)
	var provenance string
	if e := h.f.DB.QueryRowContext(h.ctx, `SELECT to_jsonb(ur)::text FROM user_role ur WHERE user_id=2 AND role_id=3`).Scan(&provenance); e != nil {
		t.Fatal(e)
	}
	req := applicationGrant(1, 6, 5, 6)
	first, e := h.applications.GrantApplicationAccess(h.actor("grant"), req)
	if e != nil || !first.Changed || first.Version != 2 || !reflect.DeepEqual(first.ChangedRoleIds, []uint64{5, 6}) || first.GetAccessStatus() != "ACTIVE" {
		t.Fatal("grant", first, e)
	}
	var restored bool
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT granted_by=1 AND granted_at=$1 AND revoked_by IS NULL AND revoked_at IS NULL AND revoke_reason IS NULL FROM user_role WHERE user_id=2 AND role_id=5`, h.now()).Scan(&restored); e != nil || !restored {
		t.Fatal("D pair reactivation", e)
	}
	var after string
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT to_jsonb(ur)::text FROM user_role ur WHERE user_id=2 AND role_id=3`).Scan(&after); e != nil || provenance != after {
		t.Fatal("unselected active rewritten", e)
	}
	replay, e := h.applications.GrantApplicationAccess(h.actor("grant"), req)
	if e != nil || !proto.Equal(first, replay) {
		t.Fatal("response lost replay", e)
	}
	if _, e = h.f.DB.ExecContext(h.ctx, `UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled prior revoke' WHERE user_id=2 AND application_id=2`); e != nil {
		t.Fatal(e)
	}
	only, e := h.applications.GrantApplicationAccess(h.actor("access-only"), applicationGrant(2, 3))
	if e != nil || !only.Changed || only.Version != 3 || len(only.ChangedRoleIds) != 0 {
		t.Fatal("access only", only, e)
	}
	if _, e = h.f.DB.ExecContext(h.ctx, `INSERT INTO role(role_name,role_code,application_id,code) SELECT 'Role'||n,100+n,2,'sales.role_'||n FROM generate_series(1,25) n; INSERT INTO user_role(user_id,role_id) SELECT 2,id FROM role WHERE code LIKE 'sales.role_%'`); e != nil {
		t.Fatal(e)
	}
	removed, e := h.applications.RevokeApplicationAccess(h.actor("revoke"), applicationRevoke(3))
	if e != nil || !removed.Changed || removed.Version != 4 || len(removed.ChangedRoleIds) != 29 || removed.GetAccessStatus() != "REVOKED" {
		t.Fatal("complete revoke", removed, e)
	}
	if union := roleEffective(t, h); len(union["sales.order.read"]) != 0 || len(union["sales.order.update"]) != 0 {
		t.Fatal("original JWT did not lose contribution", union)
	}
	subset, e := h.applications.GrantApplicationAccess(h.actor("subset"), applicationGrant(4, 3))
	if e != nil || subset.Version != 5 || !reflect.DeepEqual(subset.ChangedRoleIds, []uint64{3}) {
		t.Fatal("subset", subset, e)
	}
	if union := roleEffective(t, h); !reflect.DeepEqual(union["sales.order.read"], []uint64{3}) {
		t.Fatal("revoked unselected role restored", union)
	}
	noopReq := applicationGrant(5, 3)
	noop, e := h.applications.GrantApplicationAccess(h.actor("noop"), noopReq)
	if e != nil || noop.Changed || noop.Version != 5 {
		t.Fatal("noop", e)
	}
	snap := managementBusinessState(t, h.managementHarness)
	noReplay, e := h.applications.GrantApplicationAccess(h.actor("noop"), noopReq)
	if e != nil || !proto.Equal(noop, noReplay) || snap != managementBusinessState(t, h.managementHarness) {
		t.Fatal("noop replay", e)
	}
	if _, e = h.f.DB.ExecContext(h.ctx, `DELETE FROM user_application_access WHERE user_id=2 AND application_id=2`); e != nil {
		t.Fatal(e)
	}
	clean, e := h.applications.RevokeApplicationAccess(h.actor("absent"), applicationRevoke(5))
	if e != nil || !clean.Changed || clean.Version != 6 || clean.AccessStatus != nil || !reflect.DeepEqual(clean.ChangedRoleIds, []uint64{3}) {
		t.Fatal("absent cleanup", clean, e)
	}
	noAccess, e := h.applications.RevokeApplicationAccess(h.actor("absent-noop"), applicationRevoke(6))
	if e != nil || noAccess.Changed || noAccess.AccessStatus != nil || noAccess.Version != 6 {
		t.Fatal("absent noop", e)
	}
	var rows, events, roleEvents, noops int
	var safe bool
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT (SELECT count(*) FROM user_application_access WHERE user_id=2 AND application_id=2),(SELECT count(*) FROM outbox_events WHERE event_type LIKE 'iam.user.application_access_%' AND published_at IS NULL),(SELECT count(*) FROM outbox_events WHERE event_type IN ('iam.user.roles_granted.v1','iam.user.roles_revoked.v1')),(SELECT count(*) FROM audit_logs WHERE action='NO_OP'),(SELECT bool_and(NOT(payload->'data' ? 'reason') AND payload->'data'->>'application_code'='sales') FROM outbox_events WHERE event_type LIKE 'iam.user.application_access_%')`).Scan(&rows, &events, &roleEvents, &noops, &safe)
	if e != nil || rows != 0 || events != 5 || roleEvents != 0 || noops != 2 || !safe {
		t.Fatal("compound evidence", rows, events, roleEvents, noops, e)
	}
	if credentials != accessCredentialState(t, h) {
		t.Fatal("credential or other app state changed")
	}
	// A retained grant must ignore fresh target eligibility/version, then expiry
	// executes a fresh command with its expected version and replaces the record.
	original, e := h.applications.GrantApplicationAccess(h.actor("grant"), req)
	if e != nil || !proto.Equal(first, original) {
		t.Fatal("historical replay", e)
	}
	conflict := proto.Clone(req).(*applicationv1.GrantApplicationAccessRequest)
	conflict.Reason = "Different review"
	_, e = h.applications.GrantApplicationAccess(h.actor("grant"), conflict)
	assertManagementCatalog(t, e, codes.Aborted, usererrors.ErrIdempotencyConflict.Key())
	if _, e = h.f.DB.ExecContext(h.ctx, `UPDATE idempotency_records SET created_at=$1,expires_at=$2 WHERE method=$3 AND key='grant'`, h.now().Add(-25*time.Hour), h.now().Add(-time.Hour), applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName); e != nil {
		t.Fatal(e)
	}
	_, e = h.applications.GrantApplicationAccess(h.actor("grant"), req)
	assertManagementCatalog(t, e, codes.Aborted, usererrors.ErrVersionConflict.Key())
	fresh, e := h.applications.GrantApplicationAccess(h.actor("grant"), applicationGrant(6, 3))
	if e != nil || fresh.Version != 7 {
		t.Fatal("expired execution", e)
	}
	var retention bool
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT expires_at=created_at+interval '24 hours' FROM idempotency_records WHERE method=$1 AND key='grant'`, applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName).Scan(&retention); e != nil || !retention {
		t.Fatal("retention", e)
	}
	assertAssignmentLockReuse(t, h)
}

func postgresApplicationErrors(t *testing.T) {
	cases := []struct {
		name, setup string
		code        codes.Code
		reason      string
	}{
		{"unknown", "", codes.NotFound, roleerrors.ErrRoleNotFound.Key()},
		{"disabled role", "UPDATE role SET status='DISABLED' WHERE id=6", codes.FailedPrecondition, roleerrors.ErrRoleDisabled.Key()},
		{"disabled app", "UPDATE applications SET status='DISABLED' WHERE id=2", codes.FailedPrecondition, applicationerrors.ErrApplicationInactive.Key()},
		{"cross app", "", codes.InvalidArgument, ""},
		{"system", "UPDATE applications SET is_system=true WHERE id=2", codes.FailedPrecondition, applicationerrors.ErrApplicationAccessSystemProtected.Key()},
		{"IAM", "", codes.FailedPrecondition, applicationerrors.ErrApplicationAccessSystemProtected.Key()},
		{"missing app permission", "UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.application.grant')", codes.PermissionDenied, ""},
		{"missing role permission", "UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.role.grant')", codes.PermissionDenied, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newApplicationHarness(t)
			if c.setup != "" {
				if _, e := h.f.DB.ExecContext(h.ctx, c.setup); e != nil {
					t.Fatal(e)
				}
			}
			before := managementBusinessState(t, h.managementHarness)
			req := applicationGrant(1, 5, 6)
			if c.name == "unknown" {
				req.RoleIds = []uint64{6, 999}
			}
			if c.name == "cross app" {
				req.RoleIds = []uint64{1, 6}
			}
			if c.name == "IAM" {
				req.ApplicationCode = "iam"
				req.RoleIds = []uint64{1}
			}
			_, e := h.applications.GrantApplicationAccess(h.actor("deny"), req)
			if c.reason != "" {
				assertManagementCatalog(t, e, c.code, c.reason)
			} else if status.Code(e) != c.code {
				t.Fatal(e)
			}
			if before != managementBusinessState(t, h.managementHarness) {
				t.Fatal("partial invalid outcome")
			}
		})
	}
	for _, state := range []string{"PENDING", "SUSPENDED", "DEACTIVATED"} {
		t.Run(state, func(t *testing.T) {
			h := newApplicationHarness(t)
			if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE auth SET status=$1 WHERE id=2`, state); e != nil {
				t.Fatal(e)
			}
			_, e := h.applications.GrantApplicationAccess(h.actor("inactive"), applicationGrant(1, 6))
			assertManagementCatalog(t, e, codes.FailedPrecondition, usererrors.ErrGrantTargetInactive.Key())
			if _, e = h.f.DB.ExecContext(h.ctx, `UPDATE applications SET status='DISABLED' WHERE id=2;UPDATE role SET status='DISABLED' WHERE application_id=2;DELETE FROM user_application_access WHERE user_id=2`); e != nil {
				t.Fatal(e)
			}
			out, e := h.applications.RevokeApplicationAccess(h.actor("cleanup"), applicationRevoke(1))
			if e != nil || len(out.ChangedRoleIds) != 2 || out.AccessStatus != nil {
				t.Fatal(out, e)
			}
		})
	}
	for _, permission := range []string{"iam.user.application.grant", "iam.user.role.grant"} {
		t.Run("replay loses "+permission, func(t *testing.T) {
			h := newApplicationHarness(t)
			req := applicationGrant(1, 6)
			if _, e := h.applications.GrantApplicationAccess(h.actor("saved"), req); e != nil {
				t.Fatal(e)
			}
			if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code=$1)`, permission); e != nil {
				t.Fatal(e)
			}
			before := managementBusinessState(t, h.managementHarness)
			if _, e := h.applications.GrantApplicationAccess(h.actor("saved"), req); status.Code(e) != codes.PermissionDenied {
				t.Fatal("stale replay", e)
			}
			if before != managementBusinessState(t, h.managementHarness) {
				t.Fatal("denied replay side effects")
			}
		})
	}
}

func postgresApplicationFailures(t *testing.T) {
	for _, cmd := range []string{"grant", "revoke"} {
		for _, stage := range []string{"access", "reactivate access", "zero access", "zero role", "role", "later role", "version", "conditional", "audit", "outbox", "result", "commit", "audit failure"} {
			t.Run(cmd+"/"+stage, func(t *testing.T) {
				h := newApplicationHarness(t)
				if cmd == "grant" && stage != "reactivate access" {
					if _, e := h.f.DB.ExecContext(h.ctx, `DELETE FROM user_application_access WHERE user_id=2 AND application_id=2`); e != nil {
						t.Fatal(e)
					}
				}
				if cmd == "grant" && stage == "reactivate access" {
					if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Prior review' WHERE user_id=2 AND application_id=2`); e != nil {
						t.Fatal(e)
					}
				}
				before := managementBusinessState(t, h.managementHarness)
				table, op, condition := "user_application_access", "UPDATE", "TRUE"
				if cmd == "grant" {
					op = "INSERT"
				}
				switch stage {
				case "reactivate access":
					op = "UPDATE"
				case "zero role":
					table, op = "user_role", "UPDATE"
				case "role", "later role":
					table = "user_role"
					op = "UPDATE"
					if cmd == "grant" {
						if stage == "later role" {
							op = "INSERT"
							condition = "NEW.role_id=6"
						} else {
							condition = "NEW.role_id=5"
						}
					} else if stage == "later role" {
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
				body := "IF " + condition + " THEN RAISE EXCEPTION 'controlled access failure' USING ERRCODE='23514'; END IF; RETURN NEW;"
				trigger := "CREATE TRIGGER access_fault AFTER " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION access_fault()"
				if stage == "zero access" || stage == "zero role" {
					body = "RETURN NULL;"
					trigger = "CREATE TRIGGER access_fault BEFORE " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION access_fault()"
				}
				if stage == "conditional" {
					body = "RETURN NULL;"
					trigger = "CREATE TRIGGER access_fault BEFORE UPDATE ON auth FOR EACH ROW EXECUTE FUNCTION access_fault()"
				}
				if stage == "commit" {
					trigger = "CREATE CONSTRAINT TRIGGER access_fault AFTER INSERT ON idempotency_records DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION access_fault()"
				}
				if _, e := h.f.DB.ExecContext(h.ctx, "CREATE FUNCTION access_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN "+body+" END $$; "+trigger); e != nil {
					t.Fatal(e)
				}
				var e error
				if cmd == "grant" {
					_, e = h.applications.GrantApplicationAccess(h.actor("fault"), applicationGrant(1, 5, 6))
				} else {
					_, e = h.applications.RevokeApplicationAccess(h.actor("fault"), applicationRevoke(1))
				}
				want := codes.Unavailable
				if stage == "conditional" || stage == "zero access" || stage == "zero role" {
					want = codes.Aborted
				}
				if status.Code(e) != want {
					t.Fatal("fault", e)
				}
				if before != managementBusinessState(t, h.managementHarness) {
					t.Fatal("partial outcome")
				}
				assertManagementFailureMetrics(t, h.managementHarness, stage)

				var attempts int
				if e = h.f.DB.QueryRowContext(h.ctx, `SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED'`).Scan(&attempts); e != nil || attempts != map[bool]int{true: 0, false: 1}[stage == "audit failure"] {
					t.Fatal("failure audit", attempts, e)
				}
				assertAssignmentLockReuse(t, h)
			})
		}
	}
}

func postgresApplicationSchema(t *testing.T) {
	h := newApplicationHarness(t)
	for _, sql := range []string{`INSERT INTO user_application_access(user_id,application_id) VALUES(2,2)`, `UPDATE user_application_access SET status='REVOKED' WHERE user_id=2`, `UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='x' WHERE user_id=2`, `UPDATE user_application_access SET granted_by=999 WHERE user_id=2`, `UPDATE user_application_access SET revoked_by=999 WHERE user_id=2`, `UPDATE user_application_access SET application_id=999 WHERE user_id=2`, `UPDATE user_application_access SET user_id=999 WHERE user_id=2`, `UPDATE user_application_access SET granted_at=NULL WHERE user_id=2`} {
		if _, e := h.f.DB.ExecContext(h.ctx, sql); e == nil {
			t.Fatal("owned constraint missing")
		}
	}
	var index bool
	e := h.f.DB.QueryRowContext(h.ctx, `SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname=$1 AND tablename='user_application_access' AND indexdef LIKE '%(application_id, status, user_id)%')`, h.f.Name).Scan(&index)
	if e != nil || !index {
		t.Fatal("owned access index", e)
	}
}

func postgresApplicationScopes(t *testing.T) {
	h := newApplicationHarness(t)
	if _, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO applications(code,name) VALUES('clm','CLM');INSERT INTO role(role_name,role_code,application_id,code) VALUES('CLM reader',9,3,'clm.reader');INSERT INTO auth(email,password,status) SELECT 'scope@example.test',password,'ACTIVE' FROM auth WHERE id=1;INSERT INTO profile(user_id,name) VALUES(3,'Scope target')`); e != nil {
		t.Fatal(e)
	}
	if _, e := h.applications.GrantApplicationAccess(h.actor("shared"), applicationGrant(1, 6)); e != nil {
		t.Fatal(e)
	}
	otherApp := applicationGrant(2, 9)
	otherApp.ApplicationCode = "clm"
	if _, e := h.applications.GrantApplicationAccess(h.actor("shared"), otherApp); e != nil {
		t.Fatal("app scope", e)
	}
	otherUser := applicationGrant(1, 6)
	otherUser.UserId = 3
	if _, e := h.applications.GrantApplicationAccess(h.actor("shared"), otherUser); e != nil {
		t.Fatal("user scope", e)
	}
	if _, e := h.applications.RevokeApplicationAccess(h.actor("shared"), applicationRevoke(3)); e != nil {
		t.Fatal("method scope", e)
	}
	var scopes int
	if e := h.f.DB.QueryRowContext(h.ctx, `SELECT count(*) FROM idempotency_records WHERE key='shared' AND route_scope IN ('user/2/application/2','user/2/application/3','user/3/application/2')`).Scan(&scopes); e != nil || scopes != 4 {
		t.Fatal("independent records", scopes, e)
	}
	// actor() advances the controlled clock; freeze admission time within each
	// burst to prove D uses exactly the existing per-user buckets.
	ctx := h.actor("")
	for i := 0; i < 5; i++ {
		var e error
		if i%2 == 0 {
			_, e = h.applications.GrantApplicationAccess(ctx, &applicationv1.GrantApplicationAccessRequest{})
		} else {
			_, e = h.applications.RevokeApplicationAccess(ctx, &applicationv1.RevokeApplicationAccessRequest{})
		}
		if status.Code(e) != codes.InvalidArgument {
			t.Fatal("invalid quota", e)
		}
	}
	if _, e := h.users.GrantUserRoles(ctx, &userv1.GrantUserRolesRequest{}); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("C/D shared mutation", e)
	}
	for i := 0; i < 20; i++ {
		if _, e := h.users.ListUserApplicationAccess(ctx, &userv1.ListUserApplicationAccessRequest{UserId: 2}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := h.users.ListUserRoles(ctx, &userv1.ListUserRolesRequest{UserId: 2}); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("C/D shared read", e)
	}
}

func postgresApplicationRevokedCleanup(t *testing.T) {
	h := newApplicationHarness(t)
	if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE user_application_access SET status='REVOKED',revoked_at='2026-01-01',revoked_by=2,revoke_reason='Previous revocation' WHERE user_id=2 AND application_id=2`); e != nil {
		t.Fatal(e)
	}
	var before, after string
	if e := h.f.DB.QueryRowContext(h.ctx, `SELECT to_jsonb(a)::text FROM user_application_access a WHERE user_id=2 AND application_id=2`).Scan(&before); e != nil {
		t.Fatal(e)
	}
	res, e := h.applications.RevokeApplicationAccess(h.actor("cleanup"), applicationRevoke(1))
	if e != nil || !res.Changed || res.Version != 2 || res.GetAccessStatus() != "REVOKED" || !reflect.DeepEqual(res.ChangedRoleIds, []uint64{3, 4}) {
		t.Fatal(res, e)
	}
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT to_jsonb(a)::text FROM user_application_access a WHERE user_id=2 AND application_id=2`).Scan(&after); e != nil || before != after {
		t.Fatal("prior provenance rewritten", e)
	}
}
