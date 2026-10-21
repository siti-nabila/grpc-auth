//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	applicationerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/application"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"google.golang.org/grpc/codes"
	"reflect"
	"sync"
	"testing"
)

func AssignmentConcurrency() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "same pair two-backend same different key exclusion replay and current version noop", Run: postgresAssignmentContention},
		{Name: "current actor and role application eligibility change before locked recheck", Run: postgresAssignmentRecheck},
		{Name: "last-admin revoke revoke and revoke status in both arrival orders", Run: postgresAssignmentLastAdmin},
		{Name: "registered self nominal noop last-admin and absent guard safe failures", Run: postgresAssignmentAdminErrors},
		{Name: "uncertain actual commit replay and after-write cancellation panic rollback", Run: postgresAssignmentTerminal},
		{Name: "fresh transaction predecessor discovery retains canonical locks", Run: postgresAssignmentDiscovery},
	}
}
func assignmentCommandInput(role uint64) domain.UserRoleAssignmentInput {
	return domain.UserRoleAssignmentInput{UserID: 2, RoleIDs: []uint64{role}, ExpectedVersion: 1, Reason: "Concurrent access review", Key: "same-key"}
}

func postgresAssignmentContention(t *testing.T) {
	for _, command := range []string{"grant", "revoke"} {
		t.Run(command, func(t *testing.T) {
			h := newAssignmentHarness(t)
			before := assignmentCredentialState(t, h)
			method := userv1.UserService_GrantUserRoles_FullMethodName
			id := uint64(6)
			if command == "revoke" {
				method = userv1.UserService_RevokeUserRoles_FullMethodName
				id = 3
			}
			actor := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
			d := assignmentDeps(h)
			ready, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			done := make(chan managementResult, 1)
			finished := false
			defer func() {
				unblock()
				if !finished {
					waitManagement(t, actor, done)
				}
			}()
			var held domain.Transaction
			d.Bind = func(tx domain.Transaction) user.AssignmentRepositories {
				held = tx
				r := user.BindAssignmentRepositories(tx)
				r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
				return r
			}
			first, second := assignmentService(t, d), assignmentService(t, assignmentDeps(h))
			input := assignmentCommandInput(id)
			call := func(s user.AssignmentService) (*domain.SafeCommandResult, error) {
				if command == "grant" {
					return s.GrantUserRoles(actor, input)
				}
				return s.RevokeUserRoles(actor, input)
			}
			go func() { r, e := call(first); done <- managementResult{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("holder early", r.e)
			case <-actor.Done():
				t.Fatal("holder deadline")
			}
			probe, e := productionProvider(h.f).Begin(actor)
			if e != nil {
				t.Fatal(e)
			}
			a, b := transactionBackend(t, held), transactionBackend(t, probe)
			if a == b {
				t.Fatal("same backend")
			}
			probe.Rollback()
			for _, key := range []string{"same-key", "different-key"} {
				input.Key = key
				if r, e := call(second); r != nil || !errors.Is(e, usererrors.ErrResourceBusy) {
					t.Fatal("contention escaped", e)
				}
			}
			input.Key = "same-key"
			unblock()
			winner := waitManagement(t, actor, done)
			finished = true
			if winner.e != nil || winner.r == nil {
				t.Fatal(winner.e)
			}
			replay, e := call(second)
			if e != nil || !equalRoleResult(replay, winner.r) {
				t.Fatal("concurrent retained result", e)
			}
			input.Key = "different-key"
			if _, e = call(second); !errors.Is(e, usererrors.ErrVersionConflict) {
				t.Fatal("different-key stale version", e)
			}
			input.ExpectedVersion = 2
			noop, e := call(second)
			if e != nil || noop.Changed || noop.Version != 2 {
				t.Fatal("current noop", noop, e)
			}
			var pairs, events, audits int
			e = h.f.DB.QueryRowContext(actor, `SELECT (SELECT count(*) FROM user_role WHERE user_id=2 AND role_id=$1),(SELECT count(*) FROM outbox_events WHERE event_type IN ('iam.user.roles_granted.v1','iam.user.roles_revoked.v1')),(SELECT count(*) FROM audit_logs WHERE action IN ('USER_ROLES_GRANTED','USER_ROLES_REVOKED'))`, id).Scan(&pairs, &events, &audits)
			if e != nil || pairs != 1 || events != 1 || audits != 1 {
				t.Fatal("race multiplicity", pairs, events, audits, e)
			}
			if before != assignmentCredentialState(t, h) {
				t.Fatal("concurrency changed credential state")
			}
			assertAssignmentLockReuse(t, h)
			t.Logf("assignment %s holder=%d contender=%d", command, a, b)
		})
	}
}

func assertAssignmentLockReuse(t *testing.T, h *roleHarness) {
	t.Helper()
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("pool InUse after cleanup")
	}
	tx, e := productionProvider(h.f).Begin(h.ctx)
	if e != nil {
		t.Fatal(e)
	}
	ok, e := tx.TryLock(h.ctx, "auth:guard:policy:global")
	if e != nil || !ok {
		tx.Rollback()
		t.Fatal("policy lock leaked", e)
	}
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
}

