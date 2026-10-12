//go:build postgres

package test_scenarios

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func UserManagement() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered management safe creation replay profile search status and credential lifecycle", Run: postgresUserManagement},
		{Name: "registered management every required write and deferred commit failure is atomic with independent audit", Run: postgresManagementFailures},
		{Name: "registered management quota counts invalid commands across shared group without effects", Run: postgresManagementQuota},
		{Name: "management target scoped keys expired replacement and revoked access remain distinct", Run: postgresManagementRetention},
	}
}

func postgresManagementRetention(t *testing.T) {
	h := newManagementHarness(t)
	name := "Retained target"
	req := &userv1.UpdateUserRequest{UserId: 2, ExpectedUserVersion: managementVersion(1), Name: &name, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}}}
	if _, e := h.users.UpdateUser(h.actor("shared"), req); e != nil {
		t.Fatal(e)
	}
	other := &userv1.UpdateUserRequest{UserId: 1, ExpectedUserVersion: managementVersion(1), Name: &name, FieldMask: req.FieldMask}
	if _, e := h.users.UpdateUser(h.actor("shared"), other); e != nil {
		t.Fatal("independent target scope", e)
	}
	var scopes int
	if e := h.f.DB.QueryRowContext(h.ctx, "SELECT count(*) FROM idempotency_records WHERE key='shared'").Scan(&scopes); e != nil || scopes != 2 {
		t.Fatal("scope collision", scopes, e)
	}
	if _, e := h.f.DB.ExecContext(h.ctx, `UPDATE idempotency_records SET created_at=$1,expires_at=$2 WHERE key='shared' AND route_scope='user/2'`, h.now().Add(-25*time.Hour), h.now().Add(-time.Hour)); e != nil {
		t.Fatal(e)
	}
	if _, e := h.users.UpdateUser(h.actor("shared"), req); status.Code(e) != codes.Aborted {
		t.Fatal("expired response bypassed expected version", e)
	}
	req.ExpectedUserVersion = managementVersion(2)
	r, e := h.users.UpdateUser(h.actor("shared"), req)
	if e != nil || r.Changed || r.User.Version != 2 {
		t.Fatal("expired scope not replaced by current no-op", e)
	}
	var fresh bool
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT created_at=$1 AND expires_at=$2 FROM idempotency_records WHERE key='shared' AND route_scope='user/2'`, h.now(), h.now().Add(24*time.Hour)).Scan(&fresh); e != nil || !fresh {
		t.Fatal("24h replacement", e)
	}
	for _, table := range []string{"user_role", "user_application_access"} {
		if _, e = h.f.DB.ExecContext(h.ctx, "UPDATE "+table+" SET status='REVOKED',revoked_at=$1,revoke_reason='Prior revocation' WHERE user_id=2", h.now()); e != nil {
			t.Fatal(e)
		}
	}
	for i, s := range []string{"DEACTIVATED", "ACTIVE"} {
		if _, e = h.users.ChangeUserStatus(h.actor("retention-"+s), &userv1.ChangeUserStatusRequest{UserId: 2, ExpectedUserVersion: managementVersion(uint64(2 + i)), Status: managementStatus(s), Reason: "Account review"}); e != nil {
			t.Fatal(e)
		}
	}
	detail, e := h.users.GetUser(h.actor(""), &userv1.GetUserRequest{Id: 2})
	if e != nil || len(detail.User.Roles) != 0 || len(detail.User.ApplicationAccess) != 0 {
		t.Fatal("reactivation regranted revoked access", detail, e)
	}
	var revoked int
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT (SELECT count(*) FROM user_role WHERE user_id=2 AND status='REVOKED')+(SELECT count(*) FROM user_application_access WHERE user_id=2 AND status='REVOKED')`).Scan(&revoked); e != nil || revoked != 2 {
		t.Fatal("historical revocation changed", e)
	}
}

type managementHarness struct {
	ctx           context.Context
	f             *fixtures.Schema
	conn          *grpc.ClientConn
	users         userv1.UserServiceClient
	auths         authv1.AuthServiceClient
	clock         atomic.Int64
	admin, target *userv1.UserTokenResponse
}

