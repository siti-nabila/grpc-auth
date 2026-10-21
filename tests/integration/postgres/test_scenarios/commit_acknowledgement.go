//go:build postgres

package test_scenarios

import (
	"errors"
	"testing"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func CommitAcknowledgement() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "production management commit acknowledgement loss exposes no success and retry replays exactly once", Run: managementCommitAcknowledgement},
		{Name: "production reuse commit acknowledgement loss preserves terminal state without credentials or duplicate event", Run: reuseCommitAcknowledgement},
	}
}

func managementCommitAcknowledgement(t *testing.T) {
	h := newManagementHarness(t)
	ctx := managementPrincipal(t, h, h.admin.Token, userv1.UserService_UpdateUser_FullMethodName)
	metrics := telemetry.NewStore()
	ctx = featurecommon.WithFinalizationObserver(ctx, func() { metrics.AuditFailure(telemetry.FinalizationUnknown) })
	d := managementDeps(t, h)
	cause := errors.New("controlled acknowledgement loss")
	p := &mocks.CommitAcknowledgement{Inner: d.Transactions, Failure: cause, FailNext: true}
	d.Transactions = p
	svc := managementService(t, d)
	name := "Committed despite lost acknowledgement"
	input := domain.UpdateUserInput{UserID: 2, ExpectedVersion: 1, Name: &name, Fields: []string{"name"}, Key: "acknowledgement-loss"}
	result, err := svc.UpdateUser(ctx, input)
	if result != nil || !errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable) || p.Commits != 1 || p.FailNext {
		t.Fatal("commit acknowledgement loss returned success or retried automatically", err, p.Commits)
	}
	if metrics.Snapshot(shared.ServiceTime).Audit[telemetry.FinalizationUnknown] != 1 {
		t.Fatal("lost commit uncertainty observation")
	}
	var version, generation, audits, events, results int
	var stored string
	err = h.f.DB.QueryRowContext(h.ctx, `SELECT version,auth_version,
 (SELECT name FROM profile WHERE user_id=2),
 (SELECT count(*) FROM audit_logs WHERE action='USER_PROFILE_UPDATED' AND target_id='2'),
 (SELECT count(*) FROM outbox_events WHERE event_type='iam.user.profile_updated.v1' AND aggregate_id='2' AND aggregate_version=2 AND published_at IS NULL),
 (SELECT count(*) FROM idempotency_records WHERE method=$1 AND route_scope='user/2' AND key=$2)
 FROM auth WHERE id=2`, userv1.UserService_UpdateUser_FullMethodName, input.Key).Scan(&version, &generation, &stored, &audits, &events, &results)
	if err != nil || version != 2 || generation != 1 || stored != name || audits != 1 || events != 1 || results != 1 {
		t.Fatal("unknown outcome was incorrectly treated as rollback or partial commit", version, generation, audits, events, results, err)
	}
	committed := managementBusinessState(t, h)
	retry, err := svc.UpdateUser(ctx, input)
	if err != nil || retry == nil || !retry.Changed || retry.Version != 2 || retry.Name == nil || *retry.Name != name || p.Commits != 2 {
		t.Fatal("safe retry did not replay committed result before stale version", err)
	}
	if after := managementBusinessState(t, h); after != committed || h.f.DB.Stats().InUse != 0 {
		t.Fatal("retry repeated committed effects or leaked resources")
	}
}

func reuseCommitAcknowledgement(t *testing.T) {
	ctx, f := loginFixture(t, false)
	initial, err := loginService(t, loginDependencies(t, ctx, f, 0)).Login(ctx, loginRequest())
	if err != nil {
		t.Fatal(err)
	}
	ctx = refreshCaller(t, ctx)
	metrics := telemetry.NewStore()
	ctx = featurecommon.WithFinalizationObserver(ctx, func() { metrics.AuditFailure(telemetry.FinalizationUnknown) })
	next, err := refreshService(t, refreshDependencies(t, ctx, f, 80)).Refresh(ctx, initial.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	d := refreshDependencies(t, ctx, f, 160)
	cause := errors.New("controlled reuse acknowledgement loss")
	p := &mocks.CommitAcknowledgement{Inner: d.Transactions, Failure: cause, FailNext: true}
	d.Transactions = p
	svc := refreshService(t, d)
	result, err := svc.Refresh(ctx, initial.RefreshToken)
	if result != nil || !errors.Is(err, common.ErrDependencyUnavailable) || !errors.Is(err, cause) || errors.Is(err, auth.ErrRefreshTokenReused) || p.Commits != 1 {
		t.Fatal("unknown security outcome claimed a successful denial or returned credentials", err)
	}
	if metrics.Snapshot(shared.ServiceTime).Audit[telemetry.FinalizationUnknown] != 1 {
		t.Fatal("reuse commit uncertainty observation")
	}
	assertCounts := func(reuses int) {
		t.Helper()
		var active, revoked, attempts, terminal, events, version, generation int
		err := f.DB.QueryRowContext(ctx, `SELECT
 (SELECT count(*) FROM sessions WHERE status='ACTIVE'),
 (SELECT count(*) FROM refresh_tokens WHERE revoked_at IS NOT NULL),
 (SELECT count(*) FROM audit_logs WHERE action='REFRESH_REUSE_DETECTED'),
 (SELECT count(*) FROM audit_logs WHERE action='SESSION_REVOKED'),
 (SELECT count(*) FROM outbox_events WHERE event_type='iam.session.revoked.v1' AND aggregate_id=$1 AND aggregate_version=2 AND published_at IS NULL),
 version,auth_version FROM auth WHERE id=1`, initial.SessionID).Scan(&active, &revoked, &attempts, &terminal, &events, &version, &generation)
		if err != nil || active != 0 || revoked != 2 || attempts != reuses || terminal != 1 || events != 1 || version != 1 || generation != 1 {
			t.Fatal("terminal security state or event cardinality changed", active, revoked, attempts, terminal, events, version, generation, err)
		}
	}
	assertCounts(1)
	if result, err = svc.Refresh(ctx, initial.RefreshToken); result != nil || !errors.Is(err, auth.ErrRefreshTokenReused) || p.Commits != 2 {
		t.Fatal("proven reuse retry returned credentials or repeated terminal transition", err)
	}
	assertCounts(2)
	if result, err = svc.Refresh(ctx, next.RefreshToken); result != nil || !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatal("successor credential survived committed revocation", err)
	}
	if f.DB.Stats().InUse != 0 {
		t.Fatal("unknown commit outcome leaked pool resources")
	}
}
