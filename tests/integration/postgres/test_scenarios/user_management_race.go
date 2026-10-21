//go:build postgres

package test_scenarios

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	usererrors "github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/repositories/reader"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"github.com/siti-nabila/orm/orm"
)

func UserManagementConcurrency() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "two-backend management holds all scopes through commit then replays conflicts and version checks", Run: postgresManagementContention},
		{Name: "last administrator survives concurrent different-target commands and stale actor is denied", Run: postgresManagementLastAdmin},
		{Name: "management current authority rejects invalidated actor before saved replay", Run: postgresManagementRecheck},
		{Name: "GetUser authority and collections use one real primary snapshot", Run: postgresManagementSnapshot},
		{Name: "status holder serializes Login refresh and password producers then denies old credentials", Run: postgresManagementProducers},
		{Name: "management predecessor discovery restarts a fresh transaction with deduplicated actor target locks", Run: postgresManagementDiscovery},
	}
}

type managementPermissionDiscovery struct {
	inner domain.PermissionReader
	calls *int
}

func (d managementPermissionDiscovery) Permissions(ctx context.Context, id uint64) ([]domain.PermissionContribution, error) {
	rows, e := d.inner.Permissions(ctx, id)
	*d.calls++
	if *d.calls == 1 {
		for i := range rows {
			rows[i].RoleID = 9
		}
	}
	return rows, e
}

type managementLockTrace struct {
	domain.Transaction
	locks      []string
	rolledBack bool
}

func (tx *managementLockTrace) TryLock(ctx context.Context, key string) (bool, error) {
	tx.locks = append(tx.locks, key)
	return tx.Transaction.TryLock(ctx, key)
}
func (tx *managementLockTrace) Rollback() error {
	tx.rolledBack = true
	return tx.Transaction.Rollback()
}

type managementTraceProvider struct {
	inner        domain.TransactionProvider
	transactions []*managementLockTrace
}

func (p *managementTraceProvider) Begin(ctx context.Context) (domain.Transaction, error) {
	tx, e := p.inner.Begin(ctx)
	if e != nil {
		return nil, e
	}
	trace := &managementLockTrace{Transaction: tx}
	p.transactions = append(p.transactions, trace)
	return trace, nil
}
func postgresManagementDiscovery(t *testing.T) {
	h := newManagementHarness(t)
	actor := managementPrincipal(t, h, h.admin.Token, userv1.UserService_UpdateUser_FullMethodName)
	d := managementDeps(t, h)
	trace := &managementTraceProvider{inner: d.Transactions}
	d.Transactions = trace
	calls := 0
	d.Bind = func(tx domain.Transaction) user.ManagementRepositories {
		r := user.BindManagementRepositories(tx)
		r.Permissions = managementPermissionDiscovery{r.Permissions, &calls}
		return r
	}
	name := "Same actor target"
	r, e := managementService(t, d).UpdateUser(actor, domain.UpdateUserInput{UserID: 1, ExpectedVersion: 1, Name: &name, Fields: []string{"name"}, Key: "discovery"})
	if e != nil || r == nil || len(trace.transactions) != 2 || !trace.transactions[0].rolledBack || trace.transactions[0].Transaction == trace.transactions[1].Transaction {
		t.Fatal("predecessor restart", e, len(trace.transactions))
	}
	for _, tx := range trace.transactions {
		canonical := featurecommon.CanonicalLockKeys(tx.locks...)
		if len(canonical) != len(tx.locks) {
			t.Fatal("actor target lock duplicated")
		}
		for i, key := range canonical {
			if key != tx.locks[i] {
				t.Fatal("lock order changed across discovery")
			}
		}
	}
	if h.f.DB.Stats().InUse != 0 {
		t.Fatal("restart pool leak")
	}
}