type assignmentBeginBarrier struct {
	inner   domain.TransactionProvider
	ready   chan struct{}
	release <-chan struct{}
	once    sync.Once
	pid     int
}

func (p *assignmentBeginBarrier) Begin(c context.Context) (domain.Transaction, error) {
	tx, e := p.inner.Begin(c)
	if e != nil {
		return nil, e
	}
	p.once.Do(func() {
		var row struct {
			PID int `sql:"column:pid"`
		}
		e = tx.UseModel(domain.Auth{}).WithContext(c).Select("pg_backend_pid() AS pid").Limit(1).Scan(&row)
		p.pid = row.PID
		close(p.ready)
		select {
		case <-p.release:
		case <-c.Done():
			e = c.Err()
		}
	})
	if e != nil {
		tx.Rollback()
		return nil, e
	}
	return tx, nil
}

func postgresAssignmentRecheck(t *testing.T) {
	for _, change := range []string{"generation", "session", "status", "permission", "assignment", "actor role", "actor app", "target role", "target app", "target access"} {
		t.Run(change, func(t *testing.T) {
			h := newAssignmentHarness(t)
			actor := managementPrincipal(t, h.managementHarness, h.admin.Token, userv1.UserService_GrantUserRoles_FullMethodName)
			input := assignmentCommandInput(7)
			saved := true
			if change == "target role" || change == "target app" || change == "target access" {
				input.RoleIDs = []uint64{6}
				saved = false
			}
			if saved {
				if _, e := assignmentService(t, assignmentDeps(h)).GrantUserRoles(actor, input); e != nil {
					t.Fatal(e)
				}
			}
			d := assignmentDeps(h)
			ready, release := make(chan struct{}), make(chan struct{})
			barrier := &assignmentBeginBarrier{inner: d.Transactions, ready: ready, release: release}
			d.Transactions = barrier
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			done := make(chan managementResult, 1)
			finished := false
			defer func() {
				unblock()
				if !finished {
					waitManagement(t, actor, done)
				}
			}()
			go func() { r, e := assignmentService(t, d).GrantUserRoles(actor, input); done <- managementResult{r, e} }()
			select {
			case <-ready:
			case <-actor.Done():
				t.Fatal("recheck deadline")
			}
			query := map[string]string{"generation": "UPDATE auth SET auth_version=auth_version+1 WHERE id=1", "session": "UPDATE sessions SET status='REVOKED',revoked_at=created_at,revoke_reason='Controlled revocation' WHERE user_id=1", "status": "UPDATE auth SET status='SUSPENDED' WHERE id=1", "permission": "UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id IN(SELECT id FROM permissions WHERE code='iam.user.role.grant')", "assignment": "UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled revocation' WHERE user_id=1 AND role_id=2", "actor role": "UPDATE role SET status='DISABLED' WHERE id=2", "actor app": "UPDATE applications SET status='DISABLED' WHERE id=1", "target role": "UPDATE role SET status='DISABLED' WHERE id=6", "target app": "UPDATE applications SET status='DISABLED' WHERE id=2", "target access": "UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled fixture revocation' WHERE user_id=2 AND application_id=2"}[change]
			conn, e := h.f.DB.Conn(actor)
			if e != nil {
				t.Fatal(e)
			}
			other := physicalIdentity(t, actor, conn, h.f.Name)
			if other == barrier.pid || barrier.pid == 0 {
				t.Fatal("same backend recheck")
			}
			if _, e = conn.ExecContext(actor, query); e != nil {
				t.Fatal(e)
			}
			conn.Close()
			after := managementBusinessState(t, h.managementHarness)
			unblock()
			result := waitManagement(t, actor, done)
			finished = true
			want := error(common.ErrForbidden)
			if change == "generation" || change == "session" || change == "status" {
				want = auth.ErrInvalidSession
			}
			if change == "target role" {
				want = roleerrors.ErrRoleDisabled
			}
			if change == "target app" {
				want = applicationerrors.ErrApplicationInactive
			}
			if change == "target access" {
				want = applicationerrors.ErrApplicationAccessRequired
			}
			if result.r != nil || !errors.Is(result.e, want) {
				t.Fatal("locked current recheck", result.e, want)
			}
			if after != managementBusinessState(t, h.managementHarness) {
				t.Fatal("stale replay/invalid batch effects")
			}
			assertAssignmentLockReuse(t, h)
			t.Logf("assignment recheck=%s waiting=%d invalidator=%d", change, barrier.pid, other)
		})
	}
}

