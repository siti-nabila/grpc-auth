package test_scenarios

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	userv1 "github.com/siti-nabila/api-contracts/pb/user/v1"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/auth"
	"github.com/siti-nabila/api-contracts/pkg/dictionary/common"
	"github.com/siti-nabila/grpc-auth/internal/interceptors"
	"github.com/siti-nabila/grpc-auth/internal/repositories/domain"
	"github.com/siti-nabila/grpc-auth/pkg/helpers"
	"github.com/siti-nabila/grpc-auth/pkg/types"
	"github.com/siti-nabila/grpc-auth/tests/shared/fixtures"
	"github.com/siti-nabila/grpc-auth/tests/shared/testutils"
	"google.golang.org/grpc"
)

func SecurityAttempts() []testutils.Scenario {
	return []testutils.Scenario{{Name: "independent safe attempts preserve denial after rollback and audit failure", Run: securityAttempts}}
}
func securityAttempts(t *testing.T) {
	for _, kind := range []string{"denied", "dependency", "canceled", "audit fails", "audit panic", "unknown caller", "login already audited", "quota", "encoded quota", "success"} {
		t.Run(kind, func(t *testing.T) {
			base, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := fixtures.CallerContext(t, base, userv1.UserService_Me_FullMethodName)
			if kind == "unknown caller" {
				ctx = base
			}
			var calls []string
			observed := 0
			denial := error(common.ErrForbidden)
			if kind == "dependency" {
				denial = common.ErrDependencyUnavailable
			}
			if kind == "canceled" {
				denial = context.Canceled
			}
			if kind == "login already audited" {
				denial = auth.ErrLoginFailed
			}
			if kind == "success" {
				denial = nil
			}
			if kind == "quota" {
				denial = common.ErrRateLimited
			}
			if kind == "encoded quota" {
				denial = helpers.HandleErrorContext(ctx, common.ErrRateLimited)
			}
			interceptor, err := interceptors.NewSecurityAttemptInterceptor(func(got context.Context, req domain.AuditRequest) error {
				calls = append(calls, "audit")
				if got.Err() != nil {
					t.Fatal("canceled request reused for independent audit")
				}
				if _, ok := got.Deadline(); !ok {
					t.Fatal("attempt not bounded")
				}
				record, err := req.Record()
				if err != nil || record.ActorUserID != nil || record.ActorService == nil || *record.ActorService != "rest-orc" || req.Action != types.AuditSecurityAttemptFailed || req.Reason != types.AuditRequestDenied || req.Before != nil || req.After != nil {
					t.Fatal("fabricated user or unsafe audit", err)
				}
				data, _ := json.Marshal(record)
				if strings.Contains(string(data), "private-credential") {
					t.Fatal("credential leak")
				}
				if kind == "audit fails" {
					return errors.New("private-credential")
				}
				if kind == "audit panic" {
					panic("private-credential")
				}
				return nil
			}, func() { observed++ }, func() time.Time { return fixtures.ServiceTime })
			if err != nil {
				t.Fatal(err)
			}
			method := userv1.UserService_Me_FullMethodName
			if kind == "login already audited" {
				method = userv1.UserService_Login_FullMethodName
			}
			result, err := interceptor(ctx, "private-credential", &grpc.UnaryServerInfo{FullMethod: method}, func(got context.Context, _ any) (any, error) {
				if got != ctx {
					t.Fatal("request context changed")
				}
				calls = append(calls, "rollback")
				if kind == "canceled" {
					cancel()
				}
				return nil, denial
			})
			want := []string{"rollback"}
			if kind != "unknown caller" && kind != "login already audited" && kind != "success" && kind != "quota" && kind != "encoded quota" {
				want = append(want, "audit")
			}
			if !reflect.DeepEqual(calls, want) || result != nil || !errors.Is(err, denial) {
				t.Fatal("outcome changed", calls, err)
			}
			if observed != map[bool]int{true: 1, false: 0}[kind == "audit fails" || kind == "audit panic"] {
				t.Fatal("unbounded or missing telemetry")
			}
		})
	}
}