func postgresManagementProducers(t *testing.T) {
	h := newManagementHarness(t)
	actor := managementPrincipal(t, h, h.admin.Token, userv1.UserService_ChangeUserStatus_FullMethodName)
	passwordActor := managementPrincipal(t, h, h.target.Token, "/auth.v1.AuthService/ChangePassword")
	d := managementDeps(t, h)
	ready, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	d.Bind = func(tx domain.Transaction) user.ManagementRepositories {
		r := user.BindManagementRepositories(tx)
		r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
		return r
	}
	svc := managementService(t, d)
	inactive := types.UserSuspended
	done := make(chan managementResult, 1)
	go func() {
		r, e := svc.ChangeUserStatus(actor, domain.ChangeUserStatusInput{UserID: 2, ExpectedVersion: 1, Status: &inactive, Reason: "Producer race", Key: "producer"})
		done <- managementResult{r, e}
	}()
	select {
	case <-ready:
	case <-actor.Done():
		t.Fatal("producer holder deadline")
	}
	loginCtx := shared.CallerContext(t, h.ctx, userv1.UserService_Login_FullMethodName)
	loginD := loginDependencies(t, loginCtx, h.f, 160)
	loginD.Now = h.now
	login := loginService(t, loginD)
	req := loginRequest()
	req.Email = "target@example.test"
	refreshCtx := refreshCaller(t, h.ctx)
	refreshD := refreshDependencies(t, refreshCtx, h.f, 160)
	refreshD.Now = h.now
	refresh := refreshService(t, refreshD)
	passwordD := passwordDependencies(passwordActor, h.f)
	passwordD.Now = h.now
	password := passwordService(t, passwordD)
	for _, call := range []func() error{
		func() error { _, e := login.Login(loginCtx, req); return e },
		func() error { _, e := refresh.Refresh(refreshCtx, h.target.RefreshToken); return e },
		func() error { _, e := password.ChangePassword(passwordActor, passwordRequest()); return e },
	} {
		if e := call(); !errors.Is(e, usererrors.ErrResourceBusy) && !errors.Is(e, auth.ErrResourceBusy) {
			t.Fatal("producer escaped status lock", e)
		}
	}
	unblock()
	if r := waitManagement(t, actor, done); r.e != nil {
		t.Fatal("status commit", r.e)
	}
	if _, e := login.Login(loginCtx, req); !errors.Is(e, auth.ErrLoginFailed) {
		t.Fatal("inactive Login", e)
	}
	if _, e := refresh.Refresh(refreshCtx, h.target.RefreshToken); !errors.Is(e, auth.ErrInvalidSession) {
		t.Fatal("old refresh", e)
	}
	if _, e := password.ChangePassword(passwordActor, passwordRequest()); !errors.Is(e, auth.ErrInvalidSession) {
		t.Fatal("old password principal", e)
	}
	var version, generation, active, events int
	e := h.f.DB.QueryRowContext(h.ctx, `SELECT version,auth_version,(SELECT count(*) FROM sessions WHERE user_id=2 AND status='ACTIVE'),(SELECT count(*) FROM outbox_events WHERE aggregate_id='2') FROM auth WHERE id=2`).Scan(&version, &generation, &active, &events)
	if e != nil || version != 2 || generation != 2 || active != 0 || events != 1 {
		t.Fatal("producer race counters/events", version, generation, active, events, e)
	}
}

