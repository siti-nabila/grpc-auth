package test_scenarios

import (
	"context"
	"errors"
	"testing"
	"time"

	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/features/catalog"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/features/catalog/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Reads() []testutils.Scenario {
	return []testutils.Scenario{{Name: "catalog authority and data share snapshot with cleanup on every outcome", Run: catalogReads}}
}

func catalogReads(t *testing.T) {
	for _, method := range []string{permissionv1.PermissionService_ListPermissions_FullMethodName, applicationv1.ApplicationService_ListApplications_FullMethodName} {
		for _, name := range []string{"success", "missing caller", "missing principal", "begin", "state", "permission dependency", "denied", "restricted", "stale session", "query invalid", "read error", "panic", "commit", "cancel"} {
			t.Run(method+"/"+name, func(t *testing.T) {
				ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, context.Background(), method, method))
				if name == "missing caller" {
					ctx = fixtures.PrincipalContext(t, context.Background())
				}
				if name == "missing principal" {
					ctx = fixtures.CallerContext(t, context.Background(), method, method)
				}
				if name == "cancel" {
					c, cancel := context.WithCancel(ctx)
					cancel()
					ctx = c
				}
				s := mocks.NewSnapshot(ctx)
				s.Rows = mocks.Contribution("iam.permission.read")
				target := "catalog-permissions"
				if method == applicationv1.ApplicationService_ListApplications_FullMethodName {
					target = "applications"
				}
				s.Steps = []string{"begin", "bind", "state", "permissions", target, "commit"}
				cause := errors.New("controlled catalog failure")
				var want error
				switch name {
				case "missing caller":
					s.Steps = nil
					want = auth.ErrServiceAuthenticationFailed
				case "missing principal":
					s.Steps = nil
					want = auth.ErrInvalidSession
				case "cancel":
					s.Steps = nil
					want = context.Canceled
				case "begin":
					s.Steps = []string{"begin"}
					s.Fail = "begin"
					s.Failure = cause
					want = common.ErrDependencyUnavailable
				case "state":
					s.Steps = []string{"begin", "bind", "state", "rollback"}
					s.Fail = "state"
					s.Failure = cause
					want = common.ErrDependencyUnavailable
				case "permission dependency":
					s.Steps = []string{"begin", "bind", "state", "permissions", "rollback"}
					s.Fail = "permissions"
					s.Failure = cause
					want = common.ErrDependencyUnavailable
				case "denied":
					s.Steps = []string{"begin", "bind", "state", "permissions", "rollback"}
					s.Rows = mocks.Contribution("iam.role.read")
					want = common.ErrForbidden
				case "restricted":
					s.Steps = []string{"begin", "bind", "state", "rollback"}
					s.State.MustChangePassword = true
					want = common.ErrForbidden
				case "stale session":
					s.Steps = []string{"begin", "bind", "state", "rollback"}
					s.State.AuthVersion++
					want = auth.ErrInvalidSession
				case "query invalid":
					s.Steps = []string{"begin", "bind", "state", "permissions", "rollback"}
					want = common.ErrBadRequest
				case "read error":
					s.Steps = []string{"begin", "bind", "state", "permissions", target, "rollback"}
					s.Fail = target
					s.Failure = cause
					want = common.ErrDependencyUnavailable
				case "panic":
					s.Steps = []string{"begin", "bind", "state", "permissions", target, "rollback"}
					s.Panic = target
					want = common.ErrInternalServerError
				case "commit":
					s.Steps = append(s.Steps, "rollback")
					s.Fail = "commit"
					s.Failure = cause
					want = common.ErrDependencyUnavailable
				}
				service, err := catalog.NewService(catalog.Dependencies{Snapshots: s, Now: func() time.Time { return fixtures.ServiceTime }, Bind: func(tx domain.Transaction) catalog.Repositories {
					if tx != s {
						t.Fatal("snapshot handle changed")
					}
					if err := s.Step(ctx, "bind"); err != nil {
						t.Fatal(err)
					}
					return catalog.Repositories{State: s, Permissions: s, Catalog: mocks.Catalog{Snapshot: s}}
				}})
				if err != nil {
					t.Fatal(err)
				}
				q := domain.CatalogQuery{}
				if name == "query invalid" {
					q.Fields = []string{"password"}
				}
				count := 0
				if method == permissionv1.PermissionService_ListPermissions_FullMethodName {
					page, e := service.ListPermissions(ctx, q)
					err = e
					count = len(page.Items)
				} else {
					page, e := service.ListApplications(ctx, q)
					err = e
					count = len(page.Items)
				}
				if !errors.Is(err, want) || want != nil && count != 0 || want == nil && count != 1 {
					t.Fatalf("result count=%d err=%v want=%v", count, err, want)
				}
				if s.Fail != "" && !errors.Is(err, cause) {
					t.Fatal("dependency cause lost")
				}
				s.Verify(t)
			})
		}
	}
}
