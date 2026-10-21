//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	"github.com/siti-nabila/grpc-auth/internal/features/application"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"reflect"
	"sync"
	"testing"
)

func ApplicationConcurrency() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "two backend D-D D-C D-status D-role exclusion both arrival orders no partial outcome", Run: postgresApplicationContention},
		{Name: "locked current AND authority rejects retained replay after concurrent privilege removal", Run: postgresApplicationRecheck},
		{Name: "canonical predecessor restart and uncertain commit original replay terminal cleanup", Run: postgresApplicationTerminal},
	}
}
func applicationInput() domain.ApplicationAccessInput {
	return domain.ApplicationAccessInput{UserID: 2, ApplicationCode: "sales", RoleIDs: []uint64{6}, ExpectedVersion: 1, Reason: "Concurrent access review", Key: "same-key"}
}

func postgresApplicationContention(t *testing.T) {
	for _, pair := range [][2]string{{"grant", "grant"}, {"grant", "revoke"}, {"revoke", "grant"}, {"grant", "assignment"}, {"assignment", "grant"}, {"revoke", "status"}, {"status", "revoke"}, {"grant", "role"}, {"role", "grant"}} {
		t.Run(pair[0]+"/"+pair[1], func(t *testing.T) {
			h := newApplicationHarness(t)
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
			var held domain.Transaction
			run := func(kind string, hold bool) (*domain.SafeCommandResult, error) {
				switch kind {
				case "grant", "revoke":
					method := applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName
					if kind == "revoke" {
						method = applicationv1.ApplicationService_RevokeApplicationAccess_FullMethodName
					}
					actor := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
					d := applicationDeps(h)
					if hold {
						d.Bind = func(tx domain.Transaction) application.Repositories {
							held = tx
							r := application.BindRepositories(tx)
							r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
							return r
						}
					}
					input := applicationInput()
					s := applicationService(t, d)
					if kind == "revoke" {
						input.RoleIDs = nil
						return s.RevokeApplicationAccess(actor, input)
					}
					return s.GrantApplicationAccess(actor, input)
				case "assignment":
					actor := managementPrincipal(t, h.managementHarness, h.admin.Token, userv1.UserService_GrantUserRoles_FullMethodName)
					d := assignmentDeps(h)
					if hold {
						d.Bind = func(tx domain.Transaction) user.AssignmentRepositories {
							held = tx
							r := user.BindAssignmentRepositories(tx)
							r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
							return r
						}
					}
					return assignmentService(t, d).GrantUserRoles(actor, assignmentCommandInput(6))
				case "status":
					actor := managementPrincipal(t, h.managementHarness, h.admin.Token, userv1.UserService_ChangeUserStatus_FullMethodName)
					d := managementDeps(t, h.managementHarness)
					if hold {
						d.Bind = func(tx domain.Transaction) user.ManagementRepositories {
							held = tx
							r := user.BindManagementRepositories(tx)
							r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
							return r
						}
					}
					state := types.UserSuspended
					return managementService(t, d).ChangeUserStatus(actor, domain.ChangeUserStatusInput{UserID: 2, ExpectedVersion: 1, Status: &state, Reason: "Concurrent status review", Key: "status-key"})
				case "role":
					actor := managementPrincipal(t, h.managementHarness, h.admin.Token, rolev1.RoleService_ChangeRoleStatus_FullMethodName)
					d := roleDeps(h)
					if hold {
						d.Bind = func(tx domain.Transaction) role.ManagementRepositories {
							held = tx
							r := role.BindManagementRepositories(tx)
							r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
							return r
						}
					}
					state := types.RoleDisabled
					return roleService(t, d).ChangeRoleStatus(actor, domain.ChangeRoleStatusInput{RoleID: 6, ExpectedVersion: 1, Status: &state, Reason: "Concurrent role review", Key: "role-key"})
				}
				t.Fatal("unknown command")
				return nil, nil
			}
			go func() { r, e := run(pair[0], true); done <- managementResult{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("holder early", r.e)
			case <-h.ctx.Done():
				t.Fatal("holder deadline")
			}
			probe, e := productionProvider(h.f).Begin(h.ctx)
			if e != nil {
				t.Fatal(e)
			}
			pid, other := transactionBackend(t, held), transactionBackend(t, probe)
			probe.Rollback()
			if pid == other {
				t.Fatal("same physical backend")
			}
			snapshot := managementBusinessState(t, h.managementHarness)
			if res, e := run(pair[1], false); res != nil || !errors.Is(e, usererrors.ErrResourceBusy) && !errors.Is(e, roleerrors.ErrResourceBusy) {
				t.Fatal("contention escaped", res, e)
			}
			if snapshot != managementBusinessState(t, h.managementHarness) {
				t.Fatal("contender partial write")
			}
			unblock()
			winner := waitManagement(t, h.ctx, done)
			finished = true
			if winner.e != nil || winner.r == nil {
				t.Fatal("winner", winner.e)
			}
			assertAssignmentLockReuse(t, h)
			t.Logf("access %s/%s holder=%d contender=%d", pair[0], pair[1], pid, other)
		})
	}
}

type accessRoutingBarrier struct {
	domain.ApplicationLookup
	ready   chan struct{}
	release <-chan struct{}
	once    *sync.Once
}

func (b accessRoutingBarrier) Application(c context.Context, code string) (domain.ApplicationItem, error) {
	row, e := b.ApplicationLookup.Application(c, code)
	b.once.Do(func() {
		close(b.ready)
		select {
		case <-b.release:
		case <-c.Done():
			e = c.Err()
		}
	})
	return row, e
}
func postgresApplicationRecheck(t *testing.T) {
	for _, change := range []string{"application permission", "role permission", "generation", "session", "status"} {
		t.Run(change, func(t *testing.T) {
			h := newApplicationHarness(t)
			method := applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName
			actor := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
			input := applicationInput()
			if _, e := applicationService(t, applicationDeps(h)).GrantApplicationAccess(actor, input); e != nil {
				t.Fatal(e)
			}
			d := applicationDeps(h)
			ready, release := make(chan struct{}), make(chan struct{})
			var block, once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			done := make(chan managementResult, 1)
			finished := false
			defer func() {
				unblock()
				if !finished {
					waitManagement(t, actor, done)
				}
			}()
			pid := 0
			d.Bind = func(tx domain.Transaction) application.Repositories {
				pid = transactionBackend(t, tx)
				r := application.BindRepositories(tx)
				r.Applications = accessRoutingBarrier{r.Applications, ready, release, &block}
				return r
			}
			go func() {
				r, e := applicationService(t, d).GrantApplicationAccess(actor, input)
				done <- managementResult{r, e}
			}()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("early", r.e)
			case <-actor.Done():
				t.Fatal("deadline")
			}
			conn, e := h.f.DB.Conn(actor)
			if e != nil {
				t.Fatal(e)
			}
			other := physicalIdentity(t, actor, conn, h.f.Name)
			if pid == other || pid == 0 {
				t.Fatal("same backend")
			}
			sql := map[string]string{"application permission": "UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.application.grant')", "role permission": "UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id=(SELECT id FROM permissions WHERE code='iam.user.role.grant')", "generation": "UPDATE auth SET auth_version=auth_version+1 WHERE id=1", "session": "UPDATE sessions SET status='REVOKED',revoked_at=created_at,revoke_reason='Controlled revoke' WHERE user_id=1", "status": "UPDATE auth SET status='SUSPENDED' WHERE id=1"}[change]
			if _, e = conn.ExecContext(actor, sql); e != nil {
				t.Fatal(e)
			}
			conn.Close()
			before := managementBusinessState(t, h.managementHarness)
			unblock()
			result := waitManagement(t, actor, done)
			finished = true
			want := error(common.ErrForbidden)
			if change == "generation" || change == "session" || change == "status" {
				want = auth.ErrInvalidSession
			}
			if result.r != nil || !errors.Is(result.e, want) || before != managementBusinessState(t, h.managementHarness) {
				t.Fatal("stale replay", result.e, want)
			}
			assertAssignmentLockReuse(t, h)
		})
	}
}

