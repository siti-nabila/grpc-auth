package test_scenarios

import (
	"context"
	"database/sql"
	"errors"
	commondictionary "github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{{Name: "explicit lifecycle preserves failures and prevents duplicate finalization", Run: lifecycle}, {Name: "canonical keys preserve category and numeric ordering", Run: keys}}
}

func lifecycle(t *testing.T) {
	failure := errors.New("operation failed")
	cleanup := errors.New("cleanup failed")
	for _, name := range []string{"success", "operation", "cleanup", "panic", "commit", "commit cleanup", "cancel", "denial"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx := &mocks.Transaction{Context: ctx}
			owner := common.NewTransactionOwner(ctx, tx)
			if name == "cleanup" || name == "commit cleanup" {
				tx.RollbackError = cleanup
			}
			if name == "commit" || name == "commit cleanup" {
				tx.CommitError = failure
			}
			err := func() (err error) {
				defer owner.Finish(&err)
				if name == "panic" {
					panic("private payload")
				}
				if name == "operation" || name == "cleanup" {
					return failure
				}
				if name == "cancel" {
					cancel()
				}
				if err = owner.Commit(); err != nil {
					return err
				}
				if name == "denial" {
					return commondictionary.ErrUnauthorized
				}
				return nil
			}()
			want := []string{"rollback"}
			if name == "success" || name == "denial" {
				want = []string{"commit"}
			}
			if name == "commit" || name == "commit cleanup" {
				want = []string{"commit", "rollback"}
			}
			if !reflect.DeepEqual(tx.Calls, want) {
				t.Fatalf("calls %v, want %v", tx.Calls, want)
			}
			if name == "success" && err != nil {
				t.Fatal(err)
			}
			if name == "operation" || name == "cleanup" || name == "commit" || name == "commit cleanup" {
				if !errors.Is(err, failure) {
					t.Fatal("operation cause lost")
				}
			}
			if (name == "cleanup" || name == "commit cleanup") && !errors.Is(err, cleanup) {
				t.Fatal("cleanup cause lost")
			}
			if name == "panic" && !errors.Is(err, commondictionary.ErrInternalServerError) {
				t.Fatal("panic not redacted")
			}
			if name == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost")
			}
			if name == "denial" && !errors.Is(err, commondictionary.ErrUnauthorized) {
				t.Fatal("denial lost")
			}
			if owner.Rollback() != nil || !errors.Is(owner.Commit(), sql.ErrTxDone) {
				t.Fatal("duplicate finalization")
			}
			if _, err := owner.TryLock("closed"); !errors.Is(err, sql.ErrTxDone) {
				t.Fatal("closed lock permitted")
			}
			if !reflect.DeepEqual(tx.Calls, want) {
				t.Fatal("closed owner reused adapter")
			}
		})
	}
}

func keys(t *testing.T) {
	if !reflect.DeepEqual(common.RegisterLockKeys(" USER@example.test "), common.RegisterLockKeys("user@example.test")) {
		t.Fatal("email key differs")
	}
	want := []string{"auth:user:2", "auth:user:10"}
	if !reflect.DeepEqual(common.UserLockKeys([]uint64{10, 2, 10}), want) {
		t.Fatal("numeric keys not ordered/deduplicated")
	}
	input := []string{"auth:session:00000000-0000-0000-0000-000000000001", "auth:user:10", "auth:role:3", "auth:guard:policy:global", "auth:user:2", "auth:application:1", "auth:guard:last-admin:global", "auth:idem:scope", "auth:account:digest", "auth:user:2"}
	ordered := common.CanonicalLockKeys(input...)
	reversed := slices.Clone(input)
	slices.Reverse(reversed)
	expected := []string{"auth:idem:scope", "auth:guard:last-admin:global", "auth:guard:policy:global", "auth:account:digest", "auth:user:2", "auth:user:10", "auth:role:3", "auth:application:1", "auth:session:00000000-0000-0000-0000-000000000001"}
	if !reflect.DeepEqual(ordered, expected) || !reflect.DeepEqual(common.CanonicalLockKeys(reversed...), expected) {
		t.Fatal("category order depends on input")
	}
	for _, key := range common.RegisterLockKeys("user@example.test") {
		if strings.Contains(key, "@") || strings.Contains(key, "user@example.test") {
			t.Fatal("raw email in scope")
		}
	}
	if !common.RequiresLockRestart([]string{"auth:user:10"}, []string{"auth:guard:policy:global"}) || !common.RequiresLockRestart([]string{"auth:user:10"}, []string{"auth:user:2"}) || common.RequiresLockRestart([]string{"auth:user:2"}, []string{"auth:user:2", "auth:user:10"}) || common.RequiresLockRestart(nil, expected) {
		t.Fatal("discovery restart decision incorrect")
	}
	ctx := context.Background()
	tx := &mocks.Transaction{Context: ctx}
	owner := common.NewTransactionOwner(ctx, tx)
	if err := common.AcquireLocks(owner, commondictionary.ErrForbidden, "a", "a", "b"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tx.Calls, []string{"lock:a", "lock:b"}) {
		t.Fatal(tx.Calls)
	}
}
