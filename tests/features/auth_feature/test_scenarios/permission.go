package test_scenarios

import (
	"context"
	"errors"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	authfeature "github.com/siti-nabila/grpc-auth/internal/features/auth_feature"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/mocks"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"reflect"
	"testing"
)

func CurrentPermission() []testutils.Scenario {
	return []testutils.Scenario{
		{Name: "permission returns current principal only after state and exact grant", Run: currentPermission},
		{Name: "compound authority requires both exact permissions with one state and contribution read", Run: currentPermissions},
	}
}

func currentPermissions(t *testing.T) {
	for _, name := range []string{"both", "application only", "role only", "neither", "restricted", "empty set", "state failure", "permission failure"} {
		t.Run(name, func(t *testing.T) {
			method := "/application.v1.ApplicationService/GrantApplicationAccess"
			ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, context.Background(), method, method))
			cause := errors.New("controlled authority dependency")
			calls := []string{}
			state := shared.StateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
				if got != ctx || id != fixtures.SessionID {
					t.Fatal("state context/identity")
				}
				calls = append(calls, "state")
				s := fixtures.SessionState()
				s.UserVersion = 9
				s.MustChangePassword = name == "restricted"
				if name == "state failure" {
					return s, cause
				}
				return s, nil
			}}
			grants := mocks.Permissions{Read: func(got context.Context, id uint64) ([]domain.PermissionContribution, error) {
				if got != ctx || id != 42 {
					t.Fatal("permission context/identity")
				}
				calls = append(calls, "permission")
				if name == "permission failure" {
					return nil, cause
				}
				var rows []domain.PermissionContribution
				for _, code := range []types.PermissionCode{types.PermissionIAMUserApplicationGrant, types.PermissionIAMUserRoleGrant} {
					if name == "neither" || name == "application only" && code == types.PermissionIAMUserRoleGrant || name == "role only" && code == types.PermissionIAMUserApplicationGrant {
						continue
					}
					rows = append(rows, domain.PermissionContribution{PermissionCode: string(code), PermissionApplicationID: 1, RoleID: 2, RoleCode: "iam.platform_admin", RoleApplicationID: 1, RoleStatus: "ACTIVE", AssignmentStatus: "ACTIVE", ApplicationCode: "iam", ApplicationStatus: "ACTIVE"})
				}
				return rows, nil
			}}
			required := []types.PermissionCode{types.PermissionIAMUserApplicationGrant, types.PermissionIAMUserRoleGrant}
			if name == "empty set" {
				required = nil
			}
			p, err := authfeature.AuthorizeCurrentPermissions(ctx, fixtures.AccessClaims(t), method, required, state, grants, fixtures.ServiceTime)
			want := []string{"state", "permission"}
			if name == "restricted" || name == "state failure" {
				want = want[:1]
			}
			if name == "empty set" {
				want = []string{}
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatal(calls, want)
			}
			if name == "both" {
				if err != nil || p.UserVersion() != 9 || !p.EvaluatedAt().Equal(fixtures.ServiceTime) {
					t.Fatal(p, err)
				}
				return
			}
			if p.UserID() != 0 || err == nil {
				t.Fatal(p, err)
			}
			switch name {
			case "empty set":
				if !errors.Is(err, common.ErrInternalServerError) {
					t.Fatal(err)
				}
			case "state failure", "permission failure":
				if !errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable) {
					t.Fatal(err)
				}
			default:
				if !errors.Is(err, common.ErrForbidden) {
					t.Fatal(err)
				}
			}
		})
	}
}
func currentPermission(t *testing.T) {
	for _, name := range []string{"fresh", "missing grant", "restricted", "state error", "permission error"} {
		t.Run(name, func(t *testing.T) {
			ctx := fixtures.PrincipalContext(t, fixtures.CallerContext(t, context.Background(), userv1.UserService_GetUser_FullMethodName, userv1.UserService_GetUser_FullMethodName))
			calls := []string{}
			cause := errors.New("controlled authority error")
			state := shared.StateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
				if got != ctx || id != fixtures.SessionID {
					t.Fatal("context/identity changed")
				}
				calls = append(calls, "state")
				s := fixtures.SessionState()
				s.UserVersion = 9
				s.MustChangePassword = name == "restricted"
				if name == "state error" {
					return s, cause
				}
				return s, nil
			}}
			grants := mocks.Permissions{Read: func(got context.Context, id uint64) ([]domain.PermissionContribution, error) {
				if got != ctx || id != 42 {
					t.Fatal("permission owner changed")
				}
				calls = append(calls, "permission")
				if name == "permission error" {
					return nil, cause
				}
				if name == "missing grant" {
					return nil, nil
				}
				return []domain.PermissionContribution{{PermissionCode: "iam.user.read", PermissionApplicationID: 1, RoleID: 2, RoleCode: "iam.reader", RoleApplicationID: 1, RoleStatus: "ACTIVE", AssignmentStatus: "ACTIVE", ApplicationCode: "iam", ApplicationStatus: "ACTIVE"}}, nil
			}}
			p, err := authfeature.AuthorizeCurrentPermission(ctx, fixtures.AccessClaims(t), userv1.UserService_GetUser_FullMethodName, types.PermissionIAMUserRead, state, grants, fixtures.ServiceTime)
			want := []string{"state", "permission"}
			if name == "restricted" || name == "state error" {
				want = want[:1]
			}
			if !reflect.DeepEqual(calls, want) {
				t.Fatal(calls)
			}
			if name == "fresh" {
				if err != nil || p.UserVersion() != 9 || !p.EvaluatedAt().Equal(fixtures.ServiceTime) {
					t.Fatal(p, err)
				}
				return
			}
			if p.UserID() != 0 || err == nil {
				t.Fatal("failed authority returned principal", p, err)
			}
			if name == "missing grant" || name == "restricted" {
				if !errors.Is(err, common.ErrForbidden) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, cause) || !errors.Is(err, common.ErrDependencyUnavailable) {
				t.Fatal(err)
			}
		})
	}
}