func assignmentManager(t *testing.T, h *roleHarness) *userv1.UserTokenResponse {
	t.Helper()
	_, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO user_role(user_id,role_id) VALUES(2,2);INSERT INTO auth(email,password,status) SELECT 'assignment-manager@example.test',password,'ACTIVE' FROM auth WHERE id=1;INSERT INTO profile(user_id,name) VALUES(3,'Assignment manager');INSERT INTO role(role_name,role_code,application_id,code) VALUES('Assignment manager',9,1,'iam.assignment_manager');INSERT INTO role_permissions(role_id,permission_id) SELECT 9,id FROM permissions WHERE application_id=1;INSERT INTO user_role(user_id,role_id) VALUES(3,9)`)
	if e != nil {
		t.Fatal(e)
	}
	return h.login(t, "assignment-manager@example.test", "secret12")
}

func postgresAssignmentLastAdmin(t *testing.T) {
	for _, pair := range []string{"revoke/revoke", "revoke/status", "status/revoke"} {
		for _, target := range []uint64{1, 2} {
			t.Run(pair+map[uint64]string{1: " target1-first", 2: " target2-first"}[target], func(t *testing.T) {
				h := newAssignmentHarness(t)
				manager := assignmentManager(t, h)
				revokeActor := managementPrincipal(t, h.managementHarness, manager.Token, userv1.UserService_RevokeUserRoles_FullMethodName)
				statusActor := managementPrincipal(t, h.managementHarness, manager.Token, userv1.UserService_ChangeUserStatus_FullMethodName)
				ready, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				unblock := func() { once.Do(func() { close(release) }) }
				done := make(chan managementResult, 1)
				finished := false
				defer func() {
					unblock()
					if !finished {
						waitManagement(t, h.ctx, done)
					}
				}()
				ad, md := assignmentDeps(h), managementDeps(t, h.managementHarness)
				var held domain.Transaction
				ad.Bind = func(tx domain.Transaction) user.AssignmentRepositories {
					held = tx
					r := user.BindAssignmentRepositories(tx)
					r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
					return r
				}
				md.Bind = func(tx domain.Transaction) user.ManagementRepositories {
					held = tx
					r := user.BindManagementRepositories(tx)
					r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
					return r
				}
				a, b := assignmentService(t, ad), managementService(t, md)
				plainA, plainB := assignmentService(t, assignmentDeps(h)), managementService(t, managementDeps(t, h.managementHarness))
				suspended := types.UserSuspended
				call := func(kind string, id uint64, key string, hold bool) (*domain.SafeCommandResult, error) {
					if kind == "revoke" {
						svc := plainA
						if hold {
							svc = a
						}
						return svc.RevokeUserRoles(revokeActor, domain.UserRoleAssignmentInput{UserID: id, RoleIDs: []uint64{2}, ExpectedVersion: 1, Reason: "Admin transfer", Key: key})
					}
					svc := plainB
					if hold {
						svc = b
					}
					return svc.ChangeUserStatus(statusActor, domain.ChangeUserStatusInput{UserID: id, ExpectedVersion: 1, Status: &suspended, Reason: "Admin transfer", Key: key})
				}
				first, second := "revoke", "revoke"
				if pair == "revoke/status" {
					second = "status"
				}
				if pair == "status/revoke" {
					first = "status"
				}
				go func() { r, e := call(first, target, "first", true); done <- managementResult{r, e} }()
				select {
				case <-ready:
				case r := <-done:
					finished = true
					t.Fatal("admin early", r.e)
				case <-h.ctx.Done():
					t.Fatal("admin deadline")
				}
				probe, e := productionProvider(h.f).Begin(h.ctx)
				if e != nil {
					t.Fatal(e)
				}
				pid, other := transactionBackend(t, held), transactionBackend(t, probe)
				probe.Rollback()
				if pid == other {
					t.Fatal("same admin backend")
				}
				if _, e = call(second, 3-target, "second", false); !errors.Is(e, usererrors.ErrResourceBusy) {
					t.Fatal("shared guard contention", e)
				}
				unblock()
				winner := waitManagement(t, h.ctx, done)
				finished = true
				if winner.e != nil {
					t.Fatal(winner.e)
				}
				if _, e = call(second, 3-target, "second", false); !errors.Is(e, usererrors.ErrLastAdminRequired) {
					t.Fatal("last admin removed", e)
				}
				var count int
				e = h.f.DB.QueryRowContext(h.ctx, `SELECT count(DISTINCT a.id) FROM auth a JOIN user_role ur ON ur.user_id=a.id JOIN role r ON r.id=ur.role_id WHERE a.status='ACTIVE' AND a.deleted_at IS NULL AND ur.status='ACTIVE' AND r.code='iam.platform_admin'`).Scan(&count)
				if e != nil || count != 1 {
					t.Fatal("eligible admins", count, e)
				}
				assertAssignmentLockReuse(t, h)
				t.Logf("last-admin %s target=%d holder=%d contender=%d", pair, target, pid, other)
			})
		}
	}
}

func postgresAssignmentAdminErrors(t *testing.T) {
	h := newAssignmentHarness(t)
	_, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO role(role_name,role_code,application_id,code) VALUES('Manager',9,1,'iam.manager');INSERT INTO role_permissions(role_id,permission_id) SELECT 9,id FROM permissions WHERE application_id=1;INSERT INTO user_role(user_id,role_id) VALUES(1,9),(2,2);UPDATE user_role SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Admin transferred' WHERE user_id=1 AND role_id=2`)
	if e != nil {
		t.Fatal(e)
	}
	before := managementBusinessState(t, h.managementHarness)
	_, e = h.users.RevokeUserRoles(h.actor("self-noop"), assignmentRevoke(1, 1, 2))
	assertManagementCatalog(t, e, codes.PermissionDenied, usererrors.ErrSelfAdminRoleRevokeForbidden.Key())
	_, e = h.users.RevokeUserRoles(h.actor("last"), assignmentRevoke(2, 1, 2))
	assertManagementCatalog(t, e, codes.FailedPrecondition, usererrors.ErrLastAdminRequired.Key())
	if before != managementBusinessState(t, h.managementHarness) {
		t.Fatal("admin denial effects")
	}
	if _, e = h.f.DB.ExecContext(h.ctx, "DELETE FROM admin_guard"); e != nil {
		t.Fatal(e)
	}
	_, e = h.users.RevokeUserRoles(h.actor("missing-guard"), assignmentRevoke(2, 1, 2))
	assertManagementCatalog(t, e, codes.Unavailable, common.ErrDependencyUnavailable.Key())
	assertAssignmentLockReuse(t, h)
}

