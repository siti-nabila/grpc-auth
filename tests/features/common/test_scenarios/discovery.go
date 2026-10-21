package test_scenarios

import (
	"context"
	"errors"
	"fmt"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	featurecommon "github.com/siti-nabila/grpc-auth/internal/features/common"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"reflect"
	"testing"
)

func Discovery() []testutils.Scenario {
	return []testutils.Scenario{{Name: "bounded discovery uses one owner without lifecycle or predecessor locks", Run: discovery}}
}

func discovery(t *testing.T) {
	for _, name := range []string{"stable", "missing", "predecessor", "unstable", "busy", "lock failure", "read failure", "recheck failure", "cancel"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tx := &mocks.Transaction{Context: ctx}
			owner := featurecommon.NewTransactionOwner(ctx, tx)
			cause := errors.New("controlled discovery failure")
			busy := common.ErrForbidden
			held := []string{"auth:user:42"}
			reads := 0
			if name == "busy" {
				tx.Busy = true
			}
			if name == "lock failure" {
				tx.LockError = cause
			}
			if name == "cancel" {
				cancel()
			}
			discover := func() ([]string, error) {
				reads++
				if name == "read failure" || name == "recheck failure" && reads == 2 {
					return nil, cause
				}
				switch name {
				case "stable":
					return []string{"auth:user:42", "auth:user:42"}, nil
				case "predecessor":
					return []string{"auth:user:2"}, nil
				case "unstable":
					return []string{fmt.Sprintf("auth:session:%02d", reads)}, nil
				default:
					return []string{"auth:role:3", "auth:role:3"}, nil
				}
			}
			restart, err := featurecommon.LockDiscoveredAuthority(owner, held, discover, busy)
			calls := []string(nil)
			wantReads := 2
			switch name {
			case "stable":
			case "missing", "recheck failure":
				calls = []string{"lock:auth:role:3"}
			case "predecessor":
				wantReads = 1
				if !reflect.DeepEqual(restart, []string{"auth:user:2", "auth:user:42"}) {
					t.Fatal(restart)
				}
			case "unstable":
				wantReads = 6
				calls = []string{"lock:auth:session:01", "lock:auth:session:03", "lock:auth:session:05"}
				if !reflect.DeepEqual(restart, []string{"auth:user:42", "auth:session:01", "auth:session:03", "auth:session:05", "auth:session:06"}) {
					t.Fatal(restart)
				}
			case "busy", "lock failure":
				wantReads = 1
				calls = []string{"lock:auth:role:3"}
			case "read failure", "cancel":
				wantReads = 1
			}
			if reads != wantReads || !reflect.DeepEqual(tx.Calls, calls) || !reflect.DeepEqual(held, []string{"auth:user:42"}) {
				t.Fatal(reads, tx.Calls, held)
			}
			switch name {
			case "busy":
				if !errors.Is(err, busy) || errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal(err)
				}
			case "lock failure":
				if !errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal(err)
				}
			case "read failure", "recheck failure":
				if !errors.Is(err, cause) {
					t.Fatal(err)
				}
			case "cancel":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if name != "predecessor" && name != "unstable" && len(restart) != 0 {
				t.Fatal(restart)
			}
		})
	}
}
