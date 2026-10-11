package test_scenarios

import (
	"context"
	"errors"
	"testing"

	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func CancellationCleanup() []testutils.Scenario {
	return []testutils.Scenario{{Name: "cancellation finalization waits for driver rollback and returns the connection lease", Run: cancellationCleanup}}
}

func cancellationCleanup(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	mock := &mocks.SQL{Steps: []mocks.SQLStep{{Op: "begin"}, {Op: "rollback", After: func() { close(entered); <-release }}, {Op: "begin"}, {Op: "commit"}}}
	db := mock.Open()
	defer db.Close()
	db.SetMaxOpenConns(1)
	provider := database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tx, err := provider.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	// This barrier proves automatic rollback has begun and still holds its driver.
	<-entered
	go func() {
		err := ctx.Err()
		owner := featurecommon.NewTransactionOwner(ctx, tx)
		owner.Finish(&err)
		finished <- err
	}()
	if db.Stats().InUse != 1 {
		t.Error("lease released while driver rollback was blocked")
	}
	close(release)
	if err = <-finished; !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	if db.Stats().InUse != 0 {
		t.Fatal("finalization returned before cleanup released lease")
	}
	// An independent transaction can reuse this same single-slot pool.
	next, err := provider.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err = next.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = mock.Verify(); err != nil {
		t.Fatal(err)
	}
}
