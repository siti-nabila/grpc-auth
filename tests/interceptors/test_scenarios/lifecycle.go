package test_scenarios

import (
	"context"
	"crypto/ed25519"
	"errors"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	authv1 "github.com/siti-nabila/api-contracts/pb/auth/v1"
	permissionv1 "github.com/siti-nabila/api-contracts/pb/permission/v1"
	rolev1 "github.com/siti-nabila/api-contracts/pb/role/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/internal/serviceauth"
	"github.com/siti-nabila/grpc-auth/internal/sessions"
	jwtpkg "github.com/siti-nabila/grpc-auth/pkg/jwt"
	authmocks "github.com/siti-nabila/grpc-auth/tests/features/auth_feature/mocks"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

func Lifecycle() []testutils.Scenario {
	cases := []testutils.Scenario{{Name: "refresh applicable bucket admission is atomic and early failures use only IP", Run: refreshRate}}
	methods := []string{authv1.AuthService_RefreshSession_FullMethodName, authv1.AuthService_Logout_FullMethodName, authv1.AuthService_ChangePassword_FullMethodName, sessionv1.SessionService_ListMySessions_FullMethodName, sessionv1.SessionService_ListUserSessions_FullMethodName, sessionv1.SessionService_RevokeMySession_FullMethodName, sessionv1.SessionService_RevokeAllMySessions_FullMethodName, sessionv1.SessionService_RevokeUserSession_FullMethodName, sessionv1.SessionService_RevokeAllUserSessions_FullMethodName}
	methods = append(methods, rolePolicyMethods()...)
	methods = append(methods, userv1.UserService_ListUserRoles_FullMethodName, userv1.UserService_GrantUserRoles_FullMethodName, userv1.UserService_RevokeUserRoles_FullMethodName)
	for _, method := range methods {
		for _, kind := range []string{"success", "restricted", "missing", "duplicate", "expired", "revoked", "dependency", "namespace"} {
			cases = append(cases, testutils.Scenario{Name: method + " " + kind, Run: func(t *testing.T) {
				cfg := shared.ServiceConfig()
				cfg.Callers[0].Methods = append(cfg.Callers[0].Methods, method)
				if kind == "namespace" {
					cfg.Callers[0].Namespaces = []string{"clm"}
				}
				verifier, e := serviceauth.NewVerifier(cfg, func() time.Time { return shared.ServiceTime })
				if e != nil {
					t.Fatal(e)
				}
				ctx, e := verifier.Authenticate(serviceContext(shared.ServiceToken(1)), method)
				if e != nil {
					t.Fatal(e)
				}
				md, _ := metadata.FromIncomingContext(ctx)
				md = md.Copy()
				payload := shared.AccessPayload()
				if kind == "expired" {
					payload["iat"] = shared.ServiceTime.Add(-time.Hour).Unix()
					payload["exp"] = shared.ServiceTime.Add(-time.Minute).Unix()
				}
				md.Set("authorization", "Bearer "+shared.SignAccess(t, payload))
				if kind == "missing" {
					md.Delete("authorization")
				}
				if kind == "duplicate" {
					md.Append("authorization", "Bearer duplicate")
				}
				refresh := method == authv1.AuthService_RefreshSession_FullMethodName
				logout := method == authv1.AuthService_Logout_FullMethodName
				if refresh && kind != "duplicate" {
					md.Delete("authorization")
				}
				ctx = metadata.NewIncomingContext(ctx, md)
				count := 0
				cause := errors.New("controlled reader unavailable")
				reader := authmocks.SessionStateReader{Read: func(got context.Context, id string) (domain.SessionState, error) {
					count++
					if got != ctx || id != shared.SessionID {
						t.Fatal("context/identity lost")
					}
					s := shared.SessionState()
					if kind == "restricted" {
						s.MustChangePassword = true
					}
					if kind == "revoked" {
						s.SessionStatus = "REVOKED"
					}
					if kind == "dependency" {
						return s, cause
					}
					return s, nil
				}}
				av, e := jwtpkg.NewAccessVerifier(shared.AccessIssuer, shared.AccessKey().Public().(ed25519.PublicKey), func() time.Time { return shared.ServiceTime })
				if e != nil {
					t.Fatal(e)
				}
				boundary, e := interceptors.NewSessionInterceptor(av, reader, func() time.Time { return shared.ServiceTime })
				if e != nil {
					t.Fatal(e)
				}
				calls := 0
				var principal bool
				_, e = boundary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(next context.Context, _ any) (any, error) {
					calls++
					_, p := sessions.PrincipalFromContext(next)
					principal = p == nil
					return "called", nil
				})
				allowed := kind == "success" || kind == "restricted" && method == authv1.AuthService_ChangePassword_FullMethodName
				if refresh {
					allowed = kind != "duplicate" && kind != "namespace"
				}
				if logout {
					allowed = kind != "namespace"
				}
				if allowed {
					if e != nil || calls != 1 || principal == (refresh || logout) {
						t.Fatal("credential-mode boundary", calls, principal, e)
					}
				} else {
					want := error(auth.ErrInvalidSession)
					if kind == "namespace" || kind == "restricted" {
						want = common.ErrForbidden
					}
					if kind == "dependency" {
						want = common.ErrDependencyUnavailable
					}
					if !errors.Is(e, want) || calls != 0 {
						t.Fatal("guard bypass", calls, e, want)
					}
				}
				if (refresh || logout || kind == "namespace" || kind == "missing" || kind == "duplicate" || kind == "expired") && count != 0 {
					t.Fatal("special/invalid mode performed principal read", count)
				}
			}})
		}
	}
	return cases
}
func refreshRate(t *testing.T) {
	store, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
	boundary, admit, e := interceptors.NewRefreshAdmission(store, rateEncoder())
	if e != nil {
		t.Fatal(e)
	}
	ctx := shared.CallerContext(t, context.Background(), authv1.AuthService_RefreshSession_FullMethodName, authv1.AuthService_RefreshSession_FullMethodName)
	info := &grpc.UnaryServerInfo{FullMethod: authv1.AuthService_RefreshSession_FullMethodName}
	next := func(ctx context.Context, _ any) (any, error) {
		if e := admit(ctx, shared.SessionID); e != nil {
			return nil, e
		}
		return "allowed", nil
	}
	for n := 0; n < 2; n++ {
		if _, e = boundary(ctx, nil, info, next); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = boundary(ctx, nil, info, next); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("session burst", e)
	}
	// Rejected session attempt consumed none of the remaining eight IP tokens.
	for n := 0; n < 8; n++ {
		_, e = boundary(ctx, nil, info, func(context.Context, any) (any, error) { return nil, auth.ErrInvalidSession })
		if !errors.Is(e, auth.ErrInvalidSession) {
			t.Fatal("IP wrongly consumed", n, e)
		}
	}
	if _, e = boundary(ctx, nil, info, func(context.Context, any) (any, error) { return nil, auth.ErrInvalidSession }); status.Code(e) != codes.ResourceExhausted {
		t.Fatal("IP burst", e)
	}
	if _, e = boundary(ctx, nil, info, func(context.Context, any) (any, error) { return nil, common.ErrDependencyUnavailable }); !errors.Is(e, common.ErrDependencyUnavailable) {
		t.Fatal("lookup dependency hidden by quota", e)
	}
	if store.Counts().Entries != 2 {
		t.Fatal("claimed/unknown session bucket allocated", store.Counts())
	}
	if e = admit(ctx, shared.SessionID); !errors.Is(e, common.ErrInternalServerError) {
		t.Fatal("out-of-bound admission accepted")
	}
	if _, _, e = interceptors.NewRefreshAdmission(nil, rateEncoder()); e == nil {
		t.Fatal("nil store")
	}
}