func postgresAssignmentTerminal(t *testing.T) {
	h := newAssignmentHarness(t)
	actor := managementPrincipal(t, h.managementHarness, h.admin.Token, userv1.UserService_GrantUserRoles_FullMethodName)
	d := assignmentDeps(h)
	cause := errors.New("controlled response loss after actual commit")
	p := &roleUncertainProvider{inner: d.Transactions, cause: cause}
	d.Transactions = p
	input := assignmentCommandInput(7)
	r, e := assignmentService(t, d).GrantUserRoles(actor, input)
	if r != nil || !errors.Is(e, cause) || p.commits != 1 || p.begins != 2 {
		t.Fatal("uncertain commit or discovery retries", p.begins, p.commits, e)
	}
	before := managementBusinessState(t, h.managementHarness)
	recovered, e := assignmentService(t, assignmentDeps(h)).GrantUserRoles(actor, input)
	if e != nil || recovered == nil || recovered.Version != 2 || !reflect.DeepEqual(recovered.ChangedRoleIDs, []uint64{7}) || before != managementBusinessState(t, h.managementHarness) {
		t.Fatal("actual committed replay", e)
	}
	assertAssignmentLockReuse(t, h)
	for _, command := range []string{"grant", "revoke"} {
		for _, terminal := range []string{"panic", "cancel"} {
			t.Run(command+"/"+terminal, func(t *testing.T) {
				h := newAssignmentHarness(t)
				before := managementBusinessState(t, h.managementHarness)
				method, id := userv1.UserService_GrantUserRoles_FullMethodName, uint64(6)
				if command == "revoke" {
					method, id = userv1.UserService_RevokeUserRoles_FullMethodName, 3
				}
				parent := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
				ctx, cancel := context.WithCancel(parent)
				defer cancel()
				d := assignmentDeps(h)
				d.EventID = func() (string, error) {
					if terminal == "panic" {
						panic("controlled terminal")
					}
					cancel()
					return "", ctx.Err()
				}
				input := assignmentCommandInput(id)
				svc := assignmentService(t, d)
				var r *domain.SafeCommandResult
				var e error
				if command == "grant" {
					r, e = svc.GrantUserRoles(ctx, input)
				} else {
					r, e = svc.RevokeUserRoles(ctx, input)
				}
				want := error(common.ErrInternalServerError)
				if terminal == "cancel" {
					want = context.Canceled
				}
				if r != nil || !errors.Is(e, want) || before != managementBusinessState(t, h.managementHarness) {
					t.Fatal("terminal atomicity", r, e)
				}
				fresh := assignmentService(t, assignmentDeps(h))
				if command == "grant" {
					r, e = fresh.GrantUserRoles(parent, input)
				} else {
					r, e = fresh.RevokeUserRoles(parent, input)
				}
				if e != nil || r == nil || !r.Changed {
					t.Fatal("scope after cleanup", e)
				}
				assertAssignmentLockReuse(t, h)
			})
		}
	}
}

