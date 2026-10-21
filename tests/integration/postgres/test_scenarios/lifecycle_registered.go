//go:build postgres

package test_scenarios

import (
	"context"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync/atomic"
	"testing"
	"time"
)

func RegisteredLifecycle() []testutils.Scenario {
	return []testutils.Scenario{{Name: "production registered lifecycle Login refresh protected self admin password Logout and atomic failure", Run: postgresRegisteredLifecycle}, {Name: "registered restricted principal password and Logout flows", Run: postgresRegisteredRestricted}, {Name: "registered Refresh IP/session quotas and RetryInfo are atomic", Run: postgresRegisteredRefreshQuota}}
}
func postgresRegisteredLifecycle(t *testing.T) {
	ctx, f := loginFixture(t, false)
	if _, e := f.DB.ExecContext(ctx, `
 INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.session.read','fixture'),(1,'iam.session.revoke','fixture'),(1,'iam.user.read','fixture');
 INSERT INTO role(role_name,role_code,application_id,code) VALUES('Admin',2,1,'iam.admin'),('Auditor',3,1,'iam.auditor');
 INSERT INTO role_permissions(role_id,permission_id) SELECT 2,id FROM permissions;
 INSERT INTO user_role(user_id,role_id) VALUES(1,2);
 INSERT INTO auth(email,password,status) SELECT 'target@example.test',password,'ACTIVE' FROM auth WHERE id=1;
 INSERT INTO profile(user_id,name,address,phone) VALUES(2,'Target','Display address','123');
 INSERT INTO user_profile_search SELECT id,to_tsvector('simple','Target target@example.test'),'target target@example.test' FROM profile WHERE user_id=2;
 INSERT INTO user_role(user_id,role_id) VALUES(2,1),(2,3);
 INSERT INTO applications(code,name) VALUES('clm','Contracts');
 INSERT INTO user_application_access(user_id,application_id) VALUES(2,1),(2,2);`); e != nil {
		t.Fatal("registered setup", e)
	}
	var clock atomic.Int64
	clock.Store(shared.ServiceTime.UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()).UTC() }
	conn, foundation := shared.FoundationServer(t, f.DB, now, shared.LifecycleEntropy(), shared.LifecycleMethods()...)
	users, auths, sessions := userv1.NewUserServiceClient(conn), authv1.NewAuthServiceClient(conn), sessionv1.NewSessionServiceClient(conn)
	login := func(email, password string) *userv1.UserTokenResponse {
		t.Helper()
		clock.Add(int64(12 * time.Second))
		r, e := users.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: email, Password: password})
		if e != nil {
			t.Fatal("registered Login", e)
		}
		return r
	}
	admin := login("fixture@example.test", "secret12")
	target := login("target@example.test", "secret12")
	adminCtx := shared.RPCContext(ctx, admin.Token)
	idem := func(key string) context.Context {
		return metadata.AppendToOutgoingContext(adminCtx, "idempotency-key", key)
	}
	rotated, e := auths.RefreshSession(shared.RPCContext(ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: target.RefreshToken})
	if e != nil || rotated.SessionId != target.SessionId || rotated.RefreshToken == target.RefreshToken {
		t.Fatal("registered rotation", e)
	}
	consumed := target.RefreshToken
	target = rotated
	if _, e = auths.Logout(shared.RPCContext(ctx, ""), &authv1.LogoutRequest{RefreshToken: &consumed}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("consumed Logout accepted", e)
	}
	actor := shared.RPCContext(ctx, target.Token)
	if r, e := users.Me(actor, &emptypb.Empty{}); e != nil || r.Id != 2 {
		t.Fatal("rotated protected Me", e)
	}
	mine, e := sessions.ListMySessions(actor, &sessionv1.ListMySessionsRequest{})
	if e != nil || len(mine.Items) != 1 || !mine.Items[0].IsCurrent || mine.Items[0].Id != target.SessionId {
		t.Fatal("registered self list", e)
	}
	all, e := sessions.ListUserSessions(adminCtx, &sessionv1.ListUserSessionsRequest{UserId: 2})
	if e != nil || len(all.Items) != 1 || all.Items[0].IsCurrent {
		t.Fatal("registered admin list", e)
	}
	page, e := users.ListUsers(adminCtx, &userv1.ListUsersRequest{Query: &paginatorv1.PageQuery{Limit: 1, Sort: []*paginatorv1.Sort{{Field: "roles", Desc: true}, {Field: "email"}}, Search: &paginatorv1.Search{Keyword: "Target"}}, Filter: &userv1.UserFilter{RoleCodes: []uint64{1, 3}, Statuses: []string{"ACTIVE"}, ApplicationCode: "clm"}})
	if e != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].Id != 2 || len(page.Items[0].Roles) != 2 || len(page.Items[0].ApplicationAccess) != 2 || page.Items[0].CreatedAt == nil || page.Items[0].Version != 1 || page.NextCursor == "" {
		t.Fatal("registered user list safe collection/filter/sort", page, e)
	}
	single := &sessionv1.RevokeUserSessionRequest{UserId: 2, SessionId: target.SessionId}
	for n := 0; n < 2; n++ {
		r, e := sessions.RevokeUserSession(idem("single"), single)
		if e != nil || !r.Changed {
			t.Fatal("registered single/replay", e)
		}
	}
	if r, e := sessions.RevokeUserSession(idem("terminal"), single); e != nil || r.Changed {
		t.Fatal("registered new-key no-op", e)
	}
	if _, e = users.Me(actor, &emptypb.Empty{}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("revoked protected allowed", e)
	}
	target = login("target@example.test", "secret12")
	actor = shared.RPCContext(ctx, target.Token)
	if r, e := sessions.RevokeMySession(actor, &sessionv1.RevokeMySessionRequest{SessionId: target.SessionId}); e != nil || !r.Changed {
		t.Fatal("registered self single", e)
	}
	target = login("target@example.test", "secret12")
	actor = shared.RPCContext(ctx, target.Token)
	if r, e := sessions.RevokeAllMySessions(actor, &emptypb.Empty{}); e != nil || !r.Changed || !r.LoginRequired {
		t.Fatal("registered self global", e)
	}
	target = login("target@example.test", "secret12")
	actor = shared.RPCContext(ctx, target.Token)
	expected := uint64(2)
	global := &sessionv1.RevokeAllUserSessionsRequest{UserId: 2, ExpectedUserVersion: &expected}
	for n := 0; n < 2; n++ {
		r, e := sessions.RevokeAllUserSessions(idem("global"), global)
		if e != nil || !r.Changed || r.Version != 3 {
			t.Fatal("registered global/replay", r, e)
		}
	}
	if _, e = users.Me(actor, &emptypb.Empty{}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("global old credential allowed", e)
	}
	target = login("target@example.test", "secret12")
	actor = shared.RPCContext(ctx, target.Token)
	if _, e = auths.ChangePassword(actor, &authv1.ChangePasswordRequest{CurrentPassword: "wrong12", NewPassword: "newSecret123"}); status.Code(e) != codes.InvalidArgument {
		t.Fatal("current password proof", e)
	}
	if r, e := auths.ChangePassword(actor, &authv1.ChangePasswordRequest{CurrentPassword: "secret12", NewPassword: "newSecret123"}); e != nil || !r.Changed || !r.LoginRequired {
		t.Fatal("registered password", e)
	}
	oldRefresh := target.RefreshToken
	if _, e = auths.Logout(shared.RPCContext(ctx, ""), &authv1.LogoutRequest{RefreshToken: &oldRefresh}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("old generation terminal proof allowed", e)
	}
	clock.Add(int64(12 * time.Second))
	if _, e = users.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "target@example.test", Password: "secret12"}); status.Code(e) != codes.Unauthenticated {
		t.Fatal("old password allowed", e)
	}
	target = login("target@example.test", "newSecret123")
	actor = shared.RPCContext(ctx, target.Token)
	var beforeEvents int
	if e = f.DB.QueryRowContext(ctx, "SELECT count(*) FROM outbox_events").Scan(&beforeEvents); e != nil {
		t.Fatal(e)
	}
	if _, e = f.DB.ExecContext(ctx, `CREATE FUNCTION reject_registered_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled private persistence failure' USING ERRCODE='23514'; END $$; CREATE TRIGGER reject_registered_outbox AFTER INSERT ON outbox_events FOR EACH ROW EXECUTE FUNCTION reject_registered_outbox()`); e != nil {
		t.Fatal(e)
	}
	_, e = auths.Logout(actor, &authv1.LogoutRequest{})
	assertRPCError(t, e, codes.Unavailable, common.ErrDependencyUnavailable.Key(), common.ErrDependencyUnavailable.Message(locale.Indonesian))
	var active, events, failureAudit int
	if e = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions WHERE id=$1 AND status='ACTIVE'),(SELECT count(*) FROM outbox_events),(SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND target_id='/auth.v1.AuthService/Logout' AND reason='COMMON_DEPENDENCY_UNAVAILABLE')`, target.SessionId).Scan(&active, &events, &failureAudit); e != nil || active != 1 || events != beforeEvents || failureAudit != 1 {
		t.Fatal("registered rollback before independent audit", active, events, failureAudit, e)
	}
	if _, e = f.DB.ExecContext(ctx, "DROP TRIGGER reject_registered_outbox ON outbox_events"); e != nil {
		t.Fatal(e)
	}
	if r, e := auths.Logout(actor, &authv1.LogoutRequest{}); e != nil || !r.Changed {
		t.Fatal("registered Logout access", e)
	}
	if r, e := auths.Logout(actor, &authv1.LogoutRequest{}); e != nil || r.Changed {
		t.Fatal("registered Logout terminal access", e)
	}
	if r, e := auths.Logout(shared.RPCContext(ctx, ""), &authv1.LogoutRequest{RefreshToken: &target.RefreshToken}); e != nil || r.Changed {
		t.Fatal("registered Logout terminal refresh", e)
	}
	if foundation.Login.AuditFailures() != 0 || f.DB.Stats().InUse != 0 {
		t.Fatal("registered telemetry/pool leak")
	}
}

func postgresRegisteredRestricted(t *testing.T) {
	ctx, f := loginFixture(t, true)
	conn, foundation := shared.FoundationServer(t, f.DB, func() time.Time { return shared.ServiceTime }, shared.LifecycleEntropy(), shared.LifecycleMethods()...)
	users, auths, sessions := userv1.NewUserServiceClient(conn), authv1.NewAuthServiceClient(conn), sessionv1.NewSessionServiceClient(conn)
	initial, e := users.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "fixture@example.test", Password: "secret12"})
	if e != nil || !initial.MustChangePassword {
		t.Fatal("restricted Login", e)
	}
	actor := shared.RPCContext(ctx, initial.Token)
	if me, e := users.Me(actor, &emptypb.Empty{}); e != nil || !me.MustChangePassword || len(me.Roles) > 0 {
		t.Fatal("restricted Me", e)
	}
	for _, call := range []func() error{
		func() error { _, e := sessions.ListMySessions(actor, &sessionv1.ListMySessionsRequest{}); return e },
		func() error {
			_, e := sessions.ListUserSessions(actor, &sessionv1.ListUserSessionsRequest{UserId: 1})
			return e
		},
		func() error {
			_, e := sessions.RevokeMySession(actor, &sessionv1.RevokeMySessionRequest{SessionId: initial.SessionId})
			return e
		},
		func() error { _, e := sessions.RevokeAllMySessions(actor, &emptypb.Empty{}); return e },
		func() error {
			_, e := sessions.RevokeUserSession(actor, &sessionv1.RevokeUserSessionRequest{UserId: 1, SessionId: initial.SessionId})
			return e
		},
		func() error {
			_, e := sessions.RevokeAllUserSessions(actor, &sessionv1.RevokeAllUserSessionsRequest{UserId: 1})
			return e
		},
	} {
		if e = call(); status.Code(e) != codes.PermissionDenied {
			t.Fatal("restricted method allowed", e)
		}
	}
	if _, e = auths.RefreshSession(shared.RPCContext(ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: initial.RefreshToken}); status.Code(e) != codes.PermissionDenied {
		t.Fatal("restricted refresh allowed", e)
	}
	other, e := users.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "fixture@example.test", Password: "secret12"})
	if e != nil {
		t.Fatal(e)
	}
	if r, e := auths.Logout(shared.RPCContext(ctx, other.Token), &authv1.LogoutRequest{}); e != nil || !r.Changed {
		t.Fatal("restricted Logout", e)
	}
	if r, e := auths.ChangePassword(actor, &authv1.ChangePasswordRequest{CurrentPassword: "secret12", NewPassword: "newSecret123"}); e != nil || !r.Changed || !r.LoginRequired {
		t.Fatal("restricted password", e)
	}
	fresh, e := users.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "fixture@example.test", Password: "newSecret123"})
	if e != nil || fresh.MustChangePassword {
		t.Fatal("fresh unrestricted Login", e)
	}
	if r, e := sessions.ListMySessions(shared.RPCContext(ctx, fresh.Token), &sessionv1.ListMySessionsRequest{}); e != nil || len(r.Items) != 3 {
		t.Fatal("fresh unrestricted list", e)
	}
	if foundation.Login.AuditFailures() != 0 || f.DB.Stats().InUse != 0 {
		t.Fatal("restricted cleanup")
	}
}
func postgresRegisteredRefreshQuota(t *testing.T) {
	ctx, f := loginFixture(t, false)
	conn, foundation := shared.FoundationServer(t, f.DB, func() time.Time { return shared.ServiceTime }, shared.LifecycleEntropy(), shared.LifecycleMethods()...)
	users, auths, sessions := userv1.NewUserServiceClient(conn), authv1.NewAuthServiceClient(conn), sessionv1.NewSessionServiceClient(conn)
	target, e := users.Login(shared.RPCContext(ctx, ""), &userv1.AuthRequest{Email: "fixture@example.test", Password: "secret12"})
	if e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 2; n++ {
		target, e = auths.RefreshSession(shared.RPCContext(ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: target.RefreshToken})
		if e != nil {
			t.Fatal("refresh burst", n, e)
		}
	}
	_, e = auths.RefreshSession(shared.RPCContext(ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: target.RefreshToken})
	decoded, ok := grpcerror.Decode(e)
	if !ok || status.Code(e) != codes.ResourceExhausted || decoded.RetryInfo == nil || decoded.RetryInfo.RetryDelay.AsDuration() != 6*time.Second || decoded.Message != common.ErrRateLimited.Message(locale.Indonesian) {
		t.Fatal("registered session RetryInfo", e)
	}
	// Eight remaining IP tokens survive the failed session admission.
	for n := 0; n < 8; n++ {
		_, e = auths.RefreshSession(shared.RPCContext(ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: "malformed"})
		if status.Code(e) != codes.Unauthenticated {
			t.Fatal("failed session consumed IP", n, e)
		}
	}
	if _, e = auths.RefreshSession(shared.RPCContext(ctx, ""), &authv1.RefreshSessionRequest{RefreshToken: "malformed"}); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("IP burst", e)
	}
	var refreshCount, rotations, consumed int
	if e = f.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM refresh_tokens),(SELECT count(*) FROM audit_logs WHERE action='SESSION_REFRESH_ROTATED'),(SELECT count(*) FROM refresh_tokens WHERE consumed_at IS NOT NULL)`).Scan(&refreshCount, &rotations, &consumed); e != nil || refreshCount != 3 || rotations != 2 || consumed != 2 {
		t.Fatal("quota mutated refresh state", refreshCount, rotations, consumed, e)
	}
	actor := shared.RPCContext(ctx, target.Token)
	if _, e = users.Me(actor, &emptypb.Empty{}); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 19; n++ {
		if _, e = sessions.ListMySessions(actor, &sessionv1.ListMySessionsRequest{}); e != nil {
			t.Fatal("shared read burst", n, e)
		}
	}
	if _, e = sessions.ListMySessions(actor, &sessionv1.ListMySessionsRequest{}); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("reads not shared", e)
	}
	if foundation.Login.AuditFailures() != 0 || f.DB.Stats().InUse != 0 {
		t.Fatal("quota cleanup")
	}
}