func LifecycleRateGroups() []testutils.Scenario {
	cases := []testutils.Scenario{}
	for _, tc := range []struct {
		method string
		burst  int
	}{{authv1.AuthService_ChangePassword_FullMethodName, 2}, {sessionv1.SessionService_RevokeUserSession_FullMethodName, 5}, {sessionv1.SessionService_RevokeAllUserSessions_FullMethodName, 5}, {sessionv1.SessionService_ListMySessions_FullMethodName, 20}, {sessionv1.SessionService_ListUserSessions_FullMethodName, 20}} {
		cases = append(cases, testutils.Scenario{Name: "actor quota " + tc.method, Run: func(t *testing.T) {
			store, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
			boundary, e := interceptors.NewAuthRateInterceptor(store, []byte("test-only-secret"), rateEncoder())
			if e != nil {
				t.Fatal(e)
			}
			ctx := shared.PrincipalContext(t, shared.CallerContext(t, context.Background(), tc.method, tc.method))
			calls := 0
			next := func(context.Context, any) (any, error) { calls++; return nil, nil }
			for n := 0; n < tc.burst; n++ {
				if _, e = boundary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: tc.method}, next); e != nil {
					t.Fatal(e)
				}
			}
			if _, e = boundary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: tc.method}, next); status.Code(e) != codes.ResourceExhausted || calls != tc.burst || store.Counts().Entries != 1 {
				t.Fatal("quota route", calls, e)
			}
		}})
	}
	for _, method := range []string{authv1.AuthService_Logout_FullMethodName, sessionv1.SessionService_RevokeMySession_FullMethodName, sessionv1.SessionService_RevokeAllMySessions_FullMethodName} {
		cases = append(cases, testutils.Scenario{Name: "no invented quota " + method, Run: func(t *testing.T) {
			store, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
			boundary, e := interceptors.NewAuthRateInterceptor(store, []byte("test-only-secret"), rateEncoder())
			if e != nil {
				t.Fatal(e)
			}
			ctx := shared.PrincipalContext(t, shared.CallerContext(t, context.Background(), method, method))
			calls := 0
			for n := 0; n < 25; n++ {
				if _, e = boundary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, any) (any, error) { calls++; return nil, nil }); e != nil {
					t.Fatal(e)
				}
			}
			if calls != 25 || store.Counts().Entries != 0 {
				t.Fatal("unapproved quota allocated")
			}
		}})
	}
	return cases
}

func rolePolicyMethods() []string {
	return []string{rolev1.RoleService_ListRoles_FullMethodName, rolev1.RoleService_GetRole_FullMethodName, rolev1.RoleService_CreateRole_FullMethodName, rolev1.RoleService_UpdateRole_FullMethodName, rolev1.RoleService_ChangeRoleStatus_FullMethodName, rolev1.RoleService_ReplaceRolePermissions_FullMethodName, permissionv1.PermissionService_ListPermissions_FullMethodName, applicationv1.ApplicationService_ListApplications_FullMethodName}
}
