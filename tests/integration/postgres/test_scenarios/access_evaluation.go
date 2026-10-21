//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/authorization"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const evaluateMethod = authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName

func AccessEvaluation() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "registered original JWT decisions follow commands mappings and application gates", Run: postgresEvaluationCommands},
		{Name: "two physical backends preserve decision identity and next-snapshot visibility", Run: postgresEvaluationSnapshots},
		{Name: "inactive accounts and revoked credentials cannot use descriptive role views", Run: postgresEvaluationIdentity},
		{Name: "evaluation cancellation panic driver finalization and audit failures release the pool", Run: postgresEvaluationCleanup},
	}
}

func evaluator(t *testing.T, h *roleHarness) authorizationv1.AuthorizationServiceClient {
	t.Helper()
	cfg := shared.ServiceConfig()
	cfg.Callers[0].Methods = append(cfg.Callers[0].Methods, evaluateMethod)
	cfg.Callers[0].Namespaces = []string{"iam", "sales", "clm", "unknown"}
	conn, _ := shared.FoundationServerWithService(t, h.f.DB, h.now, shared.LifecycleEntropy(), cfg)
	return authorizationv1.NewAuthorizationServiceClient(conn)
}

func postgresEvaluationCommands(t *testing.T) {
	h := newApplicationHarness(t)
	if _, err := h.f.DB.ExecContext(h.ctx, `INSERT INTO applications(code,name) VALUES('clm','Other application');INSERT INTO permissions(application_id,code,description) SELECT id,'clm.order.read','Other read' FROM applications WHERE code='clm';INSERT INTO role(role_name,role_code,application_id,code) SELECT 'Other reader',9,id,'clm.reader' FROM applications WHERE code='clm';INSERT INTO role_permissions(role_id,permission_id) SELECT r.id,p.id FROM role r JOIN permissions p ON p.application_id=r.application_id WHERE r.code='clm.reader';INSERT INTO user_role(user_id,role_id) SELECT 2,id FROM role WHERE code='clm.reader';INSERT INTO user_application_access(user_id,application_id) SELECT 2,id FROM applications WHERE code='clm'`); err != nil {
		t.Fatal(err)
	}
	client := evaluator(t, h)
	credentials := accessCredentialState(t, h)
	assert := func(app, permission string, reason authorizationv1.AccessDecisionReason) {
		t.Helper()
		h.clock.Add(int64(2 * time.Second))
		before := roleBusinessState(t, h)
		var attempts int
		if err := h.f.DB.QueryRowContext(h.ctx, `SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND target_id=$1`, evaluateMethod).Scan(&attempts); err != nil {
			t.Fatal(err)
		}
		res, err := client.EvaluateAccess(shared.RPCContext(h.ctx, h.target.Token), &authorizationv1.EvaluateAccessRequest{ApplicationCode: app, PermissionCode: permission})
		if err != nil || res.Reason != reason || res.Allowed != (reason == 1) || res.UserId != 2 || res.Sid != h.target.SessionId || !res.EvaluatedAt.AsTime().Equal(h.now()) {
			t.Fatal(res, err, reason)
		}
		if before != roleBusinessState(t, h) || credentials != accessCredentialState(t, h) {
			t.Fatal("evaluation changed business state or access commands changed credentials")
		}
		var after int
		if err := h.f.DB.QueryRowContext(h.ctx, `SELECT count(*) FROM audit_logs WHERE action='SECURITY_ATTEMPT_FAILED' AND target_id=$1`, evaluateMethod).Scan(&after); err != nil {
			t.Fatal(err)
		}
		want := attempts
		if !res.Allowed {
			want++
		}
		if after != want {
			t.Fatal("denial audit count", after, want)
		}
	}
	allowed := authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_ALLOWED
	missing := authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_PERMISSION_MISSING
	assert("sales", "sales.order.read", allowed)
	if _, err := h.users.RevokeUserRoles(h.actor("eval-revoke"), assignmentRevoke(2, 1, 4)); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", allowed)
	if _, err := h.roles.ChangeRoleStatus(h.actor("eval-disable"), roleStatus(7, "DISABLED")); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", missing)
	if _, err := h.roles.ChangeRoleStatus(h.actor("eval-enable"), roleStatus(8, "ACTIVE")); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", allowed)
	if _, err := h.roles.ReplaceRolePermissions(h.actor("eval-remove"), roleReplace(9, "sales.order.update")); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", missing)
	if _, err := h.users.GrantUserRoles(h.actor("eval-overlap"), assignmentGrant(2, 2, 4)); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", allowed)
	if _, err := h.roles.ReplaceRolePermissions(h.actor("eval-restore"), roleReplace(10, "sales.order.read", "sales.order.update")); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.update", allowed)
	if _, err := h.applications.RevokeApplicationAccess(h.actor("eval-app-revoke"), applicationRevoke(3)); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_ACCESS_REQUIRED)
	assert("clm", "clm.order.read", allowed)
	if _, err := h.applications.GrantApplicationAccess(h.actor("eval-subset"), applicationGrant(4, 4)); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", allowed)
	assert("sales", "sales.order.update", missing)
	assert("unknown", "unknown.resource.read", missing)
	if _, err := h.f.DB.ExecContext(h.ctx, `UPDATE applications SET status='DISABLED' WHERE code='sales'`); err != nil {
		t.Fatal(err)
	}
	assert("sales", "sales.order.read", authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_INACTIVE)
	assert("clm", "clm.order.read", allowed)
	if _, err := h.f.DB.ExecContext(h.ctx, `DELETE FROM user_application_access WHERE user_id=1 AND application_id=1`); err != nil {
		t.Fatal(err)
	}
	h.clock.Add(int64(2 * time.Second))
	res, err := client.EvaluateAccess(shared.RPCContext(h.ctx, h.admin.Token), &authorizationv1.EvaluateAccessRequest{ApplicationCode: "iam", PermissionCode: "iam.user.read"})
	if err != nil || !res.Allowed || res.UserId != 1 {
		t.Fatal("IAM without app grant", res, err)
	}
}

