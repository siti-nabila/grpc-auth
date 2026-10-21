package test_scenarios

import (
	"context"
	applicationv1 "github.com/siti-nabila/api-contracts/pb/application/v1"
	sessionv1 "github.com/siti-nabila/api-contracts/pb/session/v1"
	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/ratelimit"
	shared "github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func RoleRateGroups() []testutils.Scenario {
	return []testutils.Scenario{{Name: "concurrent mixed role catalog user session requests admit only the shared actor burst", Run: roleRateGroups}}
}
func roleRateGroups(t *testing.T) {
	methods := rolePolicyMethods()
	for _, group := range []struct {
		methods []string
		burst   int
	}{
		{append([]string{userv1.UserService_GetMyEffectivePermissions_FullMethodName, userv1.UserService_GetUserEffectivePermissions_FullMethodName, userv1.UserService_ListUserApplicationAccess_FullMethodName, userv1.UserService_ListUserRoles_FullMethodName, userv1.UserService_GetUser_FullMethodName, sessionv1.SessionService_ListMySessions_FullMethodName}, methods[0], methods[1], methods[6], methods[7]), 20},
		{append([]string{applicationv1.ApplicationService_GrantApplicationAccess_FullMethodName, applicationv1.ApplicationService_RevokeApplicationAccess_FullMethodName, userv1.UserService_GrantUserRoles_FullMethodName, userv1.UserService_RevokeUserRoles_FullMethodName, userv1.UserService_UpdateUser_FullMethodName, sessionv1.SessionService_RevokeUserSession_FullMethodName}, methods[2:6]...), 5},
	} {
		store, _ := ratelimit.NewStore(func() time.Time { return shared.ServiceTime })
		boundary, e := interceptors.NewAuthRateInterceptor(store, []byte("test-only-secret"), rateEncoder())
		if e != nil {
			t.Fatal(e)
		}
		ctx := shared.PrincipalContext(t, shared.CallerContext(t, context.Background(), group.methods[0], group.methods...))
		var calls, denials atomic.Int64
		var workers sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 80; i++ {
			workers.Add(1)
			go func(i int) {
				defer workers.Done()
				<-start
				_, e := boundary(ctx, nil, &grpc.UnaryServerInfo{FullMethod: group.methods[i%len(group.methods)]}, func(context.Context, any) (any, error) { calls.Add(1); return nil, common.ErrBadRequest })
				if status.Code(e) == codes.ResourceExhausted {
					denials.Add(1)
				}
			}(i)
		}
		close(start)
		workers.Wait()
		if calls.Load() != int64(group.burst) || denials.Load() != int64(80-group.burst) || store.Counts().Entries != 1 {
			t.Fatal("mixed concurrent admission", calls.Load(), denials.Load(), store.Counts())
		}
	}
}
