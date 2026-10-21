package test_scenarios

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/user"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/database"
	featuremocks "github.com/siti-nabila/grpc-auth/tests/features/user/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"github.com/siti-nabila/orm/dialect"
)

func EffectivePermissions() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "self and admin union preserve source ordering namespace and inactive target semantics", Run: permissionViews},
		{Name: "permission reads recheck authority and fail closed with exact snapshot cleanup", Run: permissionFailures},
	}
}

func permissionMethods() []string {
	return []string{userv1.UserService_GetMyEffectivePermissions_FullMethodName, userv1.UserService_GetUserEffectivePermissions_FullMethodName}
}

func permissionRead(t *testing.T, ctx context.Context, method, application string, target uint64, provider domain.ReadSnapshotProvider) (domain.EffectivePermissionsResult, error) {
	t.Helper()
	claims := shared.AccessClaims(t)
	if method == userv1.UserService_GetMyEffectivePermissions_FullMethodName {
		return user.ReadMyEffectivePermissionsSnapshot(ctx, claims, domain.MyEffectivePermissionsInput{ApplicationCode: application}, provider, shared.ServiceTime)
	}
	return user.ReadUserEffectivePermissionsSnapshot(ctx, claims, domain.UserEffectivePermissionsInput{UserID: target, ApplicationCode: application}, provider, shared.ServiceTime)
}

func permissionSteps(method, status string) ([]mocks.SQLStep, uint64) {
	id := uint64(42)
	steps := []mocks.SQLStep{{Op: "begin"}, shared.PermissionSessionStep()}
	if method == userv1.UserService_GetUserEffectivePermissions_FullMethodName {
		id = 80
		steps = append(steps, shared.PermissionRowsStep(42, shared.PermissionRows()))
	}
	steps = append(steps, shared.PermissionTargetStep(id, status), shared.PermissionRowsStep(id, shared.PermissionRows()), mocks.SQLStep{Op: "commit"})
	return steps, id
}

func permissionViews(t *testing.T) {
	for _, method := range permissionMethods() {
		for _, kind := range []string{"all", "iam", "business", "empty", "unknown", "iam only", "business only", "revoked source", "disabled role", "disabled app", "absent access", "revoked access", "foreign mapping", "wrong namespace", "wildcard", "PENDING", "SUSPENDED", "DEACTIVATED"} {
			if method == permissionMethods()[0] && (kind == "PENDING" || kind == "SUSPENDED" || kind == "DEACTIVATED") {
				continue
			}
			if method == permissionMethods()[1] && kind == "business only" {
				continue
			}
			t.Run(method+"/"+kind, func(t *testing.T) {
				ns := []string{"iam", "clm", "unknown"}
				if kind == "iam only" {
					ns = []string{"iam"}
				}
				if kind == "business only" {

					ns = []string{"clm"}
				}
				ctx := shared.PermissionCallerContext(t, method, ns...)
				state := "ACTIVE"
				if kind == "PENDING" || kind == "SUSPENDED" || kind == "DEACTIVATED" {
					state = kind
				}
				steps, id := permissionSteps(method, state)
				rows := shared.PermissionRows()
				filter := ""
				switch kind {
				case "iam":
					filter = " iam "
				case "business":
					filter = "clm"
				case "unknown":
					filter = "unknown"
				case "empty":
					rows = nil
				case "revoked source":
					rows[3][6], rows[5][6] = "REVOKED", "REVOKED"
				case "disabled role":
					rows[1][5], rows[2][5] = "DISABLED", "DISABLED"
				case "disabled app":
					for _, i := range []int{1, 2, 4} {
						rows[i][8] = "DISABLED"
					}
				case "absent access":
					for _, i := range []int{1, 2, 4} {
						rows[i][9] = nil
					}
				case "revoked access":
					for _, i := range []int{1, 2, 4} {
						rows[i][9] = "REVOKED"
					}
				case "foreign mapping":
					for _, i := range []int{1, 2, 4} {
						rows[i][1] = int64(1)
					}
				case "wrong namespace":
					for _, i := range []int{1, 2, 4} {
						rows[i][0] = "iam.user.create"
					}
				case "wildcard":
					for _, i := range []int{1, 2, 4} {
						rows[i][0] = "clm.*"
					}
				}
				steps[len(steps)-2] = shared.PermissionRowsStep(id, rows)
				mock := &mocks.SQL{Context: ctx, Steps: steps}
				db := mock.Open()
				t.Cleanup(func() { _ = db.Close() })
				res, err := permissionRead(t, ctx, method, filter, id, database.NewTransactionProvider(db, dialect.NewPostgres(), database.GetORMConfig()))
				if err != nil || res.UserID != id || string(res.UserStatus) != state || res.Version != 7 {
					t.Fatal(res, err)
				}
				want := []domain.EffectivePermission{
					{Code: "clm.contract.read", Roles: []domain.PermissionRole{{ID: 4, Code: "clm.author"}, {ID: 5, Code: "clm.reviewer"}}},
					{Code: "clm.contract.review", Roles: []domain.PermissionRole{{ID: 5, Code: "clm.reviewer"}}},
					{Code: "iam.user.read", Roles: []domain.PermissionRole{{ID: 2, Code: "iam.reader"}, {ID: 3, Code: "iam.auditor"}}},
				}
				switch kind {
				case "iam", "iam only", "disabled app", "absent access", "revoked access", "foreign mapping", "wrong namespace", "wildcard":
					want = want[2:]
				case "business", "business only":
					want = want[:2]
				case "empty", "unknown":
					want = []domain.EffectivePermission{}
				case "revoked source":
					want[2].Roles = want[2].Roles[1:]
				case "disabled role":
					want = []domain.EffectivePermission{{Code: "clm.contract.read", Roles: []domain.PermissionRole{{ID: 4, Code: "clm.author"}}}, want[2]}
				}
				if !reflect.DeepEqual(res.Permissions, want) {
					t.Fatal("union/source mismatch", res.Permissions, want)
				}
				assertPermissionSQL(t, mock, db)
			})
		}
	}
}

