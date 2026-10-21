//go:build postgres

package test_scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/orm"
	"reflect"
	"sync"
	"testing"
)

func RoleConcurrency() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "four commands retain canonical locks across two physical backends and replay after commit", Run: postgresRoleContention},
		{Name: "actor invalidation commits before policy recheck and denies saved replay", Run: postgresRoleRecheck},
		{Name: "role discovery restarts a distinct transaction and preserves lock order", Run: postgresRoleDiscovery},
		{Name: "uncertain commit has no automatic retry and next keyed call recovers exact committed response", Run: postgresRoleUncertainCommit},
		{Name: "real tentative role writes rollback after panic and cancellation", Run: postgresRoleTerminal},
	}
}
func roleDeps(h *roleHarness) role.ManagementDependencies {
	return role.ManagementDependencies{Transactions: productionProvider(h.f), Bind: role.BindManagementRepositories, Now: h.now, EventID: eventIDs()}
}
func roleService(t *testing.T, d role.ManagementDependencies) role.ManagementService {
	t.Helper()
	s, e := role.NewManagementService(d)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func postgresRoleContention(t *testing.T) {
	for _, command := range []string{"create", "update", "status", "replace"} {
		t.Run(command, func(t *testing.T) {
			h := newRoleHarness(t)
			before := roleCredentialState(t, h)
			methods := map[string]string{"create": rolev1.RoleService_CreateRole_FullMethodName, "update": rolev1.RoleService_UpdateRole_FullMethodName, "status": rolev1.RoleService_ChangeRoleStatus_FullMethodName, "replace": rolev1.RoleService_ReplaceRolePermissions_FullMethodName}
			actor := managementPrincipal(t, h.managementHarness, h.admin.Token, methods[command])
			d := roleDeps(h)
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
			d.Bind = func(tx domain.Transaction) role.ManagementRepositories {
				held = tx
				r := role.BindManagementRepositories(tx)
				r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
				return r
			}
			first, second := roleService(t, d), roleService(t, roleDeps(h))
			key, name := "same-key", "Winner"
			call := func(s role.ManagementService) (*domain.SafeCommandResult, error) {
				switch command {
				case "create":
					return s.CreateRole(actor, domain.CreateRoleInput{ApplicationCode: "sales", Code: "sales.concurrent", Name: name, Key: key, PermissionCodes: []string{"sales.order.read"}})
				case "update":
					return s.UpdateRole(actor, domain.UpdateRoleInput{RoleID: 3, ExpectedVersion: 7, Name: &name, Fields: []string{"name"}, Key: key})
				case "status":
					state := types.RoleDisabled
					return s.ChangeRoleStatus(actor, domain.ChangeRoleStatusInput{RoleID: 3, ExpectedVersion: 7, Status: &state, Reason: "Concurrency review", Key: key})
				default:
					return s.ReplaceRolePermissions(actor, domain.ReplaceRolePermissionsInput{RoleID: 3, ExpectedVersion: 7, Permissions: &domain.PermissionSet{Codes: []string{"sales.order.read"}}, Reason: "Concurrency review", Key: key})
				}
			}
			go func() { r, e := call(first); done <- managementResult{r, e} }()
			select {
			case <-ready:
			case r := <-done:
				finished = true
				t.Fatal("holder before barrier", r.e)
			case <-actor.Done():
				t.Fatal("holder deadline")
			}
			probe, e := productionProvider(h.f).Begin(h.ctx)
			if e != nil {
				t.Fatal(e)
			}
			a, b := transactionBackend(t, held), transactionBackend(t, probe)
			if a == b {
				t.Fatal("single physical backend")
			}
			_ = probe.Rollback()
			for _, scoped := range []string{"same-key", "different-key"} {
				key = scoped
				if r, e := call(second); r != nil || !errors.Is(e, roleerrors.ErrResourceBusy) {
					t.Fatal("contender escaped global policy lock", e)
				}
			}
			key = "same-key"
			unblock()
			winner := waitManagement(t, actor, done)
			finished = true
			if winner.e != nil || winner.r == nil {
				t.Fatal("winner commit", winner.e)
			}
			r, e := call(second)
			if e != nil || !equalRoleResult(r, winner.r) {
				t.Fatal("safe concurrent replay", e)
			}
			name = "Changed"
			if _, e = call(second); !errors.Is(e, roleerrors.ErrIdempotencyConflict) && command != "status" && command != "replace" {
				t.Fatal("same scope fingerprint conflict", e)
			}
			key = "different-key"
			_, e = call(second)
			want := error(roleerrors.ErrVersionConflict)
			if command == "create" {
				want = roleerrors.ErrRoleCodeAlreadyExists
			}
			if !errors.Is(e, want) {
				t.Fatal("postcommit uniqueness/version race", e)
			}
			if before != roleCredentialState(t, h) {
				t.Fatal("concurrent role commands changed credentials")
			}
			var admins int
			if e = h.f.DB.QueryRowContext(h.ctx, `SELECT count(*) FROM auth a JOIN user_role ur ON ur.user_id=a.id JOIN role r ON r.id=ur.role_id WHERE a.status='ACTIVE' AND ur.status='ACTIVE' AND r.code='iam.platform_admin' AND r.status='ACTIVE'`).Scan(&admins); e != nil || admins != 1 {
				t.Fatal("admin invariant", e)
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("lock/pool cleanup")
			}
			t.Logf("role %s holder=%d contender=%d", command, a, b)
		})
	}
}
func postgresRoleRecheck(t *testing.T) {
	for _, change := range []string{"generation", "session", "permission"} {
		t.Run(change, func(t *testing.T) {
			h := newRoleHarness(t)
			method := rolev1.RoleService_CreateRole_FullMethodName
			actor := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
			input := domain.CreateRoleInput{ApplicationCode: "sales", Code: "sales.saved", Name: "Saved", Key: "saved"}
			if _, e := roleService(t, roleDeps(h)).CreateRole(actor, input); e != nil {
				t.Fatal(e)
			}
			before := roleBusinessState(t, h)
			d := roleDeps(h)
			observed := &observedProvider{inner: d.Transactions}
			ready, release := make(chan struct{}), make(chan struct{})
			d.Transactions = profileBeginBarrier{observed, ready, release}
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
			s := roleService(t, d)
			go func() { r, e := s.CreateRole(actor, input); done <- managementResult{r, e} }()
			select {
			case <-ready:
			case <-actor.Done():
				t.Fatal("actor barrier deadline")
			}
			conn, e := h.f.DB.Conn(h.ctx)
			if e != nil {
				t.Fatal(e)
			}
			defer conn.Close()
			pid := physicalIdentity(t, h.ctx, conn, h.f.Name)
			query := "UPDATE auth SET auth_version=auth_version+1 WHERE id=1"
			if change == "session" {
				query = "UPDATE sessions SET status='REVOKED',revoked_at=created_at,revoke_reason='Controlled revocation' WHERE user_id=1"
			}
			if change == "permission" {
				query = "UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2 AND permission_id IN(SELECT id FROM permissions WHERE code='iam.role.create')"
			}
			if _, e = conn.ExecContext(h.ctx, query); e != nil {
				t.Fatal(e)
			}
			conn.Close()
			afterInvalidation := roleBusinessState(t, h)
			if before == afterInvalidation {
				t.Fatal("invalidation did not commit")
			}
			unblock()
			r := waitManagement(t, actor, done)
			finished = true
			want := error(auth.ErrInvalidSession)
			if change == "permission" {
				want = common.ErrForbidden
			}
			if r.r != nil || !errors.Is(r.e, want) || observed.pid == 0 || observed.pid == pid {
				t.Fatal("stale replay authorized or same physical backend", r.e)
			}
			if afterInvalidation != roleBusinessState(t, h) {
				t.Fatal("denied replay changed business state")
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("actor recheck cleanup")
			}
		})
	}
}
func postgresRoleDiscovery(t *testing.T) {
	h := newRoleHarness(t)
	actor := managementPrincipal(t, h.managementHarness, h.admin.Token, rolev1.RoleService_UpdateRole_FullMethodName)
	d := roleDeps(h)
	trace := &managementTraceProvider{inner: d.Transactions}
	d.Transactions = trace
	calls := 0
	d.Bind = func(tx domain.Transaction) role.ManagementRepositories {
		r := role.BindManagementRepositories(tx)
		r.Permissions = managementPermissionDiscovery{r.Permissions, &calls}
		return r
	}
	name := "After restart"
	r, e := roleService(t, d).UpdateRole(actor, domain.UpdateRoleInput{RoleID: 3, ExpectedVersion: 7, Name: &name, Fields: []string{"name"}, Key: "restart"})
	if e != nil || r == nil || len(trace.transactions) != 2 {
		t.Fatal("bounded predecessor restart", len(trace.transactions), e)
	}
	for i, tx := range trace.transactions {
		if !reflect.DeepEqual(tx.locks, featurecommon.CanonicalLockKeys(tx.locks...)) {
			t.Fatal("noncanonical discovery locks")
		}
		if i < len(trace.transactions)-1 && !tx.rolledBack {
			t.Fatal("discovery did not rollback")
		}
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("discovery leak")
	}
}

type roleUncertainTransaction struct {
	domain.Transaction
	cause   error
	commits *int
}

func (tx roleUncertainTransaction) CreateWith(v any) *orm.CreateCommand {
	return tx.Transaction.(domain.InsertWriter).CreateWith(v)
}
func (tx roleUncertainTransaction) Commit() error {
	*tx.commits++
	if e := tx.Transaction.Commit(); e != nil {
		return e
	}
	return tx.cause
}

type roleUncertainProvider struct {
	inner           domain.TransactionProvider
	cause           error
	begins, commits int
}

func (p *roleUncertainProvider) Begin(c context.Context) (domain.Transaction, error) {
	p.begins++
	tx, e := p.inner.Begin(c)
	if e != nil {
		return nil, e
	}
	return roleUncertainTransaction{tx, p.cause, &p.commits}, nil
}
func postgresRoleUncertainCommit(t *testing.T) {
	h := newRoleHarness(t)
	actor := managementPrincipal(t, h.managementHarness, h.admin.Token, rolev1.RoleService_CreateRole_FullMethodName)
	cause := errors.New("synthetic response loss after actual commit")
	d := roleDeps(h)
	p := &roleUncertainProvider{inner: d.Transactions, cause: cause}
	d.Transactions = p
	input := domain.CreateRoleInput{ApplicationCode: "sales", Code: "sales.uncertain", Name: "Committed original", Key: "uncertain"}
	r, e := roleService(t, d).CreateRole(actor, input)
	if r != nil || !errors.Is(e, cause) || p.begins != 1 || p.commits != 1 {
		t.Fatal("uncertain commit retried", p.begins, p.commits, e)
	}
	var saved string
	var count int
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT safe_response::text,(SELECT count(*) FROM role WHERE code='sales.uncertain') FROM idempotency_records WHERE key='uncertain'`).Scan(&saved, &count); e != nil || count != 1 {
		t.Fatal("actual commit absent", e)
	}
	result, e := roleService(t, roleDeps(h)).CreateRole(actor, input)
	if e != nil || result == nil || result.Role.Name != "Committed original" {
		t.Fatal("uncertain retry replay", e)
	}
	if _, e = h.f.DB.ExecContext(h.ctx, "UPDATE role SET role_name='Later changed' WHERE code='sales.uncertain'"); e != nil {
		t.Fatal(e)
	}
	replay, e := roleService(t, roleDeps(h)).CreateRole(actor, input)
	if e != nil || !equalRoleResult(result, replay) {
		t.Fatal("uncertain original response lost", e)
	}
	var next string
	if e = h.f.DB.QueryRowContext(h.ctx, "SELECT safe_response::text FROM idempotency_records WHERE key='uncertain'").Scan(&next); e != nil || next != saved {
		t.Fatal("replay rewrote saved response", e)
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("uncertain pool leak")
	}
}
func postgresRoleTerminal(t *testing.T) {
	for _, command := range []string{"create", "update", "status", "replace"} {
		for _, terminal := range []string{"panic", "cancel"} {
			t.Run(command+"/"+terminal, func(t *testing.T) {
				h := newRoleHarness(t)
				if command == "replace" {
					prepareRoleMappingDiff(t, h)
				}
				before := roleBusinessState(t, h)
				method := rolev1.RoleService_CreateRole_FullMethodName
				if command == "update" {
					method = rolev1.RoleService_UpdateRole_FullMethodName
				}
				if command == "status" {
					method = rolev1.RoleService_ChangeRoleStatus_FullMethodName
				}
				if command == "replace" {
					method = rolev1.RoleService_ReplaceRolePermissions_FullMethodName
				}
				actor := managementPrincipal(t, h.managementHarness, h.admin.Token, method)
				ctx, cancel := context.WithCancel(actor)
				defer cancel()
				d := roleDeps(h)
				d.EventID = func() (string, error) {
					if terminal == "panic" {
						panic("controlled after-write panic")
					}
					cancel()
					return "", ctx.Err()
				}
				s := roleService(t, d)
				var r *domain.SafeCommandResult
				var e error
				name := "Tentative"
				switch command {
				case "create":
					r, e = s.CreateRole(ctx, domain.CreateRoleInput{ApplicationCode: "sales", Code: "sales.terminal", Name: name, Key: "terminal", PermissionCodes: []string{"sales.order.read"}})
				case "update":
					r, e = s.UpdateRole(ctx, domain.UpdateRoleInput{RoleID: 3, ExpectedVersion: 7, Name: &name, Fields: []string{"name"}, Key: "terminal"})
				case "status":
					state := types.RoleDisabled
					r, e = s.ChangeRoleStatus(ctx, domain.ChangeRoleStatusInput{RoleID: 3, ExpectedVersion: 7, Status: &state, Reason: "Terminal check", Key: "terminal"})
				default:
					r, e = s.ReplaceRolePermissions(ctx, domain.ReplaceRolePermissionsInput{RoleID: 3, ExpectedVersion: 7, Permissions: &domain.PermissionSet{Codes: []string{"sales.order.read", "sales.order.create"}}, Reason: "Terminal check", Key: "terminal"})
				}
				want := error(common.ErrInternalServerError)
				if terminal == "cancel" {
					want = context.Canceled
				}
				if r != nil || !errors.Is(e, want) {
					t.Fatal("terminal classification", e)
				}
				if before != roleBusinessState(t, h) {
					t.Fatal("tentative role writes persisted after terminal failure")
				}
				if h.f.DB.Stats().InUse != 0 {
					t.Fatal("terminal cleanup")
				}
				// Reusing the same scope after cleanup must acquire released locks and commit.
				d = roleDeps(h)
				if _, e = roleService(t, d).CreateRole(managementPrincipal(t, h.managementHarness, h.admin.Token, rolev1.RoleService_CreateRole_FullMethodName), domain.CreateRoleInput{ApplicationCode: "sales", Code: "sales.terminal", Name: "Recovered", Key: "terminal"}); e != nil {
					t.Fatal("terminal poisoned pool or locks", e)
				}
			})
		}
	}
}

// Compare the serialized safe contract; time.Location identity is not wire data.
func equalRoleResult(a, b *domain.SafeCommandResult) bool {
	x, e := json.Marshal(a)
	if e != nil {
		return false
	}
	y, e := json.Marshal(b)
	return e == nil && bytes.Equal(x, y)
}