func managementPrincipal(t *testing.T, h *managementHarness, token, method string) context.Context {
	t.Helper()
	ctx := shared.CallerContext(t, h.ctx, method, method)
	claims, e := loginSecurity(t, h.now).Verifier.Verify(token)
	if e != nil {
		t.Fatal("management claims", e)
	}
	cfg := database.GetORMConfig()
	cfg.EnableDebug = false
	ctx, e = authfeature.AuthenticateSession(ctx, claims, reader.NewSessionStateReader(orm.NewSqlQueryAdapter(ctx, h.f.DB, dialect.NewPostgres(), cfg)), h.now())
	if e != nil {
		t.Fatal("management principal", e)
	}
	return ctx
}
func managementDeps(t *testing.T, h *managementHarness) user.ManagementDependencies {
	t.Helper()
	f, e := featurecommon.NewFingerprinter("test-v1", map[string][]byte{"test-v1": bytes.Repeat([]byte("m"), 32)})
	if e != nil {
		t.Fatal(e)
	}
	return user.ManagementDependencies{Transactions: productionProvider(h.f), Bind: user.BindManagementRepositories, Fingerprinter: f, HashPassword: func(string) (string, error) { return shared.LoginPasswordHash(), nil }, Now: h.now, EventID: eventIDs()}
}
func managementService(t *testing.T, d user.ManagementDependencies) user.ManagementService {
	t.Helper()
	s, e := user.NewManagementService(d)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

type managementResult struct {
	r *domain.SafeCommandResult
	e error
}

func waitManagement(t *testing.T, ctx context.Context, done <-chan managementResult) managementResult {
	t.Helper()
	select {
	case r := <-done:
		return r
	case <-ctx.Done():
		t.Fatal("management worker deadline")
		return managementResult{}
	}
}

type managementOutboxBarrier struct {
	inner   domain.OutboxWriter
	ready   chan struct{}
	release <-chan struct{}
}

func (b managementOutboxBarrier) Create(r domain.EventEnvelope) error {
	if e := b.inner.Create(r); e != nil {
		return e
	}
	close(b.ready)
	<-b.release
	return nil
}

func postgresManagementContention(t *testing.T) {
	for _, command := range []string{"create", "update", "status"} {
		t.Run(command, func(t *testing.T) {
			h := newManagementHarness(t)
			method := userv1.UserService_CreateUser_FullMethodName
			if command == "update" {
				method = userv1.UserService_UpdateUser_FullMethodName
			}
			if command == "status" {
				method = userv1.UserService_ChangeUserStatus_FullMethodName
			}
			actor := managementPrincipal(t, h, h.admin.Token, method)
			d := managementDeps(t, h)
			ready, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var held domain.Transaction
			d.Bind = func(tx domain.Transaction) user.ManagementRepositories {
				held = tx
				r := user.BindManagementRepositories(tx)
				r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
				return r
			}
			first := managementService(t, d)
			second := managementService(t, managementDeps(t, h))
			name := "Concurrent target"
			status := types.UserSuspended
			create := domain.CreateUserInput{Name: name, Email: "concurrent@example.test", Password: "secret12", Status: func() *types.UserStatus { s := types.UserActive; return &s }(), Key: "same-key"}
			update := domain.UpdateUserInput{UserID: 2, ExpectedVersion: 1, Name: &name, Fields: []string{"name"}, Key: "same-key"}
			change := domain.ChangeUserStatusInput{UserID: 2, ExpectedVersion: 1, Status: &status, Reason: "Concurrent review", Key: "same-key"}
			call := func(s user.ManagementService) (*domain.SafeCommandResult, error) {
				switch command {
				case "create":
					return s.CreateUser(actor, create)
				case "update":
					return s.UpdateUser(actor, update)
				default:
					return s.ChangeUserStatus(actor, change)
				}
			}
			done := make(chan managementResult, 1)
			go func() { r, e := call(first); done <- managementResult{r, e} }()
			select {
			case <-ready:
			case <-actor.Done():
				t.Fatal("holder deadline")
			}
			probe, e := productionProvider(h.f).Begin(h.ctx)
			if e != nil {
				t.Fatal(e)
			}
			if transactionBackend(t, held) == transactionBackend(t, probe) {
				t.Fatal("same physical backend")
			}
			_ = probe.Rollback()
			if r, e := call(second); r != nil || !errors.Is(e, usererrors.ErrResourceBusy) {
				t.Fatal("holder did not exclude contender", e)
			}
			unblock()
			firstResult := waitManagement(t, actor, done)
			if firstResult.e != nil || firstResult.r == nil {
				t.Fatal("holder commit", firstResult.e)
			}
			if r, e := call(second); e != nil || r == nil || r.Version != firstResult.r.Version {
				t.Fatal("postcommit replay", e)
			}
			switch command {
			case "create":
				create.Password = "different-password"
				if _, e := call(second); !errors.Is(e, usererrors.ErrIdempotencyConflict) {
					t.Fatal("password conflict", e)
				}
				create.Key = "different-key"
				if _, e := call(second); !errors.Is(e, auth.ErrDataExists) {
					t.Fatal("normalized uniqueness", e)
				}
			case "update":
				update.Key = "different-key"
				if _, e := call(second); !errors.Is(e, usererrors.ErrVersionConflict) {
					t.Fatal("concurrent stale update", e)
				}
			case "status":
				change.Key = "different-key"
				if _, e := call(second); !errors.Is(e, usererrors.ErrVersionConflict) {
					t.Fatal("concurrent stale status", e)
				}
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("contention cleanup")
			}
		})
	}
}

func postgresManagementLastAdmin(t *testing.T) {
	h := newManagementHarness(t)
	// Both administrators have a shared non-admin management actor. Removing one
	// must serialize with a request for the other administrator's distinct ID.
	_, e := h.f.DB.ExecContext(h.ctx, `INSERT INTO user_role(user_id,role_id) VALUES(2,2);INSERT INTO auth(email,password,status) SELECT 'manager@example.test',password,'ACTIVE' FROM auth WHERE id=1;INSERT INTO profile(user_id,name) VALUES(3,'Manager');INSERT INTO role(role_name,role_code,application_id,code) VALUES('Manager',3,1,'iam.manager');INSERT INTO role_permissions(role_id,permission_id) SELECT 3,id FROM permissions;INSERT INTO user_role(user_id,role_id) VALUES(3,3)`)
	if e != nil {
		t.Fatal(e)
	}
	manager := h.login(t, "manager@example.test", "secret12")
	actor := managementPrincipal(t, h, manager.Token, userv1.UserService_ChangeUserStatus_FullMethodName)
	d := managementDeps(t, h)
	ready, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	d.Bind = func(tx domain.Transaction) user.ManagementRepositories {
		r := user.BindManagementRepositories(tx)
		r.Outbox = managementOutboxBarrier{r.Outbox, ready, release}
		return r
	}
	first, second := managementService(t, d), managementService(t, managementDeps(t, h))
	status := types.UserSuspended
	a := domain.ChangeUserStatusInput{UserID: 1, ExpectedVersion: 1, Status: &status, Reason: "Admin transfer", Key: "first"}
	b := a
	b.UserID = 2
	b.Key = "second"
	done := make(chan managementResult, 1)
	go func() { r, e := first.ChangeUserStatus(actor, a); done <- managementResult{r, e} }()
	select {
	case <-ready:
	case <-actor.Done():
		t.Fatal("last admin holder deadline")
	}
	if _, e = second.ChangeUserStatus(actor, b); !errors.Is(e, usererrors.ErrResourceBusy) {
		t.Fatal("global last-admin exclusion", e)
	}
	unblock()
	if r := waitManagement(t, actor, done); r.e != nil {
		t.Fatal(r.e)
	}
	if _, e = second.ChangeUserStatus(actor, b); !errors.Is(e, usererrors.ErrLastAdminRequired) {
		t.Fatal("sole remaining administrator removed", e)
	}
	var count int
	if e = h.f.DB.QueryRowContext(h.ctx, `SELECT count(DISTINCT a.id) FROM auth a JOIN user_role ur ON ur.user_id=a.id JOIN role r ON r.id=ur.role_id WHERE a.status='ACTIVE' AND ur.status='ACTIVE' AND r.code='iam.platform_admin'`).Scan(&count); e != nil || count != 1 {
		t.Fatal("last admin invariant", count, e)
	}
	// The original admin's token and saved results no longer authorize commands.
	if _, e = h.users.ChangeUserStatus(h.actor("stale-actor"), &userv1.ChangeUserStatusRequest{UserId: 2, ExpectedUserVersion: managementVersion(1), Status: managementStatus("SUSPENDED"), Reason: "Denied stale actor"}); e == nil {
		t.Fatal("invalid actor permitted")
	}
}

func postgresManagementRecheck(t *testing.T) {
	for _, change := range []string{"generation", "permission", "session"} {
		t.Run(change, func(t *testing.T) {
			h := newManagementHarness(t)
			method := userv1.UserService_UpdateUser_FullMethodName
			actor := managementPrincipal(t, h, h.admin.Token, method)
			name := "Replay target"
			req := domain.UpdateUserInput{UserID: 2, ExpectedVersion: 1, Name: &name, Fields: []string{"name"}, Key: "saved"}
			if _, e := managementService(t, managementDeps(t, h)).UpdateUser(actor, req); e != nil {
				t.Fatal(e)
			}
			d := managementDeps(t, h)
			ready, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			d.Transactions = profileBeginBarrier{provider: d.Transactions, ready: ready, release: release}
			done := make(chan managementResult, 1)
			svc := managementService(t, d)
			go func() { r, e := svc.UpdateUser(actor, req); done <- managementResult{r, e} }()
			select {
			case <-ready:
			case <-actor.Done():
				t.Fatal("delayed actor deadline")
			}
			query := "UPDATE auth SET auth_version=auth_version+1 WHERE id=1"
			if change == "permission" {
				query = "UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2"
			}
			if change == "session" {
				query = "UPDATE sessions SET status='REVOKED',revoked_at=created_at,revoke_reason='Fixture revocation' WHERE user_id=1"
			}
			if _, e := h.f.DB.ExecContext(h.ctx, query); e != nil {
				t.Fatal(e)
			}
			unblock()
			r := waitManagement(t, actor, done)
			want := error(auth.ErrInvalidSession)
			if change == "permission" {
				want = common.ErrForbidden
			}
			if r.r != nil || !errors.Is(r.e, want) {
				t.Fatal("replay bypassed current authority", r.e)
			}
			if h.f.DB.Stats().InUse != 0 {
				t.Fatal("delayed actor cleanup")
			}
		})
	}
}

func postgresManagementSnapshot(t *testing.T) {
	h := newManagementHarness(t)
	actor := managementPrincipal(t, h, h.admin.Token, userv1.UserService_GetUser_FullMethodName)
	security := loginSecurity(t, h.now)
	claims, e := security.Verifier.Verify(h.admin.Token)
	if e != nil {
		t.Fatal(e)
	}
	ready, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	provider := database.NewTransactionProvider(h.f.DB, dialect.NewPostgres(), database.GetORMConfig())
	barrier := &snapshotBarrier{provider: provider, ready: ready, release: release}
	type result struct {
		r domain.UserDetail
		e error
	}
	done := make(chan result, 1)
	go func() { r, e := user.ReadUserDetailSnapshot(actor, claims, 2, barrier, h.now()); done <- result{r, e} }()
	select {
	case <-ready:
	case <-actor.Done():
		t.Fatal("snapshot deadline")
	}
	if _, e = h.f.DB.ExecContext(h.ctx, `UPDATE role_permissions SET deleted_at=CURRENT_TIMESTAMP WHERE role_id=2;UPDATE profile SET name='New committed name' WHERE user_id=2;UPDATE user_application_access SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP,revoke_reason='Controlled fixture revocation' WHERE user_id=2`); e != nil {
		t.Fatal(e)
	}
	unblock()
	var r result
	select {
	case r = <-done:
	case <-actor.Done():
		t.Fatal("snapshot worker deadline")
	}
	if r.e != nil || r.r.Name != "Target user" || len(r.r.ApplicationAccess) != 1 {
		t.Fatal("mixed authority/data snapshots", r.r, r.e)
	}
	if _, e = user.ReadUserDetailSnapshot(actor, claims, 2, provider, h.now()); !errors.Is(e, common.ErrForbidden) {
		t.Fatal("fresh snapshot missed permission revocation", e)
	}
}
