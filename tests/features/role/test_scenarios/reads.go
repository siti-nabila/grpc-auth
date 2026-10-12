package test_scenarios

import (
	"context"
	"errors"
	"testing"
	"time"

	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	roleerrors "github.com/siti-nabila/api-contracts/pkg/dictionary/role"
	"github.com/siti-nabila/grpc-auth/internal/features/role"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/tests/features/catalog/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
)

func Reads() []testutils.Scenario {
	return []testutils.Scenario{{Name: "role authority precedes target in one snapshot and failures return no partial data", Run: roleReads}}
}

func roleReads(t *testing.T) {
	for _, method := range []string{rolev1.RoleService_ListRoles_FullMethodName, rolev1.RoleService_GetRole_FullMethodName} {
		for _, name := range []string{"success", "denied", "restricted", "stale session", "read failure", "panic", "commit", "missing", "permission collection failure"} {
			if method == rolev1.RoleService_ListRoles_FullMethodName && (name == "missing" || name == "permission collection failure") {
				continue
			}
			t.Run(method+"/"+name, func(t *testing.T) {
				ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, context.Background(), method, method))
				s := mocks.NewSnapshot(ctx)
				s.Rows = mocks.Contribution("iam.role.read")
				target := "roles"
				s.Steps = []string{"begin", "bind", "state", "permissions", target, "commit"}
				if method == rolev1.RoleService_GetRole_FullMethodName {
					target = "role"
					s.Steps = []string{"begin", "bind", "state", "permissions", target, "role-permissions", "commit"}
				}
				cause := errors.New("controlled role failure")
				var want error
				switch name {
				case "denied":
					s.Rows = mocks.Contribution("iam.permission.read")
					s.Steps = []string{"begin", "bind", "state", "permissions", "rollback"}
					want = common.ErrForbidden
				case "restricted":
					s.State.MustChangePassword = true
					s.Steps = []string{"begin", "bind", "state", "rollback"}
					want = common.ErrForbidden
				case "stale session":
					s.State.AuthVersion++
					s.Steps = []string{"begin", "bind", "state", "rollback"}
					want = auth.ErrInvalidSession
				case "read failure", "missing", "panic":
					s.Steps = []string{"begin", "bind", "state", "permissions", target, "rollback"}
					s.Fail = target
					s.Failure = cause
					want = common.ErrDependencyUnavailable
					if name == "missing" {
						s.Failure = roleerrors.ErrRoleNotFound
						want = roleerrors.ErrRoleNotFound
					}
					if name == "panic" {
						s.Fail = ""
						s.Panic = target
						want = common.ErrInternalServerError
					}
				case "permission collection failure":
					s.Steps = []string{"begin", "bind", "state", "permissions", target, "role-permissions", "rollback"}
					s.Fail = "role-permissions"
					s.Failure = cause
					want = common.ErrDependencyUnavailable
				case "commit":
					s.Steps = append(s.Steps, "rollback")
					s.Fail = "commit"
					s.Failure = cause
					want = common.ErrDependencyUnavailable
				}
				service, err := role.NewReadService(role.ReadDependencies{Snapshots: s, Now: func() time.Time { return fixtures.ServiceTime }, Bind: func(tx domain.Transaction) role.ReadRepositories {
					if tx != s {
						t.Fatal("snapshot handle changed")
					}
					if err := s.Step(ctx, "bind"); err != nil {
						t.Fatal(err)
					}
					return role.ReadRepositories{State: s, Permissions: s, Roles: mocks.Roles{Snapshot: s}, Catalog: mocks.Catalog{Snapshot: s}}
				}})
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				if method == rolev1.RoleService_ListRoles_FullMethodName {
					page, e := service.ListRoles(ctx, domain.CatalogQuery{})
					err = e
					count = len(page.Items)
				} else {
					detail, e := service.GetRole(ctx, 7)
					err = e
					if detail.Role.ID != 0 {
						count++
					}
					if want != nil && len(detail.Permissions) > 0 {
						t.Fatal("partial permissions escaped")
					}
				}
				if !errors.Is(err, want) || want != nil && count != 0 || want == nil && count != 1 {
					t.Fatalf("role count=%d err=%v want=%v", count, err, want)
				}
				if s.Fail != "" && !errors.Is(err, s.Failure) {
					t.Fatal("cause lost")
				}
				s.Verify(t)
			})
		}
	}
}