func postgresEvaluationSnapshots(t *testing.T) {
	for _, kind := range []string{"assignment", "mapping", "application", "access", "session", "account"} {
		t.Run(kind, func(t *testing.T) {
			h := newApplicationHarness(t)
			ctx, cancel := context.WithTimeout(shared.PermissionCallerContext(t, evaluateMethod, "sales"), 5*time.Second)
			defer cancel()
			claims, err := loginSecurity(t, h.now).Verifier.Verify(h.target.Token)
			if err != nil {
				t.Fatal(err)
			}
			provider := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
			read := func(p domain.ReadSnapshotProvider) (domain.AccessDecision, error) {
				return authorization.EvaluateAccessSnapshot(ctx, claims, domain.EvaluateAccessInput{ApplicationCode: "sales", PermissionCode: "sales.order.read"}, p, h.now())
			}
			ready, release := make(chan struct{}), make(chan struct{})
			barrier := &snapshotBarrier{provider: provider, ready: ready, release: release}
			done := make(chan snapshotResult, 1)
			released, finished := false, false
			defer func() {
				if !released {
					close(release)
				}
				if !finished {
					select {
					case <-done:
					case <-ctx.Done():
						t.Error("evaluation worker cleanup")
					}
				}
			}()
			go func() { r, e := read(barrier); done <- snapshotResult{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("before barrier", r.err)
			case <-ctx.Done():
				t.Fatal("barrier timeout")
			}
			conn, err := h.f.DB.Conn(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			pid := physicalIdentity(t, ctx, conn, h.f.Name)
			if pid == barrier.pid || barrier.pid == 0 {
				t.Fatal("two physical backends required")
			}
			statement := `UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Evaluation review' WHERE user_id=2 AND role_id IN(3,4)`
			switch kind {
			case "mapping":
				statement = `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id IN(3,4)`
			case "application":
				statement = `UPDATE applications SET status='DISABLED' WHERE code='sales'`
			case "access":
				statement = `UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Evaluation review' WHERE user_id=2 AND application_id=2`
			case "session":
				statement = `UPDATE sessions SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Evaluation review' WHERE user_id=2`
			case "account":
				statement = `UPDATE auth SET status='SUSPENDED',version=version+1 WHERE id=2`
			}
			tx, err := conn.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			close(release)
			released = true
			var old snapshotResult
			select {
			case old = <-done:
				finished = true
			case <-ctx.Done():
				t.Fatal("evaluation completion timeout")
			}
			if old.err != nil || !old.res.(domain.AccessDecision).Allowed || old.res.(domain.AccessDecision).Version != 1 {
				t.Fatal("mixed snapshot", old)
			}
			next, err := read(provider)
			if kind == "session" || kind == "account" {
				if !errors.Is(err, auth.ErrInvalidSession) || !reflect.DeepEqual(next, domain.AccessDecision{}) {
					t.Fatal(next, err)
				}
			} else {
				want := types.DecisionPermissionMissing
				if kind == "application" {
					want = types.DecisionApplicationInactive
				}
				if kind == "access" {
					want = types.DecisionApplicationAccessRequired
				}
				if err != nil || next.Allowed || next.Reason != want {
					t.Fatal(next, err, want)
				}
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("snapshot lease leaked")
			}
			t.Logf("evaluation RR backend=%d concurrent commit backend=%d", barrier.pid, pid)
		})
	}
}

func postgresEvaluationIdentity(t *testing.T) {
	for _, kind := range []string{"suspend", "deactivate", "global revoke", "password"} {
		t.Run(kind, func(t *testing.T) {
			h := newApplicationHarness(t)
			client := evaluator(t, h)
			switch kind {
			case "suspend", "deactivate":
				state := "SUSPENDED"
				if kind == "deactivate" {
					state = "DEACTIVATED"
				}
				if _, err := h.users.ChangeUserStatus(h.actor("eval-status"), &userv1.ChangeUserStatusRequest{UserId: 2, Status: &state, ExpectedUserVersion: managementVersion(1), Reason: "Evaluation review"}); err != nil {
					t.Fatal(err)
				}
				view, err := permissionClient(t, h).GetUserEffectivePermissions(h.actor(""), &userv1.GetUserEffectivePermissionsRequest{UserId: 2, ApplicationCode: "sales"})
				if err != nil || len(view.Permissions) == 0 || view.UserStatus != state {
					t.Fatal("descriptive inactive view", view, err)
				}
				state = "ACTIVE"
				if _, err := h.users.ChangeUserStatus(h.actor("eval-reactivate"), &userv1.ChangeUserStatusRequest{UserId: 2, Status: &state, ExpectedUserVersion: managementVersion(2), Reason: "Evaluation review"}); err != nil {
					t.Fatal(err)
				}
			case "global revoke":
				if _, err := sessionv1.NewSessionServiceClient(h.conn).RevokeAllMySessions(shared.RPCContext(h.ctx, h.target.Token), &emptypb.Empty{}); err != nil {
					t.Fatal(err)
				}
			case "password":
				if _, err := h.auths.ChangePassword(shared.RPCContext(h.ctx, h.target.Token), &authv1.ChangePasswordRequest{CurrentPassword: "secret12", NewPassword: "replacement12"}); err != nil {
					t.Fatal(err)
				}
			}
			res, err := client.EvaluateAccess(shared.RPCContext(h.ctx, h.target.Token), &authorizationv1.EvaluateAccessRequest{ApplicationCode: "sales", PermissionCode: "sales.order.read"})
			if status.Code(err) != codes.Unauthenticated || res != nil {
				t.Fatal("revoked credentials restored", res, err)
			}
		})
	}
}

func postgresEvaluationCleanup(t *testing.T) {
	for _, kind := range []string{"cancel", "deadline", "panic", "driver", "commit", "audit failure"} {
		t.Run(kind, func(t *testing.T) {
			h := newApplicationHarness(t)
			if kind == "audit failure" {
				client := evaluator(t, h)
				if _, err := h.f.DB.ExecContext(h.ctx, `UPDATE applications SET status='DISABLED' WHERE code='sales';CREATE FUNCTION evaluation_audit_fault() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'controlled audit failure';END $$;CREATE TRIGGER evaluation_audit_fault BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION evaluation_audit_fault()`); err != nil {
					t.Fatal(err)
				}
				res, err := client.EvaluateAccess(shared.RPCContext(h.ctx, h.target.Token), &authorizationv1.EvaluateAccessRequest{ApplicationCode: "sales", PermissionCode: "sales.order.read"})
				if err != nil || res.Allowed || res.Reason != authorizationv1.AccessDecisionReason_ACCESS_DECISION_REASON_APPLICATION_INACTIVE {
					t.Fatal(res, err)
				}
			} else {
				ctx, cancel := context.WithCancel(shared.PermissionCallerContext(t, evaluateMethod, "sales"))
				defer cancel()
				claims, err := loginSecurity(t, h.now).Verifier.Verify(h.target.Token)
				if err != nil {
					t.Fatal(err)
				}
				base := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
				var provider domain.ReadSnapshotProvider = base
				want := error(common.ErrDependencyUnavailable)
				switch kind {
				case "cancel":
					provider = featuremocks.PermissionSnapshot{Provider: base, CancelAt: 2, Cancel: cancel}
					want = context.Canceled
				case "deadline":
					var stop context.CancelFunc
					ctx, stop = context.WithDeadline(ctx, shared.ServiceTime)
					defer stop()
					want = context.DeadlineExceeded
				case "panic":
					provider = featuremocks.PermissionSnapshot{Provider: base, PanicAt: 2}
					want = common.ErrInternalServerError
				case "driver":
					if _, err := h.f.DB.ExecContext(h.ctx, `DROP TABLE role_permissions`); err != nil {
						t.Fatal(err)
					}
				case "commit":
					provider = permissionCommitFailure{base}
				}
				res, err := authorization.EvaluateAccessSnapshot(ctx, claims, domain.EvaluateAccessInput{ApplicationCode: "sales", PermissionCode: "sales.order.read"}, provider, h.now())
				if !errors.Is(err, want) || !reflect.DeepEqual(res, domain.AccessDecision{}) {
					t.Fatal(res, err, want)
				}
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("primary lease not returned")
			}
			var one int
			if err := h.f.DB.QueryRowContext(h.ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
				t.Fatal("pool not reusable", err)
			}
		})
	}
}