func postgresApplicationTerminal(t *testing.T) {
	h := newApplicationHarness(t)
	method := applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName
	actor := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
	d := applicationDeps(h)
	trace := &managementTraceProvider{inner: d.Transactions}
	d.Transactions = trace
	// Requested role6 initially precedes discovered actor role2. A restart must
	// rollback its predecessor transaction before locking the enlarged scope.
	r, e := applicationService(t, d).GrantApplicationAccess(actor, applicationInput())
	if e != nil || r == nil || len(trace.transactions) != 2 || !trace.transactions[0].rolledBack {
		t.Fatal("predecessor", e, len(trace.transactions))
	}
	for _, tx := range trace.transactions {
		if !reflect.DeepEqual(tx.locks, featurecommon.CanonicalLockKeys(tx.locks...)) {
			t.Fatal("lock order")
		}
	}
	for _, terminal := range []string{"uncertain", "panic", "cancel"} {
		t.Run(terminal, func(t *testing.T) {
			h := newApplicationHarness(t)
			parent := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
			ctx, cancel := context.WithCancel(parent)
			defer cancel()
			d := applicationDeps(h)
			cause := errors.New("controlled response loss")
			p := &roleUncertainProvider{inner: d.Transactions, cause: cause}
			if terminal == "uncertain" {
				d.Transactions = p
			} else {
				d.EventID = func() (string, error) {
					if terminal == "panic" {
						panic("controlled")
					}
					cancel()
					return "", ctx.Err()
				}
			}
			before := managementBusinessState(t, h.managementHarness)
			out, e := applicationService(t, d).GrantApplicationAccess(ctx, applicationInput())
			want := error(cause)
			if terminal == "panic" {
				want = common.ErrInternalServerError
			}
			if terminal == "cancel" {
				want = context.Canceled
			}
			if out != nil || !errors.Is(e, want) {
				t.Fatal("terminal", out, e)
			}
			if terminal != "uncertain" && before != managementBusinessState(t, h.managementHarness) {
				t.Fatal("terminal partial commit")
			}
			if terminal == "uncertain" && p.commits != 1 {
				t.Fatal("uncertain commit retried")
			}
			committed := managementBusinessState(t, h.managementHarness)
			recovered, e := applicationService(t, applicationDeps(h)).GrantApplicationAccess(parent, applicationInput())
			if e != nil || recovered == nil || recovered.Version != 2 {
				t.Fatal("recovery", e)
			}
			if terminal == "uncertain" && committed != managementBusinessState(t, h.managementHarness) {
				t.Fatal("replay writes")
			}
			assertAssignmentLockReuse(t, h)
		})
	}
}