func managementMethods() []string {
	return []string{userv1.UserService_GetUser_FullMethodName, userv1.UserService_CreateUser_FullMethodName, userv1.UserService_UpdateUser_FullMethodName, userv1.UserService_ChangeUserStatus_FullMethodName}
}

func newManagementHarness(t *testing.T, extra ...string) *managementHarness {
	t.Helper()
	ctx, f := loginFixture(t, false)
	_, e := f.DB.ExecContext(ctx, `
INSERT INTO permissions(application_id,code,description) VALUES
(1,'iam.user.read','fixture'),(1,'iam.user.create','fixture'),(1,'iam.user.update','fixture'),(1,'iam.user.status.update','fixture');
INSERT INTO role(role_name,role_code,application_id,code) VALUES('Administrator',2,1,'iam.platform_admin');
INSERT INTO role_permissions SELECT 2,id FROM permissions;
INSERT INTO user_role(user_id,role_id) VALUES(1,2);
INSERT INTO auth(email,password,status) SELECT 'target@example.test',password,'ACTIVE' FROM auth WHERE id=1;
INSERT INTO profile(user_id,name,address,phone) VALUES(2,'Target user','Initial address','123');
INSERT INTO user_role(user_id,role_id) VALUES(2,1);
INSERT INTO user_application_access(user_id,application_id) VALUES(2,1);
CREATE FUNCTION management_search_fixture() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 INSERT INTO user_profile_search VALUES(NEW.id,to_tsvector('simple',COALESCE(NEW.name,'')||' '||COALESCE(NEW.address,'')),COALESCE(NEW.name,'')||' '||COALESCE(NEW.address,''))
 ON CONFLICT(profile_id) DO UPDATE SET fts_keyword=EXCLUDED.fts_keyword,fts_lexeme_text=EXCLUDED.fts_lexeme_text;
 RETURN NEW; END $$;
CREATE TRIGGER management_search_fixture AFTER INSERT OR UPDATE ON profile FOR EACH ROW EXECUTE FUNCTION management_search_fixture();
UPDATE profile SET name=name;`)
	if e != nil {
		t.Fatal("owned management fixture setup", e)
	}
	h := &managementHarness{ctx: ctx, f: f}
	h.clock.Store(shared.ServiceTime.UnixNano())
	methods := append(append(shared.LifecycleMethods(), managementMethods()...), extra...)
	conn, _ := shared.FoundationServer(t, f.DB, h.now, shared.LifecycleEntropy(), methods...)
	h.conn = conn
	h.users, h.auths = userv1.NewUserServiceClient(conn), authv1.NewAuthServiceClient(conn)
	h.admin = h.login(t, "fixture@example.test", "secret12")
	h.target = h.login(t, "target@example.test", "secret12")
	return h
}

func (h *managementHarness) now() time.Time { return time.Unix(0, h.clock.Load()).UTC() }
func (h *managementHarness) actor(key string) context.Context {
	h.clock.Add(int64(2 * time.Second))
	ctx := shared.RPCContext(h.ctx, h.admin.Token)
	if key != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "idempotency-key", key)
	}
	return ctx
}
func (h *managementHarness) login(t *testing.T, email, password string) *userv1.UserTokenResponse {
	t.Helper()
	h.clock.Add(int64(12 * time.Second))
	r, e := h.users.Login(shared.RPCContext(h.ctx, ""), &userv1.AuthRequest{Email: email, Password: password})
	if e != nil {
		t.Fatal("controlled Login", e)
	}
	return r
}
func managementVersion(v uint64) *uint64 { return &v }
func managementStatus(s string) *string  { return &s }