func permissionFailures(t *testing.T) {
	for _, method := range permissionMethods() {
		for _, kind := range []string{"no caller", "nil provider", "begin", "missing session", "restricted", "inactive actor", "wrong auth version", "state", "denied", "self target denied", "authority", "missing target", "deleted target", "invalid target state", "invalid target version", "target", "contributions", "commit", "rollback", "bad application", "disallowed", "no IAM", "canceled", "deadline", "panic", "cancel during read"} {
			if method == permissionMethods()[0] && (kind == "denied" || kind == "self target denied" || kind == "authority" || kind == "no IAM") {
				continue
			}
			t.Run(method+"/"+kind, func(t *testing.T) {
				ns := []string{"iam", "clm"}
				if kind == "disallowed" || kind == "no IAM" {
					ns = []string{"clm"}
				}
				ctx := shared.PermissionCallerContext(t, method, ns...)
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				steps, id := permissionSteps(method, "ACTIVE")
				if kind == "self target denied" {
					id = 42
				}
				target := len(steps) - 3
				contributions := len(steps) - 2
				cause := errors.New("controlled private permission failure")
				cleanup := errors.New("controlled rollback failure")
				want := error(cause)
				end := -1
				filter := ""
				switch kind {
				case "no caller":
					ctx = context.Background()
					steps = nil
					want = auth.ErrServiceAuthenticationFailed
				case "nil provider":
					steps = nil
					want = common.ErrDependencyUnavailable
				case "begin":
					steps[0].Error = cause
					steps = steps[:1]
				case "missing session":
					steps[1].Rows = nil
					end = 1
					want = auth.ErrInvalidSession
				case "restricted":
					steps[1].Rows[0][4] = true
					end = 1
					want = common.ErrForbidden
				case "inactive actor":
					steps[1].Rows[0][1] = "SUSPENDED"
					end = 1
					want = auth.ErrInvalidSession
				case "wrong auth version":
					steps[1].Rows[0][2] = int64(4)
					end = 1
					want = auth.ErrInvalidSession
				case "state":
					steps[1].Error = cause
					end = 1
				case "denied", "self target denied":
					steps[2].Rows = nil
					end = 2
					want = common.ErrForbidden
				case "authority":
					steps[2].Error = cause
					end = 2
				case "missing target":
					steps[target].Rows = nil
					end = target
					want = auth.ErrNotFound
				case "deleted target":
					steps[target].Rows[0][4] = shared.ServiceTime
					end = target
					want = auth.ErrNotFound
				case "invalid target state":
					steps[target].Rows[0][1] = "BROKEN"
					end = target
					want = common.ErrDependencyUnavailable
				case "invalid target version":
					steps[target].Rows[0][2] = int64(0)
					end = target
					want = common.ErrDependencyUnavailable
				case "target", "rollback":
					steps[target].Error = cause
					end = target
				case "contributions":
					steps[contributions].Error = cause
					end = contributions
				case "commit":
					steps[len(steps)-1].Error = cause
				case "bad application":
					filter = "iam.*"
					steps = nil
					want = common.ErrBadRequest
				case "disallowed":
					filter = "iam"
					steps = nil
					want = common.ErrForbidden
				case "no IAM":
					steps = nil
					want = common.ErrForbidden
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
				res, err := permissionRead(t, ctx, method, filter, id, provider)
				if !errors.Is(err, want) || !reflect.DeepEqual(res, domain.EffectivePermissionsResult{}) {
					t.Fatal("partial result or error lost", res, err, want)
				}
				if kind == "rollback" && !errors.Is(err, cleanup) {
					t.Fatal("cleanup cause lost", err)
				}
				if want == cause && !errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal("dependency mapping lost", err)
				}
				if len(steps) > 1 {
					assertPermissionSQL(t, mock, db)
				} else if err := mock.Verify(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func assertPermissionSQL(t *testing.T, mock *mocks.SQL, db *sql.DB) {
	t.Helper()
	if err := mock.Verify(); err != nil {
		t.Fatal(err)
	}
	if mock.Options.Isolation != driver.IsolationLevel(sql.LevelRepeatableRead) || !mock.Options.ReadOnly {
		t.Fatal("snapshot options changed")
	}
	if db.Stats().InUse != 0 {
		t.Fatal("snapshot lease leaked")
	}
}
