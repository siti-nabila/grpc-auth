package test_scenarios

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	authorizationv1 "github.com/siti-nabila/api-contracts/pb/authorization/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/authorization"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func All() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "exact self decisions follow application access and active source gates", Run: decisions},
		{Name: "identity namespace and every snapshot failure suppress decisions and release resources", Run: failures},
	}
}

const method = authorizationv1.AuthorizationService_EvaluateAccess_FullMethodName

func decisions(t *testing.T) {
	for _, kind := range []string{"allowed", "iam", "unknown app", "disabled app", "absent access", "revoked access", "unknown permission", "no sources", "one source revoked", "disabled roles", "foreign mapping", "different application IDs", "wrong namespace", "wildcard"} {
		t.Run(kind, func(t *testing.T) {
			input := domain.EvaluateAccessInput{ApplicationCode: "clm", PermissionCode: "clm.contract.read"}
			want := types.DecisionAllowed
			if kind == "iam" {
				input = domain.EvaluateAccessInput{ApplicationCode: "iam", PermissionCode: "iam.user.read"}
			}
			if kind == "unknown app" {
				input = domain.EvaluateAccessInput{ApplicationCode: "unknown", PermissionCode: "unknown.resource.read"}
			}
			ctx := shared.PermissionCallerContext(t, method, input.ApplicationCode)
			steps := shared.EvaluationSteps(input.ApplicationCode)
			last := len(steps) - 2
			switch kind {
			case "unknown app":
				steps[2].Rows = nil
				steps = append(steps[:3], mocks.SQLStep{Op: "commit"})
				want = types.DecisionPermissionMissing
			case "disabled app":
				steps[2].Rows[0][3] = "DISABLED"
				steps = append(steps[:3], mocks.SQLStep{Op: "commit"})
				want = types.DecisionApplicationInactive
			case "absent access", "revoked access":
				if kind == "absent access" {
					steps[3].Rows = nil
					input.PermissionCode = "clm.unknown.read"
				} else {
					steps[3].Rows[0][6], steps[3].Rows[0][10], steps[3].Rows[0][11] = "REVOKED", shared.ServiceTime, "Access removed"
				}
				steps = append(steps[:4], mocks.SQLStep{Op: "commit"})
				want = types.DecisionApplicationAccessRequired
			case "unknown permission":
				input.PermissionCode = "clm.unknown.read"
				want = types.DecisionPermissionMissing
			case "no sources":
				steps[last].Rows = nil
				want = types.DecisionPermissionMissing
			case "one source revoked":
				steps[last].Rows[2][6] = "REVOKED"
			case "disabled roles", "foreign mapping", "different application IDs", "wrong namespace", "wildcard":
				for _, row := range steps[last].Rows {
					if row[7] != "clm" {
						continue
					}
					switch kind {
					case "disabled roles":
						row[5] = "DISABLED"
					case "foreign mapping":
						row[1] = int64(1)
					case "different application IDs":
						row[1], row[4] = int64(3), int64(3)
					case "wrong namespace":
						row[0] = "iam.user.read"
					case "wildcard":
						row[0] = "clm.*"
					}
				}
				want = types.DecisionPermissionMissing
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			t.Cleanup(func() { _ = db.Close() })
			input.ApplicationCode = " " + input.ApplicationCode + " "
			input.PermissionCode = " " + input.PermissionCode + " "
			res, err := authorization.EvaluateAccessSnapshot(ctx, shared.AccessClaims(t), input, database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()), shared.ServiceTime)
			if err != nil || res.Reason != want || res.Allowed != (want == types.DecisionAllowed) || res.UserID != 42 || res.SessionID != shared.SessionState().SessionID || res.Version != shared.SessionState().UserVersion || !res.EvaluatedAt.Equal(shared.ServiceTime) {
				t.Fatal(res, err, want)
			}
			verify(t, mock, db)
		})
	}
}