func postgresUserManagement(t *testing.T) {
	h := newManagementHarness(t)
	req := &userv1.CreateUserRequest{Name: "Created user", Email: " CREATED@EXAMPLE.TEST ", Password: "synthetic-password", Status: managementStatus("ACTIVE"), Address: "Created address", Phone: "987"}
	created, e := h.users.CreateUser(h.actor("create"), req)
	if e != nil || created.GetUser().GetId() != 3 || created.User.Email != "created@example.test" || created.User.Version != 1 || !created.User.MustChangePassword || len(created.User.Roles) != 1 || created.User.Roles[0].Code != "iam.member" {
		t.Fatal("safe creation", created, e)
	}
	assertManagementSafe(t, created.User)
	replayed, e := h.users.CreateUser(h.actor("create"), req)
	if e != nil || !proto.Equal(created, replayed) {
		t.Fatal("exact creation replay", e)
	}
	conflict := proto.Clone(req).(*userv1.CreateUserRequest)
	conflict.Password = "different-password"
	_, e = h.users.CreateUser(h.actor("create"), conflict)
	assertManagementCatalog(t, e, codes.Aborted, usererrors.ErrIdempotencyConflict.Key())
	_, e = h.users.CreateUser(h.actor("duplicate"), req)
	if status.Code(e) != codes.AlreadyExists {
		t.Fatal("duplicate normalized account", e)
	}
	var accounts, sessions, audits, events, records int
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT (SELECT count(*) FROM auth WHERE email='created@example.test'),(SELECT count(*) FROM sessions WHERE user_id=3),(SELECT count(*) FROM audit_logs WHERE action='USER_CREATED' AND target_id='3'),(SELECT count(*) FROM outbox_events WHERE event_type='iam.user.created.v1' AND aggregate_id='3' AND published_at IS NULL),(SELECT count(*) FROM idempotency_records WHERE route_scope='account/created@example.test')`).Scan(&accounts, &sessions, &audits, &events, &records)
	if e != nil || accounts != 1 || sessions != 0 || audits != 1 || events != 1 || records != 1 {
		t.Fatal("atomic creation or lost-response replay", accounts, sessions, audits, events, records, e)
	}
	initial := h.login(t, "created@example.test", req.Password)
	actor := shared.RPCContext(h.ctx, initial.Token)
	if !initial.MustChangePassword {
		t.Fatal("created account unrestricted")
	}
	if _, e = h.users.Me(actor, &emptypb.Empty{}); e != nil {
		t.Fatal("restricted Me", e)
	}
	if _, e = h.users.GetUser(actor, &userv1.GetUserRequest{Id: 3}); status.Code(e) != codes.PermissionDenied {
		t.Fatal("restricted management", e)
	}
	if _, e = h.auths.RefreshSession(shared.RPCContext(h.ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: initial.RefreshToken}); status.Code(e) != codes.PermissionDenied {
		t.Fatal("restricted refresh", e)
	}
	if r, e := h.auths.ChangePassword(actor, &authv1.ChangePasswordRequest{CurrentPassword: req.Password, NewPassword: "changed-password"}); e != nil || !r.LoginRequired {
		t.Fatal("restricted ChangePassword", e)
	}
	fresh := h.login(t, "created@example.test", "changed-password")
	if fresh.MustChangePassword {
		t.Fatal("fresh Login still restricted")
	}
	name := "UniqueZebra"
	update := &userv1.UpdateUserRequest{UserId: 3, ExpectedUserVersion: managementVersion(2), Name: &name, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}}}
	updated, e := h.users.UpdateUser(h.actor("profile"), update)
	if e != nil || !updated.Changed || updated.User.Version != 3 || updated.User.Address != "Created address" || updated.User.PhoneNo != "987" {
		t.Fatal("presence update", updated, e)
	}
	assertManagementSafe(t, updated.User)
	clear := ""
	cleared, e := h.users.UpdateUser(h.actor(""), &userv1.UpdateUserRequest{UserId: 3, ExpectedUserVersion: managementVersion(3), Address: &clear, Phone: &clear, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"address", "phone"}}})
	if e != nil || cleared.User.Version != 4 || cleared.User.Address != "" || cleared.User.PhoneNo != "" {
		t.Fatal("clear selected empty", cleared, e)
	}
	retry, e := h.users.UpdateUser(h.actor("profile"), update)
	if e != nil || !proto.Equal(updated, retry) {
		t.Fatal("profile replay read mutable detail", e)
	}
	_, e = h.users.UpdateUser(h.actor("stale"), update)
	assertManagementCatalog(t, e, codes.Aborted, usererrors.ErrVersionConflict.Key())
	update.ExpectedUserVersion = managementVersion(4)
	noop, e := h.users.UpdateUser(h.actor("noop"), update)
	if e != nil || noop.Changed || noop.User.Version != 4 {
		t.Fatal("valid no-op", e)
	}
	page, e := h.users.ListUsers(h.actor(""), &userv1.ListUsersRequest{Query: &paginatorv1.PageQuery{Limit: 10, Search: &paginatorv1.Search{Keyword: name}}})
	if e != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Id != 3 {
		t.Fatal("committed profile search visibility", page, e)
	}
	if _, e = h.users.Me(shared.RPCContext(h.ctx, fresh.Token), &emptypb.Empty{}); e != nil {
		t.Fatal("profile changed credentials", e)
	}
	var generation int
	if e = h.f.DB.QueryRowContext(h.ctx, "SELECT auth_version FROM auth WHERE id=3").Scan(&generation); e != nil || generation != 2 {
		t.Fatal("profile incremented generation", e)
	}
	// A latest revoked refresh is valid Logout proof until status advances its generation.
	if _, e = h.auths.Logout(shared.RPCContext(h.ctx, fresh.Token), &authv1.LogoutRequest{}); e != nil {
		t.Fatal(e)
	}
	active := h.login(t, "created@example.test", "changed-password")
	change := &userv1.ChangeUserStatusRequest{UserId: 3, ExpectedUserVersion: managementVersion(4), Status: managementStatus("SUSPENDED"), Reason: "Security review"}
	statusResult, e := h.users.ChangeUserStatus(h.actor("suspend"), change)
	if e != nil || !statusResult.Changed || statusResult.Version != 5 {
		t.Fatal("suspend", statusResult, e)
	}
	for _, r := range []*userv1.UserTokenResponse{fresh, active} {
		if _, e = h.users.Me(shared.RPCContext(h.ctx, r.Token), &emptypb.Empty{}); status.Code(e) != codes.Unauthenticated {
			t.Fatal("old access accepted", e)
		}
		if _, e = h.auths.Logout(shared.RPCContext(h.ctx, ""), &authv1.LogoutRequest{RefreshToken: &r.RefreshToken}); status.Code(e) != codes.Unauthenticated {
			t.Fatal("old terminal generation accepted", e)
		}
	}
	if _, e = h.auths.RefreshSession(shared.RPCContext(h.ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: active.RefreshToken}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("suspended refresh", e)
	}
	if r, e := h.users.GetUser(h.actor(""), &userv1.GetUserRequest{Id: 3}); e != nil || r.User.Status != "SUSPENDED" || len(r.User.Roles) != 1 {
		t.Fatal("inactive detail/assignment preservation", e)
	}
	var pending, duplicateEvents int
	e = h.f.DB.QueryRowContext(h.ctx, `SELECT (SELECT count(*) FROM outbox_events WHERE aggregate_id='3' AND aggregate_version=5 AND event_type='iam.user.status_changed.v1' AND payload->'data'->>'sessions_revoked'='true' AND published_at IS NULL),(SELECT count(*) FROM outbox_events WHERE aggregate_id='3' AND aggregate_version=5 AND event_type IN ('iam.user.sessions_revoked.v1','iam.session.revoked.v1'))`).Scan(&pending, &duplicateEvents)
	if e != nil || pending != 1 || duplicateEvents != 0 {
		t.Fatal("duplicate status events", pending, duplicateEvents, e)
	}
	if r, e := h.users.ChangeUserStatus(h.actor("suspend"), change); e != nil || !proto.Equal(statusResult, r) {
		t.Fatal("status replay", e)
	}
	change.ExpectedUserVersion = managementVersion(5)
	change.Status = managementStatus("ACTIVE")
	if r, e := h.users.ChangeUserStatus(h.actor("activate"), change); e != nil || r.Version != 6 {
		t.Fatal("reactivation", e)
	}
	newLogin := h.login(t, "created@example.test", "changed-password")
	if e = h.f.DB.QueryRowContext(h.ctx, "SELECT auth_version FROM sessions WHERE id=$1", newLogin.SessionId).Scan(&generation); e != nil || generation != 3 {
		t.Fatal("fresh generation", e)
	}
	if _, e = h.users.Me(shared.RPCContext(h.ctx, active.Token), &emptypb.Empty{}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("reactivation restored credential", e)
	}
	// Pending creation cannot login until activated; activation keeps restriction.
	req.Email = "pending@example.test"
	req.Status = managementStatus("PENDING")
	pendingUser, e := h.users.CreateUser(h.actor("pending"), req)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.users.Login(shared.RPCContext(h.ctx, ""), &userv1.AuthRequest{Email: req.Email, Password: req.Password}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("pending Login", e)
	}
	if _, e = h.users.ChangeUserStatus(h.actor("pending-activate"), &userv1.ChangeUserStatusRequest{UserId: pendingUser.User.Id, ExpectedUserVersion: managementVersion(1), Status: managementStatus("ACTIVE"), Reason: "Registration approved"}); e != nil {
		t.Fatal(e)
	}
	if r := h.login(t, req.Email, req.Password); !r.MustChangePassword {
		t.Fatal("activation removed restriction")
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("management pool leak")
	}
}

func assertManagementCatalog(t *testing.T, e error, code codes.Code, reason string) {
	t.Helper()
	d, ok := grpcerror.Decode(e)
	if !ok || d.Code != code || d.Reason != reason {
		t.Fatal("catalog identity", e)
	}
}
func assertManagementSafe(t *testing.T, u *userv1.User) {
	t.Helper()
	b, e := protojson.Marshal(u)
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{`"password":`, `"authVersion":`, `"refreshToken":`, `"token":`} {
		if strings.Contains(string(b), s) {
			t.Fatal("unsafe response")
		}
	}
}

// Snapshot all affected rows, excluding sequences and independent failure audits.
func managementBusinessState(t *testing.T, h *managementHarness) string {
	t.Helper()
	var value string
	e := h.f.DB.QueryRowContext(h.ctx, `SELECT jsonb_build_object(
'auth',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM auth a),
'profile',(SELECT jsonb_agg(to_jsonb(p) ORDER BY id) FROM profile p),
'role',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM user_role r),
'access',(SELECT jsonb_agg(to_jsonb(a) ORDER BY user_id,application_id) FROM user_application_access a),
'sessions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM sessions s),
'refresh',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM refresh_tokens r),
'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM audit_logs a WHERE action<>'SECURITY_ATTEMPT_FAILED'),
'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY event_id) FROM outbox_events o),
'result',(SELECT jsonb_agg(to_jsonb(i) ORDER BY actor_id,method,route_scope,key) FROM idempotency_records i))::text`).Scan(&value)
	if e != nil {
		t.Fatal("business snapshot", e)
	}
	return value
}

func postgresManagementFailures(t *testing.T) {
	for _, command := range []string{"create", "update", "status"} {
		stages := []string{"account", "audit", "outbox", "result", "commit"}
		if command == "create" {
			stages = append(stages, "profile", "member")
		}
		if command == "update" {
			stages = append(stages, "profile", "conditional")
		}
		if command == "status" {
			stages = append(stages, "sessions", "refresh")
		}
		for _, stage := range stages {
			t.Run(command+"/"+stage, func(t *testing.T) {
				h := newManagementHarness(t)
				before := managementBusinessState(t, h)
				table, op, condition := "auth", "UPDATE", "TRUE"
				if command == "create" {
					op = "INSERT"
				}
				switch stage {
				case "profile", "conditional":
					table = "profile"
				case "member":
					table, op = "user_role", "INSERT"
				case "sessions":
					table = "sessions"
				case "refresh":
					table = "refresh_tokens"
				case "audit":
					table, op, condition = "audit_logs", "INSERT", "NEW.action<>'SECURITY_ATTEMPT_FAILED'"
				case "outbox":
					table, op = "outbox_events", "INSERT"
				case "result", "commit":
					table, op = "idempotency_records", "INSERT"
				}
				body := "IF " + condition + " THEN RAISE EXCEPTION 'controlled failure' USING ERRCODE='23514'; END IF; RETURN NEW;"
				trigger := "CREATE TRIGGER management_fault AFTER " + op + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION management_fault()"
				if stage == "conditional" {
					body = "RETURN NULL;"
					trigger = "CREATE TRIGGER management_fault BEFORE UPDATE ON profile FOR EACH ROW EXECUTE FUNCTION management_fault()"
				}
				if stage == "commit" {
					trigger = "CREATE CONSTRAINT TRIGGER management_fault AFTER INSERT ON idempotency_records DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION management_fault()"
				}
				if _, e := h.f.DB.ExecContext(h.ctx, "CREATE FUNCTION management_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN "+body+" END $$; "+trigger); e != nil {
					t.Fatal("fault setup", e)
				}
				var e error
				method := userv1.UserService_CreateUser_FullMethodName
				switch command {
				case "create":
					_, e = h.users.CreateUser(h.actor("fault"), &userv1.CreateUserRequest{Name: "Fault target", Email: "fault@example.test", Password: "secret12", Status: managementStatus("ACTIVE")})
				case "update":
					method = userv1.UserService_UpdateUser_FullMethodName
					name := "Fault name"
					_, e = h.users.UpdateUser(h.actor("fault"), &userv1.UpdateUserRequest{UserId: 2, ExpectedUserVersion: managementVersion(1), Name: &name, FieldMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}}})
				case "status":
					method = userv1.UserService_ChangeUserStatus_FullMethodName
					_, e = h.users.ChangeUserStatus(h.actor("fault"), &userv1.ChangeUserStatusRequest{UserId: 2, ExpectedUserVersion: managementVersion(1), Status: managementStatus("SUSPENDED"), Reason: "Controlled review"})
				}
				want := codes.Unavailable
				if stage == "conditional" {
					want = codes.NotFound
				}
				if status.Code(e) != want {
					t.Fatal("fault classification", e)
				}
				if after := managementBusinessState(t, h); after != before {
					t.Fatal("partial business persistence after failed command")
				}
				var failures int
				if e = h.f.DB.QueryRowContext(h.ctx, `SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND target_id=$1`, method).Scan(&failures); e != nil || failures != 1 {
					t.Fatal("independent failure audit", failures, e)
				}
				if h.f.DB.Stats().InUse != 0 {
					t.Fatal("fault pool leak")
				}
			})
		}
	}
}

func postgresManagementQuota(t *testing.T) {
	h := newManagementHarness(t)
	ctx := shared.RPCContext(h.ctx, h.admin.Token)
	before := managementBusinessState(t, h)
	for i := 0; i < 5; i++ {
		var e error
		switch i % 3 {
		case 0:
			_, e = h.users.CreateUser(ctx, &userv1.CreateUserRequest{})
		case 1:
			_, e = h.users.UpdateUser(ctx, &userv1.UpdateUserRequest{})
		case 2:
			_, e = h.users.ChangeUserStatus(ctx, &userv1.ChangeUserStatusRequest{})
		}
		if status.Code(e) != codes.InvalidArgument {
			t.Fatal("invalid command bypassed admission", i, e)
		}
	}
	_, e := h.users.CreateUser(ctx, &userv1.CreateUserRequest{})
	d, ok := grpcerror.Decode(e)
	if !ok || d.Code != codes.ResourceExhausted || d.RetryInfo == nil || d.RetryInfo.RetryDelay.AsDuration() != 2*time.Second {
		t.Fatal("shared quota RetryInfo", e)
	}
	if after := managementBusinessState(t, h); after != before {
		t.Fatal("invalid/quota changed business state")
	}
	for i := 0; i < 20; i++ {
		if _, e = h.users.GetUser(ctx, &userv1.GetUserRequest{Id: 2}); e != nil {
			t.Fatal("read group", i, e)
		}
	}
	_, e = h.users.GetUser(ctx, &userv1.GetUserRequest{Id: 2})
	if status.Code(e) != codes.ResourceExhausted {
		t.Fatal("shared protected read quota", e)
	}
}