func (tx *managementLockTrace) CreateWith(v any) *orm.CreateCommand {
	return tx.Transaction.(domain.InsertWriter).CreateWith(v)
}
func postgresAssignmentDiscovery(t *testing.T) {
	h := newAssignmentHarness(t)
	actor := managementPrincipal(t, h.managementHarness, h.admin.Token, userv1.UserService_GrantUserRoles_FullMethodName)
	d := assignmentDeps(h)
	trace := &managementTraceProvider{inner: d.Transactions}
	d.Transactions = trace
	calls := 0
	d.Bind = func(tx domain.Transaction) user.AssignmentRepositories {
		r := user.BindAssignmentRepositories(tx)
		r.Permissions = managementPermissionDiscovery{r.Permissions, &calls}
		return r
	}
	r, e := assignmentService(t, d).GrantUserRoles(actor, assignmentCommandInput(6))
	if e != nil || r == nil || len(trace.transactions) != 2 || trace.transactions[0].Transaction == trace.transactions[1].Transaction || !trace.transactions[0].rolledBack {
		t.Fatal("fresh restart", e, len(trace.transactions))
	}
	for _, tx := range trace.transactions {
		if !reflect.DeepEqual(tx.locks, featurecommon.CanonicalLockKeys(tx.locks...)) {
			t.Fatal("noncanonical restart", tx.locks)
		}
	}
	assertAssignmentLockReuse(t, h)
}