func failures(t *testing.T) {
	for _, kind := range []string{"no caller", "namespace", "invalid input", "nil provider", "begin", "state", "missing session", "restricted", "inactive", "auth version", "app", "invalid app", "wrong app", "access", "invalid access", "wrong access app", "contributions", "commit", "rollback", "canceled", "deadline", "panic", "cancel during read"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(shared.PermissionCallerContext(t, method, "clm"))
			defer cancel()
			input := domain.EvaluateAccessInput{ApplicationCode: "clm", PermissionCode: "clm.contract.read"}
			steps := shared.EvaluationSteps("clm")
			cause, cleanup := errors.New("private controlled dependency"), errors.New("controlled cleanup failure")
			want := error(cause)
			end := -1
			switch kind {
			case "no caller":
				ctx = context.Background()
				steps = nil
				want = auth.ErrServiceAuthenticationFailed
			case "namespace":
				input = domain.EvaluateAccessInput{ApplicationCode: "iam", PermissionCode: "iam.user.read"}
				steps = nil
				want = common.ErrForbidden
			case "invalid input":
				input.PermissionCode = "clm.*"
				steps = nil
				want = common.ErrBadRequest
			case "nil provider":
				steps = nil
				want = common.ErrDependencyUnavailable
			case "begin":
				steps[0].Error = cause
				steps = steps[:1]
			case "state":
				steps[1].Error = cause
				end = 1
			case "missing session":
				steps[1].Rows = nil
				end = 1
				want = auth.ErrInvalidSession
			case "restricted":
				steps[1].Rows[0][4] = true
				end = 1
				want = common.ErrForbidden
			case "inactive":
				steps[1].Rows[0][1] = "SUSPENDED"
				end = 1
				want = auth.ErrInvalidSession
			case "auth version":
				steps[1].Rows[0][2] = int64(4)
				end = 1
				want = auth.ErrInvalidSession
			case "app", "rollback":
				steps[2].Error = cause
				end = 2
			case "invalid app":
				steps[2].Rows[0][3] = "BROKEN"
				end = 2
				want = common.ErrDependencyUnavailable
			case "wrong app":
				steps[2].Rows[0][1] = "iam"
				end = 2
				want = common.ErrDependencyUnavailable
			case "access":
				steps[3].Error = cause
				end = 3
			case "invalid access":
				steps[3].Rows[0][6] = "BROKEN"
				end = 3
				want = common.ErrDependencyUnavailable
			case "wrong access app":
				steps[3].Rows[0][2] = "iam"
				end = 3
				want = common.ErrDependencyUnavailable
			case "contributions":
				steps[4].Error = cause
				end = 4
			case "commit":
				steps[5].Error = cause
			case "canceled":
				cancel()
				steps = nil
				want = context.Canceled
			case "deadline":
				steps[0].Error = context.DeadlineExceeded
				steps = steps[:1]
				want = context.DeadlineExceeded
			case "panic":
				steps = steps[:1]
				end = 0
				want = common.ErrInternalServerError
			case "cancel during read":
				steps = steps[:2]
				end = 1
				want = context.Canceled
			}
			if end >= 0 {
				steps = append(steps[:end+1], mocks.SQLStep{Op: "rollback"})
				if kind == "rollback" {
					steps[len(steps)-1].Error = cleanup
				}
			}
			mock := &mocks.SQL{Context: ctx, Steps: steps}
			db := mock.Open()
			t.Cleanup(func() { _ = db.Close() })
			var provider domain.ReadSnapshotProvider = database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig())
			if kind == "nil provider" {
				provider = nil
			}
			if kind == "panic" {
				provider = featuremocks.PermissionSnapshot{Provider: provider, PanicAt: 1}
			}
			if kind == "cancel during read" {
				provider = featuremocks.PermissionSnapshot{Provider: provider, CancelAt: 2, Cancel: cancel}
			}
			res, err := authorization.EvaluateAccessSnapshot(ctx, shared.AccessClaims(t), input, provider, shared.ServiceTime)
			if !errors.Is(err, want) || !reflect.DeepEqual(res, domain.AccessDecision{}) {
				t.Fatal("partial decision or lost error", res, err, want)
			}
			if want == cause && !errors.Is(err, common.ErrDependencyUnavailable) {
				t.Fatal("dependency mapping", err)
			}
			if kind == "rollback" && !errors.Is(err, cleanup) {
				t.Fatal("cleanup cause lost", err)
			}
			if len(steps) > 1 {
				verify(t, mock, db)
			} else if err := mock.Verify(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func verify(t *testing.T, mock *mocks.SQL, db *sql.DB) {
	t.Helper()
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
	if mock.Options.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) || !mock.Options.ReadOnly || db.Stats().InUse != 0 {
		t.Fatal("snapshot isolation or cleanup changed", mock.Options, db.Stats().InUse)
	}
}
