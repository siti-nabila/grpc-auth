//go:build postgres

package test_scenarios

import (
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
	paginatorv1 "github.com/siti-nabila/api-contracts/pb/paginator/v1"
	profilev1 "github.com/siti-nabila/api-contracts/pb/profile/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/api-contracts/pkg/grpcerror"
	"github.com/siti-nabila/api-contracts/pkg/locale"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func RegisteredServer() []testutils.Scenario {
	return []testutils.Scenario{{Name: "complete production server accepts committed Login credentials and enforces all six RPCs", Run: postgresRegisteredServer}}
}

func postgresRegisteredServer(t *testing.T) {
	for _, restricted := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "restricted"}[restricted], func(t *testing.T) {
			ctx, fixture := loginFixture(t, restricted)
			conn, foundation := shared.FoundationServer(t, fixture.DB, func() time.Time { return shared.ServiceTime }, loginEntropy(0))
			users, profiles := userv1.NewUserServiceClient(conn), profilev1.NewProfileServiceClient(conn)
			loginCtx := shared.RPCContext(ctx, "")
			// Every registered method rejects an unknown service before persistence.
			for _, call := range []func() error{
				func() error { _, err := users.Register(ctx, &userv1.AuthRequest{}); return err },
				func() error { _, err := users.Login(ctx, &userv1.AuthRequest{}); return err },
				func() error { _, err := users.Me(ctx, &emptypb.Empty{}); return err },
				func() error { _, err := users.ListUsers(ctx, &userv1.ListUsersRequest{}); return err },
				func() error { _, err := users.TesRPC(ctx, &emptypb.Empty{}); return err },
				func() error { _, err := profiles.UpdateProfile(ctx, &profilev1.ProfileRequest{}); return err },
			} {
				if err := call(); status.Code(err) != codes.Unauthenticated {
					t.Fatal("service boundary bypass", err)
				}
			}
			var initial int
			if err := fixture.DB.QueryRowContext(ctx, "SELECT count(*) FROM audit_logs").Scan(&initial); err != nil || initial != 0 {
				t.Fatal("unverified service wrote audit")
			}
			tes, err := users.TesRPC(loginCtx, &emptypb.Empty{})
			if err != nil || tes.Res != "WELCOME ANJING !!!!!!!" {
				t.Fatal("TesRPC contract changed", err)
			}
			issued, err := users.Login(loginCtx, &userv1.AuthRequest{Email: " FIXTURE@example.test ", Password: "secret12"})
			if err != nil || issued.Token == "" || issued.RefreshToken == "" || issued.MustChangePassword != restricted {
				t.Fatal("registered Login did not issue", err)
			}
			assertLoginRows(t, ctx, fixture, 1, 0)
			actor := shared.RPCContext(ctx, issued.Token)
			me, err := users.Me(actor, &emptypb.Empty{})
			if err != nil || me.Id != 1 || me.MustChangePassword != restricted || restricted && (len(me.Roles) != 0 || len(me.RoleCodes) != 0 || len(me.ApplicationAccess) != 0) {
				t.Fatal("registered Me failed", err)
			}
			list := &userv1.ListUsersRequest{Query: &paginatorv1.PageQuery{Limit: 10, LastId: "0"}}
			_, err = users.ListUsers(actor, list)
			assertRPCError(t, err, codes.PermissionDenied, common.ErrForbidden.Key(), common.ErrForbidden.Message(locale.Indonesian))
			// Public Register never issues credentials, even with a bearer supplied.
			registered, err := users.Register(actor, &userv1.AuthRequest{Email: " NEW@example.test ", Password: "secret12"})
			if err != nil || registered.Token != "" || registered.RefreshToken != "" || registered.SessionId != "" || registered.RegistrationStatus != "PENDING" || registered.RegistrationId != 2 {
				t.Fatal("registered PENDING flow failed", err)
			}
			assertRegistrationCounts(t, ctx, fixture, "new@example.test", 1)
			var sessionCount, pendingEvents int
			if err := fixture.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM sessions),(SELECT count(*) FROM outbox_events WHERE event_type='iam.user.created.v1' AND published_at IS NULL)`).Scan(&sessionCount, &pendingEvents); err != nil || sessionCount != 1 || pendingEvents != 1 {
				t.Fatal("Register issued credentials or lost event")
			}
			if restricted {
				_, err = profiles.UpdateProfile(actor, &profilev1.ProfileRequest{Name: "Denied"})
				assertRPCError(t, err, codes.PermissionDenied, common.ErrForbidden.Key(), common.ErrForbidden.Message(locale.Indonesian))
				var version int
				if err := fixture.DB.QueryRowContext(ctx, "SELECT version FROM auth WHERE id=1").Scan(&version); err != nil || version != 1 {
					t.Fatal("restricted mutation happened")
				}
			} else {
				// Add a separate reader role, retaining a safe default member.
				if _, err := fixture.DB.ExecContext(ctx, `INSERT INTO permissions(application_id,code,description) VALUES(1,'iam.user.read','fixture'); INSERT INTO role(role_name,role_code,application_id,code) VALUES('Reader',2,1,'iam.reader'); INSERT INTO role_permissions VALUES(2,1); INSERT INTO user_role(user_id,role_id) VALUES(1,2)`); err != nil {
					t.Fatal("owned grant fixture failed")
				}
				page, err := users.ListUsers(actor, list)
				if err != nil || len(page.Items) != 1 || page.Items[0].Id != 1 || page.NextCursor != "1" {
					t.Fatal("registered authorized list failed", err)
				}
				changed, err := profiles.UpdateProfile(actor, &profilev1.ProfileRequest{Name: "Changed"})
				if err != nil || !changed.Changed || changed.Version != 2 {
					t.Fatal("registered profile mutation failed", err)
				}
				again, err := profiles.UpdateProfile(actor, &profilev1.ProfileRequest{Name: "Changed"})
				if err != nil || again.Changed || again.Version != 2 {
					t.Fatal("registered profile no-op failed", err)
				}
				assertFoundationProfile(t, ctx, fixture, "Changed", 2, 1, 1)
				if _, err := fixture.DB.ExecContext(ctx, `UPDATE role SET status='DISABLED' WHERE id=2`); err != nil {
					t.Fatal(err)
				}
				list.Query.LastId = "999"
				_, err = users.ListUsers(actor, list)
				assertRPCError(t, err, codes.PermissionDenied, common.ErrForbidden.Key(), common.ErrForbidden.Message(locale.Indonesian))
			}
			// Legacy and sid-less tokens cannot reach primary session/consumer reads.
			legacy, err := jwtlib.NewWithClaims(jwtlib.SigningMethodHS256, jwtlib.MapClaims{"user_id": 1, "exp": shared.ServiceTime.Add(time.Minute).Unix()}).SignedString([]byte("test-only-legacy-key"))
			if err != nil {
				t.Fatal(err)
			}
			missingSid := shared.AccessPayload()
			delete(missingSid, "sid")
			for _, token := range []string{legacy, shared.SignAccess(t, missingSid)} {
				_, err = users.Me(shared.RPCContext(ctx, token), &emptypb.Empty{})
				assertRPCError(t, err, codes.Unauthenticated, auth.ErrInvalidSession.Key(), auth.ErrInvalidSession.Message(locale.Indonesian))
			}
			// A dependency failure rolls back the snapshot, encodes locale and audits
			// separately after rollback; no SQL detail is returned to the caller.
			if _, err := fixture.DB.ExecContext(ctx, `ALTER TABLE profile RENAME TO profile_unavailable`); err != nil {
				t.Fatal(err)
			}
			_, err = users.Me(actor, &emptypb.Empty{})
			assertRPCError(t, err, codes.Unavailable, common.ErrDependencyUnavailable.Key(), common.ErrDependencyUnavailable.Message(locale.Indonesian))
			if _, err := fixture.DB.ExecContext(ctx, `ALTER TABLE profile_unavailable RENAME TO profile`); err != nil {
				t.Fatal(err)
			}
			// Three canonical account attempts consume the burst; rejection emits
			// RetryInfo and no extra audit transaction on the fourth attempt.
			for n := 0; n < 4; n++ {
				_, err = users.Login(loginCtx, &userv1.AuthRequest{Email: " NEW@example.test ", Password: "secret12"})
				if n < 3 {
					assertRPCError(t, err, codes.Unauthenticated, auth.ErrLoginFailed.Key(), auth.ErrLoginFailed.Message(locale.Indonesian))
				} else {
					assertRPCError(t, err, codes.ResourceExhausted, common.ErrRateLimited.Key(), common.ErrRateLimited.Message(locale.Indonesian))
					decoded, _ := grpcerror.Decode(err)
					if decoded.RetryInfo == nil || decoded.RetryInfo.RetryDelay.AsDuration() != 12*time.Second {
						t.Fatal("RetryInfo was lost")
					}
				}
			}
			var denials int
			if err := fixture.DB.QueryRowContext(ctx, `SELECT count(*) FROM audit_logs WHERE action='LOGIN_FAILED'`).Scan(&denials); err != nil || denials != 3 {
				t.Fatal("quota rejection wrote per-request audit")
			}
			if _, err := fixture.DB.ExecContext(ctx, `UPDATE sessions SET status='REVOKED',revoked_at=$1,revoke_reason='fixture revoke' WHERE id=$2`, shared.ServiceTime, issued.SessionId); err != nil {
				t.Fatal(err)
			}
			_, err = users.Me(actor, &emptypb.Empty{})
			assertRPCError(t, err, codes.Unauthenticated, auth.ErrInvalidSession.Key(), auth.ErrInvalidSession.Message(locale.Indonesian))
			if foundation.Login.AuditFailures() != 0 || fixture.DB.Stats().InUse != 0 {
				t.Fatal("registered pipeline leaked transaction or failed audit")
			}
			t.Log("all six production RPCs: TLS/caller, real Login, primary Me/List, pending Register, profile/restricted/no-op, legacy/revoked denial, locale, RetryInfo PASS")
		})
	}
}

func assertRPCError(t *testing.T, err error, code codes.Code, reason, message string) {
	t.Helper()
	got, ok := grpcerror.Decode(err)
	if !ok || got.Code != code || got.Reason != reason || got.Message != message {
		t.Fatalf("registered RPC error mismatch: %v", err)
	}
}
