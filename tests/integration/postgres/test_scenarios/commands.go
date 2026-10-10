//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/user"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	authmocks "github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	profilemocks "github.com/siti-nabila/grpc-auth/tests/features/profile/mocks"
	"github.com/siti-nabila/grpc-auth/tests/integration/postgres/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"testing"
)

func ExistingCommands() []testutils.Scenario {
	return []testutils.Scenario{{Name: "same canonical registration overlaps on distinct backends and returns token only after commit", Run: concurrentRegister}, {Name: "Register partial failures roll back all real repositories", Run: registerRollback}, {Name: "profile same-owner contention and independent owners preserve committed identities", Run: concurrentProfile}, {Name: "profile write failure rolls back production repository state", Run: profileRollback}}
}

type observedProvider struct {
	inner domain.TransactionProvider
	pid   int
}

func (p *observedProvider) Begin(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.inner.Begin(ctx)
	if err != nil {
		return nil, err
	}
	var row struct {
		PID int `sql:"column:pid"`
	}
	if err = tx.UseModel(domain.Auth{}).Select("pg_backend_pid() AS pid").Limit(1).Scan(&row); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	p.pid = row.PID
	return tx, nil
}

func productionProvider(fixture *fixtures.Schema) domain.TransactionProvider {
	return database.NewTransactionProvider(fixture.DB, dialect.NewPostgres(), database.GetORMConfig())
}

func registerDependencies(provider domain.TransactionProvider) authfeature.RegisterDependencies {
	return authfeature.RegisterDependencies{Transactions: provider, Bind: authfeature.BindRegisterRepositories, HashPassword: func(string) (string, error) { return "synthetic-non-login-hash", nil }, GenerateToken: func(uint64) (*string, error) { token := "synthetic-token"; return &token, nil }}
}

type registerResult struct {
	token *string
	err   error
}

