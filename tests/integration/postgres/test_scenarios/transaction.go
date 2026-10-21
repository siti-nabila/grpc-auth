//go:build postgres

package test_scenarios

import (
	"context"
	"errors"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/internal/features/profile"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
	"testing"
)

func TransactionOwnership() []testutils.Scenario {
	return []testutils.Scenario{{Name: "production provider owner and repositories hold locks until visible terminal outcome", Run: productionLifecycle}, {Name: "discovered predecessor restarts with fresh transaction and bounded context", Run: discoveryRestart}}
}

func discoveryRestart(t *testing.T) {
	ctx, _, fixture := dedicatedFixture(t)
	provider := productionProvider(fixture)
	a, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerA := common.NewTransactionOwner(ctx, a)
	defer ownerA.Rollback()
	pidA := transactionBackend(t, a)
	if ok, err := ownerA.TryLock(common.UserLockKey(1)); err != nil || !ok {
		t.Fatal("initial scope unavailable")
	}
	discovered := []string{"auth:guard:policy:global", common.UserLockKey(1)}
	if !common.RequiresLockRestart([]string{common.UserLockKey(1)}, discovered) {
		t.Fatal("predecessor discovery ignored")
	}
	b, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerB := common.NewTransactionOwner(ctx, b)
	defer ownerB.Rollback()
	if transactionBackend(t, b) == pidA {
		t.Fatal("discovery needs distinct backends")
	}
	if ok, err := ownerB.TryLock(common.UserLockKey(1)); err != nil || ok {
		t.Fatal("initial scope not held")
	}
	if err = ownerA.Rollback(); err != nil {
		t.Fatal(err)
	}
	if ok, err := ownerB.TryLock(common.UserLockKey(1)); err != nil || !ok {
		t.Fatal("restart did not release predecessor transaction")
	}
	// One bounded retry observes contention and performs no write.
	retry, err := provider.Begin(ctx)
	if err != nil || retry == a {
		t.Fatal("restart reused original handle")
	}
	ownerRetry := common.NewTransactionOwner(ctx, retry)
	defer ownerRetry.Rollback()
	if err = common.AcquireLocks(ownerRetry, commondictionary.ErrForbidden, discovered...); !errors.Is(err, commondictionary.ErrForbidden) {
		t.Fatal("restart did not respect concurrent owner")
	}
	if err = ownerRetry.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = ownerB.Rollback(); err != nil {
		t.Fatal(err)
	}
	// A later command starts afresh and rechecks under the complete lock set.
	next, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerNext := common.NewTransactionOwner(ctx, next)
	defer ownerNext.Rollback()
	if err = common.AcquireLocks(ownerNext, commondictionary.ErrForbidden, discovered...); err != nil {
		t.Fatal("retry cleanup leaked scope")
	}
	result, err := profile.BindSessionRepositories(next).Reader.GetByUserId(1)
	if err != nil || result.UserId != 1 || result.Name != "Fixture user" {
		t.Fatal("restart did not recheck committed state")
	}
	if err = ownerNext.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func transactionBackend(t *testing.T, tx domain.Transaction) int {
	t.Helper()
	var row struct {
		PID int `sql:"column:pid"`
	}
	if err := tx.UseModel(domain.Auth{}).Select("pg_backend_pid() AS pid").Limit(1).Scan(&row); err != nil || row.PID == 0 {
		t.Fatal("transaction physical identity unavailable")
	}
	return row.PID
}

func productionLifecycle(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "operation failure", "panic", "cancellation"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, _, fixture := dedicatedFixture(t)
			provider := database.NewTransactionProvider(fixture.DB, dialect.NewPostgres(), database.GetORMConfig())
			holderCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			a, err := provider.Begin(holderCtx)
			if err != nil {
				t.Fatal("holder begin failed")
			}
			ownerA := common.NewTransactionOwner(holderCtx, a)
			defer ownerA.Rollback()
			pidA := transactionBackend(t, a)
			if ok, err := ownerA.TryLock(common.UserLockKey(1)); err != nil || !ok {
				t.Fatal("holder lock unavailable")
			}
			b, err := provider.Begin(ctx)
			if err != nil {
				t.Fatal("contender begin failed")
			}
			ownerB := common.NewTransactionOwner(ctx, b)
			defer ownerB.Rollback()
			if transactionBackend(t, b) == pidA {
				t.Fatal("production provider reused same physical backend concurrently")
			}
			if ok, err := ownerB.TryLock(common.UserLockKey(1)); err != nil || ok {
				t.Fatal("same scope not excluded before terminal outcome")
			}
			if ok, err := ownerB.TryLock(common.UserLockKey(2)); err != nil || !ok {
				t.Fatal("independent scope blocked")
			}
			reposA := profile.BindSessionRepositories(a)
			reposB := profile.BindSessionRepositories(b)
			if err = reposA.Writer.Update(&domain.UpdateProfileRequest{Id: 1, Name: "committed name", Address: "", Phone: ""}); err != nil {
				t.Fatal("holder write failed")
			}
			before, err := reposB.Reader.GetByUserId(1)
			if err != nil || before.Name != "Fixture user" {
				t.Fatal("contender observed uncommitted write")
			}
			failure := errors.New("controlled operation failure")
			err = func() (err error) {
				defer ownerA.Finish(&err)
				switch outcome {
				case "rollback":
					return ownerA.Rollback()
				case "operation failure":
					return failure
				case "panic":
					panic("private synthetic payload")
				case "cancellation":
					cancel()
				}
				return ownerA.Commit()
			}()
			if outcome == "operation failure" && !errors.Is(err, failure) {
				t.Fatal("operation error lost")
			}
			if outcome == "panic" && !errors.Is(err, commondictionary.ErrInternalServerError) {
				t.Fatal("panic not safely handled")
			}
			if outcome == "cancellation" && !errors.Is(err, context.Canceled) {
				t.Fatal("request cancellation lost")
			}
			if (outcome == "commit" || outcome == "rollback") && err != nil {
				t.Fatal("terminal operation failed")
			}
			// B still occupies one slot. Checkout of the other slot acknowledges
			// database/sql automatic cancellation cleanup before lock recheck.
			reused, err := fixture.DB.Conn(ctx)
			if err != nil {
				t.Fatal("holder slot not reusable after cleanup")
			}
			physicalIdentity(t, ctx, reused, fixture.Name)
			if err = reused.Close(); err != nil {
				t.Fatal("pool checkout cleanup failed")
			}
			if err = ownerB.Rollback(); err != nil {
				t.Fatal("contender cleanup failed")
			}
			next, err := provider.Begin(ctx)
			if err != nil || next == a || next == b {
				t.Fatal("new request did not receive fresh transaction")
			}
			ownerNext := common.NewTransactionOwner(ctx, next)
			defer ownerNext.Rollback()
			if ok, err := ownerNext.TryLock(common.UserLockKey(1)); err != nil || !ok {
				t.Fatal("lock leaked after terminal outcome")
			}
			after, err := profile.BindSessionRepositories(next).Reader.GetByUserId(1)
			if err != nil {
				t.Fatal("committed profile read failed")
			}
			want := "Fixture user"
			if outcome == "commit" {
				want = "committed name"
			}
			if after.Name != want || after.Id != 1 || after.UserId != 1 {
				t.Fatal("terminal state or owner identity differs")
			}
			if outcome == "commit" && (after.Address != "" || after.Phone != "") {
				t.Fatal("full replacement zero values not persisted")
			}
			if err = ownerNext.Rollback(); err != nil {
				t.Fatal("new request cleanup failed")
			}
			if fixture.DB.Stats().InUse != 0 {
				t.Fatal("production owner leaked pool slot")
			}
		})
	}
}