func concurrentRegister(t *testing.T) {
	ctx, _, fixture := dedicatedFixture(t)
	provider := productionProvider(fixture)
	a := &observedProvider{inner: provider}
	b := &observedProvider{inner: provider}
	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan registerResult, 1)
	deps := registerDependencies(a)
	deps.GenerateToken = func(id uint64) (*string, error) {
		var count int
		if err := fixture.DB.QueryRowContext(ctx, "SELECT count(*) FROM auth WHERE id=$1", id).Scan(&count); err != nil {
			return nil, err
		}
		if count != 0 {
			return nil, errors.New("registration visible before commit")
		}
		close(ready)
		select {
		case <-release:
			token := "synthetic-token"
			return &token, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	finished := false
	released := false
	defer func() {
		if !released {
			close(release)
		}
		if !finished {
			select {
			case <-done:
			case <-ctx.Done():
				t.Error("registration worker did not finish")
			}
		}
	}()
	go func() {
		token, err := authfeature.NewRegisterServiceWithDependencies(ctx, deps).Register(domain.AuthRequest{Email: " NEW@example.test ", Password: "secret12"})
		done <- registerResult{token, err}
	}()
	select {
	case <-ready:
	case result := <-done:
		finished = true
		t.Fatalf("registration failed before barrier: %v", result.err)
	case <-ctx.Done():
		t.Fatal("registration barrier timed out")
	}
	select {
	case <-done:
		finished = true
		t.Fatal("credential response escaped before commit")
	default:
	}
	request := domain.AuthRequest{Email: "new@example.test", Password: "secret12"}
	token, err := authfeature.NewRegisterServiceWithDependencies(ctx, registerDependencies(b)).Register(request)
	if token != nil || !errors.Is(err, auth.ErrResourceBusy) || a.pid == 0 || b.pid == 0 || a.pid == b.pid {
		t.Fatal("canonical contender not busy on distinct backend")
	}
	close(release)
	released = true
	select {
	case result := <-done:
		finished = true
		if result.err != nil || result.token == nil {
			t.Fatal("winner failed to commit")
		}
	case <-ctx.Done():
		t.Fatal("winner did not finish")
	}
	assertRegistrationCounts(t, ctx, fixture, "new@example.test", 1)
	token, err = authfeature.NewRegisterServiceWithDependencies(ctx, registerDependencies(provider)).Register(request)
	if token != nil || !errors.Is(err, auth.ErrDataExists) {
		t.Fatal("post-commit registration did not observe canonical duplicate")
	}
	// The database constraint protects identity even when a writer bypasses locks.
	tx, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := common.NewTransactionOwner(ctx, tx)
	defer owner.Rollback()
	err = authfeature.BindRegisterRepositories(tx).AuthWriter.Create(&domain.AuthRequest{Email: "new@example.test", Password: "synthetic"})
	if !errors.Is(err, auth.ErrDataExists) {
		t.Fatal("unique constraint did not protect bypass write")
	}
	if err = owner.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertRegistrationCounts(t, ctx, fixture, "new@example.test", 1)
}

func assertRegistrationCounts(t *testing.T, ctx context.Context, fixture *fixtures.Schema, email string, want int) {
	t.Helper()
	var accounts, profiles, assignments int
	err := fixture.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM auth WHERE email=$1),(SELECT count(*) FROM profile p JOIN auth a ON a.id=p.user_id WHERE a.email=$1),(SELECT count(*) FROM user_role ur JOIN auth a ON a.id=ur.user_id WHERE a.email=$1)`, email).Scan(&accounts, &profiles, &assignments)
	if err != nil || accounts != want || profiles != want || assignments != want {
		t.Fatalf("atomic row counts %d/%d/%d want %d", accounts, profiles, assignments, want)
	}
}

func registerRollback(t *testing.T) {
	for _, stage := range []string{"auth", "profile", "assignment", "token", "panic", "commit"} {
		t.Run(stage, func(t *testing.T) {
			ctx, _, fixture := dedicatedFixture(t)
			base := productionProvider(fixture)
			provider := domain.TransactionProvider(base)
			failure := errors.New("controlled repository failure")
			if stage == "commit" {
				provider = commitFailureProvider{inner: base, err: failure}
			}
			deps := registerDependencies(provider)
			deps.Bind = func(tx domain.Transaction) authfeature.RegisterRepositories {
				repos := authfeature.BindRegisterRepositories(tx)
				aw, pw, uw := repos.AuthWriter, repos.ProfileWriter, repos.UserRoleWriter
				repos.AuthWriter = authmocks.AuthWriter{Write: func(req *domain.AuthRequest) error {
					if err := aw.Create(req); err != nil {
						return err
					}
					if stage == "auth" {
						return failure
					}
					return nil
				}}
				repos.ProfileWriter = authmocks.ProfileWriter{Write: func(req *domain.ProfileRequest) error {
					if err := pw.Create(req); err != nil {
						return err
					}
					if stage == "panic" {
						panic("synthetic private payload")
					}
					if stage == "profile" {
						return failure
					}
					return nil
				}}
				repos.UserRoleWriter = authmocks.UserRoleWriter{Write: func(req *domain.UserRoleRequest) error {
					if err := uw.Create(req); err != nil {
						return err
					}
					if stage == "assignment" {
						return failure
					}
					return nil
				}}
				return repos
			}
			if stage == "token" {
				deps.GenerateToken = func(uint64) (*string, error) { return nil, failure }
			}
			token, err := authfeature.NewRegisterServiceWithDependencies(ctx, deps).Register(domain.AuthRequest{Email: "partial@example.test", Password: "secret12"})
			if err == nil || token != nil {
				t.Fatal("partial failure returned credential")
			}
			assertRegistrationCounts(t, ctx, fixture, "partial@example.test", 0)
			// A clean retry must obtain the same policy/account scopes and commit once.
			token, err = authfeature.NewRegisterServiceWithDependencies(ctx, registerDependencies(base)).Register(domain.AuthRequest{Email: "partial@example.test", Password: "secret12"})
			if err != nil || token == nil {
				t.Fatal("failure leaked transaction or lock")
			}
			assertRegistrationCounts(t, ctx, fixture, "partial@example.test", 1)
		})
	}
}

type commitFailureProvider struct {
	inner domain.TransactionProvider
	err   error
}

func (p commitFailureProvider) Begin(ctx context.Context) (domain.Transaction, error) {
	tx, err := p.inner.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return commitFailureTransaction{Transaction: tx, err: p.err}, nil
}

type commitFailureTransaction struct {
	domain.Transaction
	err error
}

func (t commitFailureTransaction) Commit() error { return t.err }

type profileResult struct {
	profile domain.Profile
	err     error
}

func profileContext(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, sessions.UserSessionKey, &sessions.UserSession{UserId: id})
}

func concurrentProfile(t *testing.T) {
	ctx, _, fixture := dedicatedFixture(t)
	provider := productionProvider(fixture)
	// Create a second synthetic owner using the production bound repositories.
	tx, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := common.NewTransactionOwner(ctx, tx)
	defer owner.Rollback()
	repos := authfeature.BindRegisterRepositories(tx)
	req := &domain.AuthRequest{Email: "second@example.test", Password: "synthetic"}
	if err = repos.AuthWriter.Create(req); err != nil {
		t.Fatal(err)
	}
	second := &domain.ProfileRequest{UserId: req.Id, Name: "second"}
	if err = repos.ProfileWriter.Create(second); err != nil {
		t.Fatal(err)
	}
	if err = owner.Commit(); err != nil {
		t.Fatal(err)
	}
	a := &observedProvider{inner: provider}
	b := &observedProvider{inner: provider}
	c := &observedProvider{inner: provider}
	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan profileResult, 1)
	deps := profile.Dependencies{Transactions: a, Bind: func(tx domain.Transaction) profile.Repositories {
		repos := profile.BindRepositories(tx)
		original := repos.Writer
		repos.Writer = profilemocks.Writer{Write: func(req *domain.UpdateProfileRequest) error {
			if err := original.Update(req); err != nil {
				return err
			}
			close(ready)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}}
		return repos
	}}
	finished, released := false, false
	defer func() {
		if !released {
			close(release)
		}
		if !finished {
			select {
			case <-done:
			case <-ctx.Done():
				t.Error("profile worker did not finish")
			}
		}
	}()
	go func() {
		result, err := profile.NewProfileServiceWithDependencies(profileContext(ctx, 1), deps).UpdateProfile(domain.UpdateProfileRequest{Name: "winner"})
		done <- profileResult{result, err}
	}()
	select {
	case <-ready:
	case result := <-done:
		finished = true
		t.Fatalf("profile failed before barrier %v", result.err)
	case <-ctx.Done():
		t.Fatal("profile barrier timed out")
	}
	result, err := profile.NewProfileServiceWithDependencies(profileContext(ctx, 1), profile.Dependencies{Transactions: b, Bind: profile.BindRepositories}).UpdateProfile(domain.UpdateProfileRequest{Name: "loser"})
	if !errors.Is(err, user.ErrResourceBusy) || result != (domain.Profile{}) || a.pid == b.pid {
		t.Fatal("same owner contention not isolated")
	}
	result, err = profile.NewProfileServiceWithDependencies(profileContext(ctx, req.Id), profile.Dependencies{Transactions: c, Bind: profile.BindRepositories}).UpdateProfile(domain.UpdateProfileRequest{Id: 1, Name: "independent"})
	if err != nil || result.Id != second.Id || result.UserId != req.Id || a.pid == c.pid {
		t.Fatal("independent owner serialized or redirected")
	}
	close(release)
	released = true
	select {
	case got := <-done:
		finished = true
		if got.err != nil || got.profile.Id != 1 || got.profile.UserId != 1 || got.profile.Name != "winner" {
			t.Fatal("winner committed identity mismatch")
		}
	case <-ctx.Done():
		t.Fatal("profile worker timed out")
	}
	var name string
	var version, authVersion int
	if err = fixture.DB.QueryRowContext(ctx, "SELECT p.name,a.version,a.auth_version FROM profile p JOIN auth a ON a.id=p.user_id WHERE p.user_id=1").Scan(&name, &version, &authVersion); err != nil || name != "winner" || version != 1 || authVersion != 1 {
		t.Fatal("profile changed other owner/version")
	}
	if err = fixture.DB.QueryRowContext(ctx, "SELECT name FROM profile WHERE user_id=$1", req.Id).Scan(&name); err != nil || name != "independent" {
		t.Fatal("cross-owner write detected")
	}
}

func profileRollback(t *testing.T) {
	ctx, _, fixture := dedicatedFixture(t)
	provider := productionProvider(fixture)
	failure := errors.New("controlled profile failure")
	deps := profile.Dependencies{Transactions: provider, Bind: func(tx domain.Transaction) profile.Repositories {
		repos := profile.BindRepositories(tx)
		original := repos.Writer
		repos.Writer = profilemocks.Writer{Write: func(req *domain.UpdateProfileRequest) error {
			if err := original.Update(req); err != nil {
				return err
			}
			return failure
		}}
		return repos
	}}
	result, err := profile.NewProfileServiceWithDependencies(profileContext(ctx, 1), deps).UpdateProfile(domain.UpdateProfileRequest{Name: "partial"})
	if !errors.Is(err, failure) || result != (domain.Profile{}) {
		t.Fatal("profile failure returned success")
	}
	var name string
	if err = fixture.DB.QueryRowContext(ctx, "SELECT name FROM profile WHERE user_id=1").Scan(&name); err != nil || name != "Fixture user" {
		t.Fatal("partial profile write committed")
	}
	result, err = profile.NewProfileServiceWithDependencies(profileContext(ctx, 1), profile.Dependencies{Transactions: provider, Bind: profile.BindRepositories}).UpdateProfile(domain.UpdateProfileRequest{Name: "retry"})
	if err != nil || result.Id != 1 || result.UserId != 1 || result.Name != "retry" {
		t.Fatal("profile failure leaked lock or transaction")
	}
}
